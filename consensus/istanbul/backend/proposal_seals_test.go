package backend

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbftengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/testutils"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/rlp"
)

// A ROUND-CHANGE's prepared block is tied to the signed digest by its hash
// alone, which leaves out the proposer seal, the committed seals and the
// round. VerifyProposalSeals must accept exactly the seals Verify accepts,
// so a prepared block that passes it can always be re-proposed, while still
// letting a block marked bad through for the bad-proposal round change.
func TestVerifyProposalSeals(t *testing.T) {
	for _, fork := range []bool{false, true} {
		key, _ := crypto.GenerateKey()
		other, _ := crypto.GenerateKey()
		genesis := testutils.Genesis([]common.Address{crypto.PubkeyToAddress(key.PublicKey)})
		cfg := *genesis.Config
		if fork {
			cfg.FutureForkBlock = big.NewInt(0)
		}
		genesis.Config = &cfg
		chain, backend := newBlockchainFromConfig(genesis, []*ecdsa.PrivateKey{key}, copyConfig(istanbul.DefaultConfig))

		snap, err := backend.snapshot(chain, 0, chain.Genesis().Hash(), nil)
		if err != nil {
			t.Fatal(err)
		}
		engine := backend.EngineForBlockNumber(big.NewInt(1))
		proposal, err := engine.Seal(chain, makeBlockWithoutSeal(chain, backend, chain.Genesis(), true), snap.ValSet)
		if err != nil {
			t.Fatal(err)
		}
		rewrite := func(mutate func(*types.QBFTExtra)) *types.Block {
			header := proposal.Header()
			extra, err := types.ExtractQBFTExtra(header)
			if err != nil {
				t.Fatal(err)
			}
			mutate(extra)
			if header.Extra, err = rlp.EncodeToBytes(extra); err != nil {
				t.Fatal(err)
			}
			return proposal.WithSeal(header)
		}
		sign := func(k *ecdsa.PrivateKey, hash []byte) []byte {
			sig, err := crypto.Sign(hash, k)
			if err != nil {
				t.Fatal(err)
			}
			return sig
		}
		junk := bytes.Repeat([]byte{0xee}, types.IstanbulExtraSeal)
		committed := func(round uint32) func(*types.QBFTExtra) {
			return func(e *types.QBFTExtra) {
				e.Round = round
				h := proposal.Header()
				e.CommittedSeal = [][]byte{sign(key, qbftengine.PrepareCommittedSeal(h, round))}
			}
		}
		cases := []struct {
			name  string
			block *types.Block
			valid bool
		}{
			{"genuine", proposal, true},
			{"junk proposer seal", rewrite(func(e *types.QBFTExtra) { e.ProposerSeal = junk }), false},
			{"proposer seal by a non-validator", rewrite(func(e *types.QBFTExtra) {
				h := proposal.Header()
				e.ProposerSeal = sign(other, crypto.Keccak256(mustEncodeFiltered(t, h)))
			}), false},
			{"junk committed seal", rewrite(func(e *types.QBFTExtra) { e.CommittedSeal = [][]byte{junk} }), false},
			{"valid committed seals", rewrite(committed(2)), true},
			{"round", rewrite(func(e *types.QBFTExtra) { e.Round = 9 }), true},
		}
		if fork {
			cases = append(cases, struct {
				name  string
				block *types.Block
				valid bool
			}{"no proposer seal", rewrite(func(e *types.QBFTExtra) { e.ProposerSeal = nil }), false})
		}
		for _, tc := range cases {
			if tc.block.Hash() != proposal.Hash() {
				t.Fatalf("fork=%v %s: rewriting the seals changed the hash", fork, tc.name)
			}
			_, verr := backend.Verify(tc.block)
			serr := backend.VerifyProposalSeals(tc.block)
			if (verr == nil) != tc.valid {
				t.Errorf("fork=%v %s: Verify returned %v, want valid=%v", fork, tc.name, verr, tc.valid)
			}
			if (serr == nil) != tc.valid {
				t.Errorf("fork=%v %s: VerifyProposalSeals returned %v, want valid=%v", fork, tc.name, serr, tc.valid)
			}
		}
		// A prepared block can be one that later went bad. Verify refuses it,
		// but its ROUND-CHANGE is how the others find out, so the seal check
		// must still pass it.
		rawdb.WriteBadBlock(backend.db, proposal)
		if _, err := backend.Verify(proposal); err == nil {
			t.Errorf("fork=%v: Verify accepted a bad block", fork)
		}
		if err := backend.VerifyProposalSeals(proposal); err != nil {
			t.Errorf("fork=%v: VerifyProposalSeals rejected a bad block with valid seals: %v", fork, err)
		}
		backend.Stop()
	}
}

// mustEncodeFiltered is the RLP of the header the proposer seal signs.
func mustEncodeFiltered(t *testing.T, h *types.Header) []byte {
	enc, err := rlp.EncodeToBytes(types.QBFTFilteredHeader(h))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}
