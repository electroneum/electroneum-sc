// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"errors"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

const (
	// === Future Window Limits ===

	// 32 blocks ahead is far beyond any legitimate async delivery.
	// If you're 32 blocks behind, you need chain sync, not backlog.
	MaxFutureSequenceGap uint64 = 32

	// 50 rounds covers hours of continuous failed consensus.
	// No legitimate scenario exceeds this.
	MaxFutureRoundGap uint64 = 15

	// === Memory Protection Limits ===

	// Per-validator cap: 1024 messages ≈ 1MB per spoofed validator address.
	// Generous enough for legitimate traffic bursts.
	MaxBacklogPerValidator = 1024

	// MinBacklogTotal: floor for small validator sets.
	// Ensures small networks (e.g., 4 validators) still have reasonable capacity.
	// 4096 messages ≈ 4MB minimum.
	MinBacklogTotal = 4096

	// MaxBacklogTotalCeiling: hard ceiling regardless of validator count.
	// Prevents memory exhaustion in very large networks.
	// 131072 messages ≈ 128MB maximum.
	MaxBacklogTotalCeiling = 131072
)

var (
	// msgPriority is defined for calculating processing priority to speedup consensus
	// msgPreprepare > msgCommit > msgPrepare
	msgPriority = map[uint64]int{
		qbfttypes.PreprepareCode: 1,
		qbfttypes.CommitCode:     2,
		qbfttypes.PrepareCode:    3,
	}
)

// checkMessage checks that a message matches our current QBFT state
//
// In particular it ensures that
// - message has the expected round
// - message has the expected sequence
// - message type is expected given our current state

// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message view is larger than current view
// return errOldMessage if the message view is smaller than current view
func (c *core) checkMessage(msgCode uint64, view *istanbul.View) error {
	if view == nil || view.Sequence == nil || view.Round == nil {
		return errInvalidMessage
	}

	if msgCode == qbfttypes.RoundChangeCode {
		// if ROUND-CHANGE message
		// check that
		// - sequence matches our current sequence
		// - round is in the future
		if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
			return errFutureMessage
		} else if view.Cmp(c.currentView()) < 0 {
			return errOldMessage
		}
		return nil
	}

	// If not ROUND-CHANGE
	// check that round and sequence equals our current round and sequence
	if view.Cmp(c.currentView()) > 0 {
		return errFutureMessage
	}

	if view.Cmp(c.currentView()) < 0 {
		return errOldMessage
	}

	switch c.state {
	case StateAcceptRequest:
		// StateAcceptRequest only accepts msgPreprepare and msgRoundChange
		// other messages are future messages
		if msgCode > qbfttypes.PreprepareCode {
			return errFutureMessage
		}
		return nil
	case StatePreprepared:
		// StatePreprepared only accepts msgPrepare and msgRoundChange
		// message less than msgPrepare are invalid and greater are future messages
		if msgCode < qbfttypes.PrepareCode {
			return errInvalidMessage
		} else if msgCode > qbfttypes.PrepareCode {
			return errFutureMessage
		}
		return nil
	case StatePrepared:
		// StatePrepared only accepts msgCommit and msgRoundChange
		// other messages are invalid messages
		if msgCode < qbfttypes.CommitCode {
			return errInvalidMessage
		}
		return nil
	case StateCommitted:
		// StateCommit rejects all messages other than msgRoundChange
		return errInvalidMessage
	}
	return nil
}

// isValidatorAddress checks if the given address is a current validator.
func (c *core) isValidatorAddress(addr common.Address) bool {
	if c.valSet == nil {
		return false
	}
	_, v := c.valSet.GetByAddress(addr)
	return v != nil
}

// withinBacklogFutureWindow checks if a message is within acceptable future bounds.
func (c *core) withinBacklogFutureWindow(msgCode uint64, view istanbul.View) bool {
	if c.current == nil {
		return false // not ready yet, drop message
	}

	cur := c.currentView()

	// Same-sequence messages are commonly received early and should be allowed.
	if view.Sequence.Cmp(cur.Sequence) == 0 {
		// ROUND-CHANGE at same sequence can be spammed with huge future rounds. Cap it.
		if msgCode == qbfttypes.RoundChangeCode {
			maxRound := new(big.Int).Add(cur.Round, new(big.Int).SetUint64(MaxFutureRoundGap))
			if view.Round.Cmp(maxRound) > 0 {
				return false
			}
		}
		return true
	}

	// Not a backlog candidate if it's behind.
	if view.Sequence.Cmp(cur.Sequence) < 0 {
		return false
	}

	// Cap how far ahead by sequence we will backlog.
	maxSeq := new(big.Int).Add(cur.Sequence, new(big.Int).SetUint64(MaxFutureSequenceGap))
	if view.Sequence.Cmp(maxSeq) > 0 {
		return false
	}

	return true
}

