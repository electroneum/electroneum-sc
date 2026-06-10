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
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/rlp"
)

// broadcastCommit is called when receiving quorum of PREPARE message

// It
// - creates a COMMIT message from current proposal
// - broadcast COMMIT message to other validators
func (c *core) broadcastCommit() {
	var err error

	logger := c.currentLogger(true, nil)

	sub := c.current.Subject()

	var header *types.Header
	if block, ok := c.current.Proposal().(*types.Block); ok {
		header = block.Header()
	}
	// Create Commit Seal
	commitSeal, err := c.backend.SignWithoutHashing(PrepareCommittedSeal(header, uint32(c.currentView().Round.Uint64())))
	if err != nil {
		logger.Error("[Consensus]: Failed to create COMMIT seal", "sub", sub, "err", err)
		return
	}

	commit := qbfttypes.NewCommit(sub.View.Sequence, sub.View.Round, sub.Digest, commitSeal)
	commit.SetSource(c.Address())

	// Sign Message
	encodedPayload, err := commit.EncodePayloadForSigning()
	if err != nil {
		withMsg(logger, commit).Error("[Consensus]: Failed to encode payload of COMMIT message", "err", err)
		return
	}

	signature, err := c.backend.Sign(encodedPayload)
	if err != nil {
		withMsg(logger, commit).Error("[Consensus]: Failed to sign COMMIT message", "err", err)
		return
	}
	commit.SetSignature(signature)

	// RLP-encode message
	payload, err := rlp.EncodeToBytes(&commit)
	if err != nil {
		withMsg(logger, commit).Error("[Consensus]: Failed to encode COMMIT message", "err", err)
		return
	}

	withMsg(logger, commit).Trace("IBFT: broadcast COMMIT message", "payload", hexutil.Encode(payload))
	c.cleanLogger.Info("[Consensus]: -> Broadcasting COMMIT message to validators")

	// Broadcast RLP-encoded message
	if err = c.backend.Broadcast(c.valSet, commit.Code(), payload); err != nil {
		withMsg(logger, commit).Error("[Consensus]: Failed to broadcast COMMIT message", "err", err)
		return
	}
}

// handleCommitMsg is called when receiving a COMMIT message from another validator

// It
// - validates COMMIT message digest matches the current block proposal
// - accumulates valid COMMIT messages until reaching quorum
// - when quorum of COMMIT messages is reached then update state and commits
func (c *core) handleCommitMsg(commit *qbfttypes.Commit) error {
	logger := c.currentLogger(true, commit)

	logger.Trace("IBFT: handle COMMIT message", "commits.count", c.current.QBFTCommits.Size(), "quorum", c.QuorumSize())

	// Check digest
	if commit.Digest != c.current.Proposal().Hash() {
		logger.Error("IBFT: invalid COMMIT message digest", "digest", commit.Digest, "proposal", c.current.Proposal().Hash().String())
		return errInvalidMessage
	}

	// Verify the committed seal embedded in the message. The outer message
	// signature (already checked in verifySignatures) only proves the sender
	// authored the COMMIT; it does not prove the CommitSeal actually signs the
	// proposal. Without this check a Byzantine validator could have a malformed
	// seal counted toward quorum and copied verbatim into the block header by
	// commitQBFT, even though verifyCommittedSeals would later reject the block
	// network-wide. We validate here so the local commit path is symmetric with
	// the receive/verify path.
	if err := c.verifyCommittedSeal(commit); err != nil {
		logger.Error("IBFT: invalid COMMIT seal", "err", err)
		return err
	}

	// Add to received msgs
	if err := c.current.QBFTCommits.Add(commit); err != nil {
		c.logger.Error("IBFT: failed to save COMMIT message", "err", err)
		return err
	}

	logger = logger.New("commits.count", c.current.QBFTCommits.Size(), "quorum", c.QuorumSize())

	// If we reached thresho
	if c.current.QBFTCommits.Size() >= c.QuorumSize() {
		logger.Trace("[Consensus]: Received quorum of COMMIT messages")
		c.cleanLogger.Info("[Consensus]: <- Received quorum of COMMIT messages", "count", c.current.QBFTCommits.Size(), "quorum", c.QuorumSize())
		c.commitQBFT()
	} else {
		logger.Debug("IBFT: accepted new COMMIT messages")
	}

	return nil
}

// verifyCommittedSeal checks that commit.CommitSeal is a signature over
// PrepareCommittedSeal(proposalHeader, commitRound) produced by the validator
// that sent the COMMIT message. It mirrors the seal construction in
// broadcastCommit and the network-wide check in Engine.verifyCommittedSeals.
func (c *core) verifyCommittedSeal(commit *qbfttypes.Commit) error {
	// A committed seal is a fixed-length ECDSA signature; reject anything else
	// before attempting recovery so a malformed length cannot reach the header.
	if len(commit.CommitSeal) != types.IstanbulExtraSeal {
		return errInvalidCommittedSeal
	}

	block, ok := c.current.Proposal().(*types.Block)
	if !ok {
		return errInvalidMessage
	}

	// The seal signs the proposal header under the round the sender committed
	// in. broadcastCommit signs PrepareCommittedSeal(header, currentRound), and
	// the COMMIT message carries that same round in its view.
	committedSeal := PrepareCommittedSeal(block.Header(), uint32(commit.View().Round.Uint64()))

	signer, err := istanbul.GetSignatureAddressNoHashing(committedSeal, commit.CommitSeal)
	if err != nil {
		return errInvalidCommittedSeal
	}

	// The recovered signer must be the validator that sent the COMMIT. Because
	// verifySignatures already established that Source() is a member of the
	// current validator set, matching against Source() also guarantees the seal
	// belongs to a validator.
	if signer != commit.Source() {
		return errInvalidCommittedSeal
	}

	return nil
}

// commitQBFT is called once quorum of commits is reached
// - computes committedSeals from each received commit messages
// - then commits block proposal to database with committed seals
// - broadcast round change
func (c *core) commitQBFT() {
	c.setState(StateCommitted)

	proposal := c.current.Proposal()
	if proposal != nil {
		// Compute committed seals
		committedSeals := make([][]byte, c.current.QBFTCommits.Size())
		for i, msg := range c.current.QBFTCommits.Values() {
			commitMsg := msg.(*qbfttypes.Commit)
			// Defense in depth: every seal in the set was already validated in
			// handleCommitMsg, but guard the length again so a malformed seal
			// can never be copied into the block header even if a future code
			// path adds to QBFTCommits without going through that check.
			if len(commitMsg.CommitSeal) != types.IstanbulExtraSeal {
				c.currentLogger(true, commitMsg).Error("IBFT: refusing to commit malformed COMMIT seal", "source", commitMsg.Source())
				c.broadcastNextRoundChange()
				return
			}
			committedSeals[i] = make([]byte, types.IstanbulExtraSeal)
			copy(committedSeals[i][:], commitMsg.CommitSeal[:])
		}

		// Commit proposal to database
		if err := c.backend.Commit(proposal, committedSeals, c.currentView().Round); err != nil {
			c.currentLogger(true, nil).Error("IBFT: error committing proposal", "err", err)
			c.broadcastNextRoundChange()
			return
		}
	}
}
