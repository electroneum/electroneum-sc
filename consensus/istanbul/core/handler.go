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
	"fmt"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rlp"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	c.logger.Info("IBFT: start")
	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.subscribeEvents()
	c.handlerWg.Add(1)
	go c.handleEvents()

	// Start a new round from last sequence + 1
	c.startNewRound(common.Big0)

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.logger.Info("IBFT: stopping...")
	c.stopTimer()
	c.unsubscribeEvents()

	// Make sure the handler goroutine exits
	c.handlerWg.Wait()

	// The event loop has stopped; do not keep serving its last snapshot to
	// IsProposer, IsCurrentProposal or RoundState.
	c.roundStateInfo.Store(nil)
	c.logger.Info("IBFT: stopped")
	return nil
}

// ----------------------------------------------------------------------------

// Subscribe both internal and external events
func (c *core) subscribeEvents() {
	c.events = c.backend.EventMux().Subscribe(
		// external events
		istanbul.RequestEvent{},
		istanbul.MessageEvent{},
		// internal events
		backlogEvent{},
	)
	c.timeoutSub = c.backend.EventMux().Subscribe(
		timeoutEvent{},
	)
	c.finalCommittedSub = c.backend.EventMux().Subscribe(
		istanbul.FinalCommittedEvent{},
	)
}

// Unsubscribe all events
func (c *core) unsubscribeEvents() {
	c.events.Unsubscribe()
	c.timeoutSub.Unsubscribe()
	c.finalCommittedSub.Unsubscribe()
}

// handleEvents starts main qbft handler loop that processes all incoming messages
// sequentially. Each time a message is processed, internal QBFT state is mutated

// when processing a message it makes sure that the message matches the current state
// - in case the message is past, either for an older round or a state that already got acknowledge (e.g. a PREPARE message but we
// are already in Prepared state), then message is discarded
// - in case the message is future, either for a future round or a state yet to be reached (e.g. a COMMIT message but we are
// in PrePrepared state), then message is added to backlog for future processing
// - if correct time, message is handled

// Each time a message is successfully handled it is gossiped to other validators
func (c *core) handleEvents() {
	// Clear state
	defer func() {
		c.current = nil
		c.handlerWg.Done()
	}()

	for {
		select {
		case event, ok := <-c.events.Chan():
			if !ok {
				return
			}

			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case istanbul.RequestEvent:
				// we are block proposer and look to get our block proposal validated by other validators
				r := &Request{
					Proposal: ev.Proposal,
				}
				err := c.handleRequest(r)
				if err == errFutureMessage {
					// store request for later treatment
					c.storeRequestMsg(r)
				}
			case istanbul.MessageEvent:
				// we received a message from another validator
				if err := c.handleEncodedMsg(ev.Code, ev.Payload); err == nil {
					// if successfully processed, we gossip message to other validators
					c.backend.Gossip(c.valSet, ev.Code, ev.Payload)
				}
			case backlogEvent:
				// we process again a future message that was backlogged
				// no need to check signature as it was already node when we first received message
				// This message was already charged against the backlog byte budget when
				// first admitted and has since been popped, so if it re-backlogs it is
				// re-charged from its own encoded size; pass ev.size as its charge.
				if err := c.handleDecodedMessage(ev.msg, ev.size); err == nil {
					if data, err := rlp.EncodeToBytes(ev.msg); err != nil {
						c.logger.Error("IBFT: can not encode backlog message", "err", err)
					} else {
						// if successfully processed, we gossip message to other validators
						c.backend.Gossip(c.valSet, ev.msg.Code(), data)
					}
				}
			}
		case event, ok := <-c.timeoutSub.Chan():
			// we received a round change timeout
			if !ok {
				return
			}
			switch ev := event.Data.(type) {
			case timeoutEvent:
				if c.current != nil && c.current.Round().Uint64() == ev.round {
					c.handleTimeoutMsg()
				}
			}
		case event, ok := <-c.finalCommittedSub.Chan():
			// our block proposal got committed
			if !ok {
				return
			}
			switch event.Data.(type) {
			case istanbul.FinalCommittedEvent:
				c.handleFinalCommitted()
			}
		}
		// Every consensus state change happens in this loop, so a snapshot
		// taken after each event is always current.
		c.publishRoundState()
	}
}

// sendEvent sends events to mux
func (c *core) sendEvent(ev interface{}) {
	c.backend.EventMux().Post(ev)
}

