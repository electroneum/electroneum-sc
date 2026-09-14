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

package backend

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	qbftengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/testutils"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
)

// setVote overwrites the validator vote in a block's QBFT extra-data.
func setVote(t *testing.T, block *types.Block, vote *types.ValidatorVote) *types.Block {
	t.Helper()
	header := block.Header()
	if err := qbftengine.ApplyHeaderQBFTExtra(header, func(extra *types.QBFTExtra) error {
		extra.Vote = vote
		return nil
	}); err != nil {
		t.Fatalf("apply vote to extra-data: %v", err)
	}
	return block.WithSeal(header)
}

// commitBlock finalizes a block with committed seals from the given keys, the
// way a real 2F+1 quorum would.
func commitBlock(t *testing.T, engine *Backend, block *types.Block, keys []*ecdsa.PrivateKey) *types.Block {
	t.Helper()
	proposalSeal := qbftengine.PrepareCommittedSeal(block.Header(), 0)
	seals := make([][]byte, 0, len(keys))
	for i, key := range keys {
		seal, err := crypto.Sign(proposalSeal, key)
		if err != nil {
			t.Fatalf("sign committed seal %d: %v", i, err)
		}
		seals = append(seals, seal)
	}
	header := block.Header()
	if err := engine.EngineForBlockNumber(header.Number).CommitHeader(header, seals, big.NewInt(0)); err != nil {
		t.Fatalf("commit header: %v", err)
	}
	return block.WithSeal(header)
}

// TestQBFTVerifyRejectsInvalidVoteType covers the proposal-verification path: a
// proposed block carrying a vote type other than 0x00/0xff must be rejected
// before honest validators sign it.
func TestQBFTVerifyRejectsInvalidVoteType(t *testing.T) {
	genesis, nodeKeys := testutils.GenesisAndKeys(4)
	config := copyConfig(istanbul.DefaultConfig)
	chain, engine := newBlockchainFromConfig(genesis, nodeKeys, config)
	defer engine.Stop()

	candidate := common.HexToAddress("0x000000000000000000000000000000000000dead")

	for _, voteType := range []byte{0x01, 0x42, 0x7f, 0xfe} {
		block := makeBlockWithoutSeal(chain, engine, chain.Genesis(), true)
		block = setVote(t, block, &types.ValidatorVote{RecipientAddress: candidate, VoteType: voteType})

		if _, err := engine.Verify(block); err != istanbulcommon.ErrInvalidVote {
			t.Errorf("vote type 0x%02x: have %v, want %v", voteType, err, istanbulcommon.ErrInvalidVote)
		}
	}
}

// TestQBFTVerifyAcceptsValidVotes guards against the fix over-rejecting: a nil
// vote (the common case, since Prepare only writes one when a candidate is
// pending) and both legal vote types must still verify.
func TestQBFTVerifyAcceptsValidVotes(t *testing.T) {
	genesis, nodeKeys := testutils.GenesisAndKeys(4)
	config := copyConfig(istanbul.DefaultConfig)
	chain, engine := newBlockchainFromConfig(genesis, nodeKeys, config)
	defer engine.Stop()

	candidate := common.HexToAddress("0x000000000000000000000000000000000000dead")

	votes := []*types.ValidatorVote{
		nil,
		{RecipientAddress: candidate, VoteType: types.QBFTAuthVote},
		{RecipientAddress: candidate, VoteType: types.QBFTDropVote},
		// A drop vote for the zero address is what ReadVote synthesizes for a
		// nil vote, so it must remain acceptable.
		{RecipientAddress: common.Address{}, VoteType: types.QBFTDropVote},
	}

	for i, vote := range votes {
		block := makeBlockWithoutSeal(chain, engine, chain.Genesis(), true)
		block = setVote(t, block, vote)

		// ErrEmptyCommittedSeals is expected and ignored by Verify, so a nil
		// error is the success condition here.
		if _, err := engine.Verify(block); err != nil {
			t.Errorf("vote %d: unexpected rejection: %v", i, err)
		}
	}
}

// TestQBFTInsertChainRejectsInvalidVoteType covers the import path: even with a
// valid 2F+1 committed-seal quorum, a block with a malformed vote must never
// reach the canonical chain. This is the regression that matters most -- before
// the fix such a block became the head and then wedged every node, since no
// successor could be produced or verified.
func TestQBFTInsertChainRejectsInvalidVoteType(t *testing.T) {
	genesis, nodeKeys := testutils.GenesisAndKeys(4)
	config := copyConfig(istanbul.DefaultConfig)
	chain, engine := newBlockchainFromConfig(genesis, nodeKeys, config)
	defer engine.Stop()

	keys := nodeKeys[:3]

	block := makeBlockWithoutSeal(chain, engine, chain.Genesis(), true)
	block = setVote(t, block, &types.ValidatorVote{
		RecipientAddress: common.HexToAddress("0x000000000000000000000000000000000000dead"),
		VoteType:         0x42,
	})
	committed := commitBlock(t, engine, block, keys)

	if _, err := chain.InsertChain(types.Blocks{committed}); err == nil {
		t.Fatal("InsertChain accepted a block with an invalid vote type")
	}

	if head := chain.CurrentBlock(); head.Hash() == committed.Hash() {
		t.Fatal("block with invalid vote type became the canonical head")
	}
	if head := chain.CurrentBlock(); head.Number.Uint64() != 0 {
		t.Fatalf("canonical head advanced past genesis: got %d", head.Number.Uint64())
	}
}

// TestQBFTValidVoteBlockStillImports proves the fix does not block legitimate
// validator voting: a block carrying a well-formed vote must still import and
// become canonical, and the chain must still be able to build on it.
func TestQBFTValidVoteBlockStillImports(t *testing.T) {
	genesis, nodeKeys := testutils.GenesisAndKeys(4)
	config := copyConfig(istanbul.DefaultConfig)
	chain, engine := newBlockchainFromConfig(genesis, nodeKeys, config)
	defer engine.Stop()

	keys := nodeKeys[:3]

	block := makeBlockWithoutSeal(chain, engine, chain.Genesis(), true)
	block = setVote(t, block, &types.ValidatorVote{
		RecipientAddress: common.HexToAddress("0x000000000000000000000000000000000000dead"),
		VoteType:         types.QBFTAuthVote,
	})
	committed := commitBlock(t, engine, block, keys)

	if _, err := chain.InsertChain(types.Blocks{committed}); err != nil {
		t.Fatalf("InsertChain rejected a block with a valid vote: %v", err)
	}
	if head := chain.CurrentBlock(); head.Hash() != committed.Hash() {
		t.Fatal("block with valid vote did not become canonical")
	}

	// The whole point of the fix is that the head never poisons the snapshot,
	// so preparing the next block must succeed.
	nextHeader := makeHeader(committed, engine.config)
	nextHeader.Coinbase = engine.Address()
	if err := engine.Prepare(chain, nextHeader); err != nil {
		t.Fatalf("Prepare on top of a valid-vote head failed: %v", err)
	}
}