// addToBacklog stores a future message for later processing, subject to
// validator verification, future window limits, and capacity caps.
func (c *core) addToBacklog(msg qbfttypes.QBFTMessage) {
	logger := c.currentLogger(true, msg)

	src := msg.Source()
	if src == c.Address() {
		logger.Warn("IBFT: backlog from self")
		return
	}

	// Drop messages that claim to be from non-validators.
	// This prevents filling the backlogs map with arbitrary addresses.
	if !c.isValidatorAddress(src) {
		logger.Trace("IBFT: dropping backlog message from non-validator", "src", src)
		return
	}

	view := msg.View()

	// Drop far-future messages (including huge ROUND-CHANGE rounds at current sequence).
	if !c.withinBacklogFutureWindow(msg.Code(), view) {
		logger.Trace("IBFT: dropping far-future backlog message",
			"src", src, "code", msg.Code(),
			"view_seq", view.Sequence, "view_round", view.Round,
			"cur_seq", c.currentView().Sequence, "cur_round", c.currentView().Round,
		)
		return
	}

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	// Global cap (dynamic based on validator count)
	maxTotal := c.maxBacklogTotal()
	if c.backlogsTotal >= maxTotal {
		logger.Trace("IBFT: dropping backlog message (global cap reached)",
			"cap", maxTotal, "total", c.backlogsTotal,
		)
		return
	}

	backlog := c.backlogs[src]
	if backlog == nil {
		backlog = prque.New()
		c.backlogs[src] = backlog
	}

	// Per-validator cap
	if backlog.Size() >= MaxBacklogPerValidator {
		logger.Trace("IBFT: dropping backlog message (per-validator cap reached)",
			"src", src, "cap", MaxBacklogPerValidator, "size", backlog.Size(),
		)
		return
	}

	backlog.Push(msg, toPriority(msg.Code(), &view))
	c.backlogsTotal++

	logger.Trace("IBFT: new backlog message", "backlogs_total", c.backlogsTotal, "src_backlog_size", backlog.Size())
}

// processBacklog looks up future messages that have been backlogged and posts them on
// the event channel so the main handler loop can handle them.
// It is called on every state change.
func (c *core) processBacklog() {
	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	for srcAddress, backlog := range c.backlogs {
		if backlog == nil {
			continue
		}

		// If the address is no longer a validator, drop its entire backlog.
		_, src := c.valSet.GetByAddress(srcAddress)
		if src == nil {
			c.backlogsTotal -= backlog.Size()
			delete(c.backlogs, srcAddress)
			continue
		}

		logger := c.logger.New("from", src, "state", c.state)
		logger.Trace("IBFT: process backlog")

		// Process until:
		//  1) backlog is empty, OR
		//  2) the next message is still a future message (we requeue it and stop)
		for !backlog.Empty() {
			m, prio := backlog.Pop()
			c.backlogsTotal--

			msg := m.(qbfttypes.QBFTMessage)
			code := msg.Code()
			view := msg.View()

			// Push back if it's still a future message
			err := c.checkMessage(code, &view)
			if err != nil {
				// Use errors.Is to be robust to wrapped errors.
				if errors.Is(err, errFutureMessage) {
					logger.Trace("IBFT: stop processing backlog", "msg", m)

					// Requeue only if it still fits our window/caps (defensive).
					if c.withinBacklogFutureWindow(code, view) &&
						backlog.Size() < MaxBacklogPerValidator &&
						c.backlogsTotal < c.maxBacklogTotal() {

						backlog.Push(m, prio)
						c.backlogsTotal++
					}
					break
				}

				// Old/invalid messages are dropped permanently.
				logger.Trace("IBFT: skip backlog message", "msg", m, "err", err)
				continue
			}

			logger.Trace("IBFT: post backlog event", "msg", m)

			// Post backlog event for main handler loop
			event := backlogEvent{
				src: src,
				msg: msg,
			}
			go c.sendEvent(event)
		}

		// Clean up empty queues
		if backlog.Empty() {
			delete(c.backlogs, srcAddress)
		}
	}
}

// maxBacklogTotal returns the dynamic maximum backlog size based on validator count.
// This ensures fair allocation per validator while maintaining a hard memory ceiling.
func (c *core) maxBacklogTotal() int {
	if c.valSet == nil {
		return MinBacklogTotal
	}

	// Allow 2x headroom so validators can burst while others are quiet
	dynamic := c.valSet.Size() * MaxBacklogPerValidator * 2

	if dynamic < MinBacklogTotal {
		return MinBacklogTotal
	}
	if dynamic > MaxBacklogTotalCeiling {
		return MaxBacklogTotalCeiling
	}
	return dynamic
}

func toPriority(msgCode uint64, view *istanbul.View) float32 {
	if msgCode == qbfttypes.RoundChangeCode {
		// For msgRoundChange, set the message priority based on its sequence
		return -float32(view.Sequence.Uint64() * 1000)
	}
	// FIXME: round will be reset as 0 while new sequence
	// 10 * Round limits the range of message code is from 0 to 9
	// 1000 * Sequence limits the range of round is from 0 to 99
	return -float32(view.Sequence.Uint64()*1000 + view.Round.Uint64()*10 + uint64(msgPriority[msgCode]))
}