func (c *core) handleEncodedMsg(code uint64, data []byte) error {
	logger := c.logger.New("code", code, "size", len(data))

	if _, ok := qbfttypes.MessageCodes()[code]; !ok {
		logger.Error("IBFT: invalid message event code")
		return fmt.Errorf("invalid message event code %v", code)
	}

	// PRE-PREPARE and ROUND-CHANGE carry a whole block, which can take tens of
	// times its wire size in memory once decoded. Check the sender signed it
	// from the raw bytes first, so an outsider can't make us decode anything big.
	verified, err := c.preVerifySignature(code, data)
	if err != nil {
		return err
	}

	// Decode data into a QBFTMessage
	m, err := qbfttypes.Decode(code, data)
	if err != nil {
		logger.Error("IBFT: invalid message", "err", err)
		return err
	}

	// Verify signatures and set source address
	if err = c.verifySignaturesWith(m, verified); err != nil {
		return err
	}

	// len(data) is the true encoded wire size of this message. We carry it into
	// the backlog so future-message admission can charge a byte budget, not only
	// a message count (a large PRE-PREPARE proposal is orders of magnitude bigger
	// than the ~1KB the count cap assumes).
	return c.handleDecodedMessage(m, len(data))
}

func (c *core) handleDecodedMessage(m qbfttypes.QBFTMessage, encodedSize int) error {
	view := m.View()
	if err := c.checkMessage(m.Code(), &view); err != nil {
		// Store in the backlog it it's a future message
		if err == errFutureMessage {
			c.addToBacklog(m, encodedSize)
		}
		return err
	}

	return c.deliverMessage(m)
}

// Deliver to specific message handler
func (c *core) deliverMessage(m qbfttypes.QBFTMessage) error {
	var err error

	switch m.Code() {
	case qbfttypes.PreprepareCode:
		err = c.handlePreprepareMsg(m.(*qbfttypes.Preprepare))
	case qbfttypes.PrepareCode:
		err = c.handlePrepare(m.(*qbfttypes.Prepare))
	case qbfttypes.CommitCode:
		err = c.handleCommitMsg(m.(*qbfttypes.Commit))
	case qbfttypes.RoundChangeCode:
		err = c.handleRoundChange(m.(*qbfttypes.RoundChange))
	default:
		c.logger.Error("IBFT: invalid message code", "code", m.Code())
		return errInvalidMessage
	}

	return err
}

func (c *core) handleTimeoutMsg() {
	logger := c.currentLogger(true, nil)
	// Start the new round
	round := c.current.Round()
	nextRound := new(big.Int).Add(round, common.Big1)

	logger.Warn("[Consensus]: Round reached timeout", "pr", c.current.preparedRound)
	c.startNewRound(nextRound)
	logger.Trace("IBFT: TIMER CHANGED ROUND", "pr", c.current.preparedRound)

	// Send Round Change
	c.broadcastRoundChange(nextRound)
}

// Verifies the signature of the message m and of any justification payloads
// piggybacked in m, if any. It also sets the source address on the messages
// and justification payloads.
func (c *core) verifySignatures(m qbfttypes.QBFTMessage) error {
	return c.verifySignaturesWith(m, make(map[common.Hash]common.Address))
}

