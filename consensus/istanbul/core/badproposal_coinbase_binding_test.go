package core

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
)

// A PRE-PREPARE in a later round may carry a block with someone else's
// coinbase, but only when it re-proposes a block a quorum already prepared.
// Once a quorum of ROUND-CHANGEs says the prepared block is bad, isJustified
// lets the proposer put up a new block on the PREPAREs for the bad one. That
// used to take the relaxed check too, so the proposer could put another
// validator's address in the coinbase and have its vote counted as theirs.
// coinbaseBackend counts how often a proposal reaches Verify.
type coinbaseBackend struct {
	viewBindingBackend
	verifies int
}

func (b *coinbaseBackend) Verify(istanbul.Proposal) (time.Duration, error) {
	b.verifies++
	return 0, nil
}

func TestHandleEncodedMsg_BadProposalRecoveryBindsCoinbase(t *testing.T) {
	const seq, round, preparedRound = 1, 2, 1

	for name, tc := range map[string]struct {
		// badProposal is whether the ROUND-CHANGEs say the prepared block is bad.
		badProposal bool
		// propose picks the block to propose given the prepared one.
		propose func(prepared *types.Block, proposer, victim common.Address) *types.Block
		// prepareFor picks what the i-th justification PREPARE is for.
		prepareFor func(i int, prepared, proposal *types.Block) common.Hash
		want       error
	}{
		"new block with another validator's coinbase": {
			badProposal: true,
			propose: func(_ *types.Block, _, victim common.Address) *types.Block {
				return blockWithCoinbase(seq, 3, victim)
			},
			prepareFor: func(_ int, prepared, _ *types.Block) common.Hash { return prepared.Hash() },
			want:       errInvalidBlockCoinbase,
		},
		// The proposer signing one PREPARE for its own block doesn't make the
		// rest of the quorum back it.
		"new block with another validator's coinbase and one PREPARE for it": {
			badProposal: true,
			propose: func(_ *types.Block, _, victim common.Address) *types.Block {
				return blockWithCoinbase(seq, 3, victim)
			},
			prepareFor: func(i int, prepared, proposal *types.Block) common.Hash {
				if i == 0 {
					return proposal.Hash()
				}
				return prepared.Hash()
			},
			want: errInvalidBlockCoinbase,
		},
		"new block with the proposer's coinbase": {
			badProposal: true,
			propose: func(_ *types.Block, proposer, _ common.Address) *types.Block {
				return blockWithCoinbase(seq, 3, proposer)
			},
			prepareFor: func(_ int, prepared, _ *types.Block) common.Hash { return prepared.Hash() },
		},
		"re-proposal of the prepared block": {
			propose:    func(prepared *types.Block, _, _ common.Address) *types.Block { return prepared },
			prepareFor: func(_ int, prepared, _ *types.Block) common.Hash { return prepared.Hash() },
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, _, proposer, keys, _ := buildViewBindingCore(t)
			backend := &coinbaseBackend{viewBindingBackend: viewBindingBackend{addr: c.address}}
			c.backend = backend

			var victim common.Address
			for _, v := range c.valSet.List() {
				if v.Address() != proposer {
					victim = v.Address()
					break
				}
			}
			// The block prepared in round 1, proposed by the victim.
			prepared := blockWithCoinbase(seq, 2, victim)
			proposal := tc.propose(prepared, proposer, victim)

			preprepare := qbfttypes.NewPreprepare(big.NewInt(seq), big.NewInt(round), proposal)
			signQBFTMessage(t, preprepare, keys[proposer])
			for i, v := range c.valSet.List()[:c.QuorumSize()] {
				rc := qbfttypes.NewRoundChange(big.NewInt(seq), big.NewInt(round), big.NewInt(preparedRound), prepared, tc.badProposal)
				signQBFTMessage(t, rc, keys[v.Address()])
				preprepare.JustificationRoundChanges = append(preprepare.JustificationRoundChanges, &rc.SignedRoundChangePayload)

				prepare := qbfttypes.NewPrepare(big.NewInt(seq), big.NewInt(preparedRound), tc.prepareFor(i, prepared, proposal))
				signQBFTMessage(t, prepare, keys[v.Address()])
				preprepare.JustificationPrepares = append(preprepare.JustificationPrepares, prepare)
			}
			payload, err := encodePreprepare(t, preprepare)
			if err != nil {
				t.Fatal(err)
			}

			err = c.handleEncodedMsg(qbfttypes.PreprepareCode, payload)
			if !errors.Is(err, tc.want) {
				t.Fatalf("have %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				if c.state != StatePreprepared || c.current.Proposal().Hash() != proposal.Hash() || backend.broadcasts != 1 {
					t.Fatalf("proposal not accepted: state %v, %d broadcasts", c.state, backend.broadcasts)
				}
				return
			}
			if c.state != StateAcceptRequest || c.current.Proposal() != nil {
				t.Error("accepted a block with the victim's coinbase")
			}
			if backend.broadcasts != 0 {
				t.Error("sent a PREPARE for a block with the victim's coinbase")
			}
			if backend.verifies != 0 {
				t.Error("block with the victim's coinbase reached Verify")
			}
		})
	}
}
