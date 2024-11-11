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
	"github.com/electroneum/electroneum-sc/common/hexutil"
	ibfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/ibft/types"
	"github.com/electroneum/electroneum-sc/rlp"
)

// broadcastPrepare is called after receiving PRE-PREPARE from proposer node

// It
// - creates a PREPARE message
// - broadcast PREPARE message to other validators
func (c *core) broadcastPrepare() {
	logger := c.currentLogger(true, nil)

	// Create PREPARE message from the current proposal
	sub := c.current.Subject()
	prepare := ibfttypes.NewPrepare(sub.View.Sequence, sub.View.Round, sub.Digest)
	prepare.SetSource(c.Address())

	// Sign Message
	encodedPayload, err := prepare.EncodePayloadForSigning()
	if err != nil {
		withMsg(logger, prepare).Error("[Consensus]: Failed to encode payload of PREPARE message", "err", err)
		return
	}
	signature, err := c.backend.Sign(encodedPayload)
	if err != nil {
		withMsg(logger, prepare).Error("[Consensus]: Failed to sign PREPARE message", "err", err)
		return
	}
	prepare.SetSignature(signature)

	// RLP-encode message
	payload, err := rlp.EncodeToBytes(&prepare)
	if err != nil {
		withMsg(logger, prepare).Error("[Consensus]: Failed to encode PREPARE message", "err", err)
		return
	}

	withMsg(logger, prepare).Trace("IBFT: broadcast PREPARE message", "payload", hexutil.Encode(payload))
	c.cleanLogger.Info("[Consensus]: -> Broadcasting PREPARE message to validators")

	// Broadcast RLP-encoded message
	if err = c.backend.Broadcast(c.valSet, prepare.Code(), payload); err != nil {
		withMsg(logger, prepare).Error("[Consensus]: Failed to broadcast PREPARE message", "err", err)
		return
	}
}

// handlePrepare is called when receiving a PREPARE message

// It
// - validates PREPARE message digest matches the current block proposal
// - accumulates valid PREPARE message until reaching quorum
// - when quorum is reached update states to "Prepared" and broadcast COMMIT
func (c *core) handlePrepare(prepare *ibfttypes.Prepare) error {
	logger := c.currentLogger(true, prepare).New()

	logger.Trace("IBFT: handle PREPARE message", "prepares.count", c.current.IBFTPrepares.Size(), "quorum", c.QuorumSize())

	// Check digest
	if prepare.Digest != c.current.Proposal().Hash() {
		logger.Error("[Consensus]: Invalid PREPARE message digest")
		return errInvalidMessage
	}

	// Save PREPARE messages
	if err := c.current.IBFTPrepares.Add(prepare); err != nil {
		logger.Error("[Consensus]: Failed to save PREPARE message", "err", err)
		return err
	}

	logger = logger.New("prepares.count", c.current.IBFTPrepares.Size(), "quorum", c.QuorumSize())

	// Change to "Prepared" state if we've received quorum of PREPARE messages
	// and we are in earlier state than "Prepared"
	if (c.current.IBFTPrepares.Size() >= c.QuorumSize()) && c.state.Cmp(StatePrepared) < 0 {
		logger.Trace("[Consensus]: Received quorum of PREPARE messages")
		c.cleanLogger.Info("[Consensus]: <- Received quorum of PREPARE messages", "count", c.current.IBFTPrepares.Size(), "quorum", c.QuorumSize())

		// Accumulates PREPARE messages
		c.current.preparedRound = c.currentView().Round
		c.IBFTPreparedPrepares = make([]*ibfttypes.Prepare, 0)
		for _, m := range c.current.IBFTPrepares.Values() {
			c.IBFTPreparedPrepares = append(
				c.IBFTPreparedPrepares,
				ibfttypes.NewPrepareWithSigAndSource(
					m.View().Sequence, m.View().Round, m.(*ibfttypes.Prepare).Digest, m.Signature(), m.Source()))
		}

		if c.current.Proposal() != nil && c.current.Proposal().Hash() == prepare.Digest {
			logger.Debug("IBFT: PREPARE message matches proposal", "proposal", c.current.Proposal().Hash(), "prepare", prepare.Digest)
			c.current.preparedBlock = c.current.Proposal()
		}

		c.setState(StatePrepared)
		c.broadcastCommit()
	} else {
		logger.Debug("IBFT: accepted PREPARE messages")
	}

	return nil
}