// verifySignaturesWith is verifySignatures seeded with (payload, signature)
// pairs that are already known to be good, keyed as in the verify closure.
func (c *core) verifySignaturesWith(m qbfttypes.QBFTMessage, verified map[common.Hash]common.Address) error {
	logger := c.currentLogger(true, m)

	// c.valSet is first populated by startNewRound. The handleEvents goroutine is started
	// before that first call (see Start), so a message can reach verifySignatures during the
	// brief startup window before any validator set exists. Every downstream signature check
	// dereferences c.valSet (checkValidatorSignature -> valSet.GetByAddress), so without this
	// guard that window is a nil-pointer panic. Reject the message instead; it is not for any
	// round we can validate yet, and a legitimate peer will re-gossip.
	if c.valSet == nil {
		logger.Warn("IBFT: dropping message received before validator set is initialised")
		return errInvalidMessage
	}

	// Memoize verified (payload, signature) pairs within this message so a justification
	// containing many identical signed payloads only pays for one ecrecover. This preserves
	// fail-fast semantics (the first bad signature still errors) while removing the CPU
	// amplification from duplicate entries.
	verify := func(m qbfttypes.QBFTMessage) error {
		payload, err := m.EncodePayloadForSigning()
		if err != nil {
			logger.Error("IBFT: invalid message payload", "err", err)
			return err
		}
		key := crypto.Keccak256Hash(payload, m.Signature())
		if source, ok := verified[key]; ok {
			m.SetSource(source)
			return nil
		}
		source, err := c.validateFn(payload, m.Signature())
		if err != nil {
			logger.Error("IBFT: invalid message signature", "err", err)
			return errInvalidSigner
		}
		verified[key] = source
		m.SetSource(source)
		return nil
	}

	// DoS hardening: reject oversized justifications BEFORE spending any ecrecover
	// work on them. A justification can never legitimately contain more payloads than
	// there are validators, so a longer list is malformed by definition. These are the
	// same bounds enforced later in isJustified/hasMatchingRoundChangeAndPrepares, moved
	// ahead of the (unbounded, per-entry) signature loops below so that a single message
	// carrying tens of thousands of duplicate signed payloads cannot amplify one message
	// into seconds of ecrecover on every receiving validator's consensus goroutine.
	maxJustification := c.valSet.Size()
	switch msgType := m.(type) {
	case *qbfttypes.RoundChange:
		if len(msgType.Justification) > maxJustification {
			logger.Warn("IBFT: rejecting round change with oversized justification", "count", len(msgType.Justification), "max", maxJustification)
			return errInvalidMessage
		}
	case *qbfttypes.Preprepare:
		if len(msgType.JustificationRoundChanges) > maxJustification || len(msgType.JustificationPrepares) > maxJustification {
			logger.Warn("IBFT: rejecting preprepare with oversized justification", "roundChanges", len(msgType.JustificationRoundChanges), "prepares", len(msgType.JustificationPrepares), "max", maxJustification)
			return errInvalidMessage
		}
	}

	// Verifies the signature of the message
	if err := verify(m); err != nil {
		return err
	}

	// Verifies the signature of piggybacked justification payloads.
	switch msgType := m.(type) {
	case *qbfttypes.RoundChange:
		signedPreparePayloads := msgType.Justification
		for _, p := range signedPreparePayloads {
			if err := verify(p); err != nil {
				return err
			}
		}
	case *qbfttypes.Preprepare:
		signedRoundChangePayloads := msgType.JustificationRoundChanges
		for _, p := range signedRoundChangePayloads {
			if err := verify(p); err != nil {
				return err
			}
		}
		// Verify the prepares provided inside the justification
		signedPreparePayloads := msgType.JustificationPrepares
		for _, p := range signedPreparePayloads {
			if err := verify(p); err != nil {
				return err
			}
		}
	}

	return nil
}

// splitFields splits an encoded list that must hold exactly n items into the
// items' encodings. The results point into b, nothing is copied.
func splitFields(b []byte, n int) ([][]byte, error) {
	content, rest, err := rlp.SplitList(b)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, rlp.ErrMoreThanOneValue
	}
	fields := make([][]byte, 0, n)
	for len(content) > 0 && len(fields) < n {
		_, _, tail, err := rlp.Split(content)
		if err != nil {
			return nil, err
		}
		fields = append(fields, content[:len(content)-len(tail)])
		content = tail
	}
	if len(fields) != n || len(content) != 0 {
		return nil, fmt.Errorf("expected %d fields", n)
	}
	return fields, nil
}

// countRawList counts the items in an encoded list. Any other zero-size value
// counts as empty, matching RoundChange.DecodeRLP, which skips anything whose
// Kind reports size 0.
func countRawList(list []byte) (int, error) {
	kind, content, rest, err := rlp.Split(list)
	if err != nil {
		return 0, err
	}
	if len(rest) != 0 {
		return 0, rlp.ErrMoreThanOneValue
	}
	switch {
	case kind == rlp.List:
		return rlp.CountValues(content)
	case kind == rlp.Byte, len(content) == 0:
		return 0, nil
	}
	return 0, rlp.ErrExpectedList
}

// preVerifySignature checks the outer signature of a PRE-PREPARE or ROUND-CHANGE,
// the length of its justifications and the size of the block it carries,
// without decoding the block or the justification entries. Other messages are
// small and pass straight through. On success the verified pair is returned so
// verifySignaturesWith doesn't pay for the same ecrecover twice.
//
// Wire layouts (see the EncodeRLP methods in qbfttypes):
//
//	PRE-PREPARE:  [[payload, sig], [roundChanges, prepares]], payload = [seq, round, block]
//	ROUND-CHANGE: [[payload, sig], preparedBlock, prepares],  payload = [seq, round, [pr, digest, bad] or []]
func (c *core) preVerifySignature(code uint64, data []byte) (map[common.Hash]common.Address, error) {
	verified := make(map[common.Hash]common.Address)
	if code != qbfttypes.PreprepareCode && code != qbfttypes.RoundChangeCode {
		return verified, nil
	}
	if c.valSet == nil {
		c.logger.Warn("IBFT: dropping message received before validator set is initialised")
		return nil, errInvalidMessage
	}
	decodeErr := istanbulcommon.ErrFailedDecodePreprepare
	if code == qbfttypes.RoundChangeCode {
		decodeErr = istanbulcommon.ErrFailedDecodeRoundChange
	}
	n := 2
	if code == qbfttypes.RoundChangeCode {
		n = 3
	}
	msg, err := splitFields(data, n)
	if err != nil {
		return nil, decodeErr
	}
	signed, err := splitFields(msg[0], 2)
	if err != nil {
		return nil, decodeErr
	}
	payload := signed[0]
	signature, _, err := rlp.SplitString(signed[1])
	if err != nil {
		return nil, decodeErr
	}
	var justifications [][]byte
	if code == qbfttypes.PreprepareCode {
		if justifications, err = splitFields(msg[1], 2); err != nil {
			return nil, decodeErr
		}
	} else {
		justifications = msg[2:]
	}
	// Same bound as verifySignaturesWith, applied before the entries are decoded.
	for _, list := range justifications {
		if n, err := countRawList(list); err != nil || n > c.valSet.Size() {
			c.logger.Warn("IBFT: rejecting message with oversized justification", "code", code, "count", n, "max", c.valSet.Size())
			return nil, errInvalidMessage
		}
	}
	var preparedBlock []byte
	if code == qbfttypes.RoundChangeCode {
		preparedBlock = msg[1]
	}
	if err := checkRawBlock(code, payload, preparedBlock); err != nil {
		c.logger.Warn("IBFT: rejecting message with invalid block", "code", code, "err", err)
		return nil, errInvalidMessage
	}
	// Signing payload is [code, payload], see EncodePayloadForSigning.
	signingPayload, err := rlp.EncodeToBytes([]interface{}{code, rlp.RawValue(payload)})
	if err != nil {
		return nil, err
	}
	source, err := c.validateFn(signingPayload, signature)
	if err != nil {
		c.logger.Warn("IBFT: invalid message signature", "code", code, "err", err)
		return nil, errInvalidSigner
	}
	verified[crypto.Keccak256Hash(signingPayload, signature)] = source
	return verified, nil
}

// minTxGas is the least gas a transaction can leave used in a block: the
// intrinsic gas less the biggest refund any fork allows (half, pre-London).
const minTxGas = params.TxGas / params.RefundQuotient

// checkRawBlock bounds the block carried by a PRE-PREPARE (inside the signed
// payload) or ROUND-CHANGE (next to it, bound only by its hash) before the
// block is decoded. Lots of tiny transactions are what make a block expensive
// to decode, and a real block can only hold GasLimit/minTxGas of them. The
// header is covered by the signature, directly or through the prepared digest,
// so its gas limit can't be inflated to get around this.
func checkRawBlock(code uint64, payload, preparedBlock []byte) error {
	fields, err := splitFields(payload, 3)
	if err != nil {
		return err
	}
	var (
		block  = fields[2]
		digest *common.Hash
	)
	if code == qbfttypes.RoundChangeCode {
		if n, err := countRawList(preparedBlock); err != nil || n == 0 {
			return err // nothing prepared
		}
		prepared, err := splitFields(fields[2], 3)
		if err != nil {
			return errors.New("prepared block without a prepared digest")
		}
		digest = new(common.Hash)
		if err := rlp.DecodeBytes(prepared[1], digest); err != nil {
			return err
		}
		block = preparedBlock
	}
	parts, err := splitFields(block, 3) // [header, txs, uncles]
	if err != nil {
		return err
	}
	var header types.Header
	if err := rlp.DecodeBytes(parts[0], &header); err != nil {
		return err
	}
	if digest != nil && header.Hash() != *digest {
		return errors.New("prepared block does not match prepared digest")
	}
	txs, err := countRawList(parts[1])
	if err != nil {
		return err
	}
	if uint64(txs) > header.GasLimit/minTxGas {
		return fmt.Errorf("%d txs exceed what gas limit %d allows", txs, header.GasLimit)
	}
	if uncles, err := countRawList(parts[2]); err != nil || uncles != 0 {
		return errors.New("block has uncles")
	}
	return nil
}

func (c *core) currentLogger(state bool, msg qbfttypes.QBFTMessage) log.Logger {
	logCtx := []interface{}{}
	if c.current != nil {
		logCtx = append(logCtx,
			"current.round", c.current.Round().Uint64(),
			"current.sequence", c.current.Sequence().Uint64(),
		)
	}

	if state {
		logCtx = append(logCtx, "state", c.state)
	}

	if msg != nil {
		logCtx = append(
			logCtx,
			"msg.code", msg.Code(),
			"msg.source", msg.Source().String(),
			"msg.round", msg.View().Round.Uint64(),
			"msg.sequence", msg.View().Sequence.Uint64(),
		)
	}

	return c.logger.New(logCtx...)
}

func withMsg(logger log.Logger, msg qbfttypes.QBFTMessage) log.Logger {
	return logger.New(
		"msg.code", msg.Code(),
		"msg.source", msg.Source().String(),
		"msg.round", msg.View().Round.Uint64(),
		"msg.sequence", msg.View().Sequence.Uint64(),
	)
}
