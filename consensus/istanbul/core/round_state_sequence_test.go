package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
)

// The core's round state has to stay with its sequence. The chain head can move
// on before the core hears about it, and a round timeout or round jump in that
// window catches the core up to the next sequence while asking for a later
// round. That used to keep the old sequence's ROUND-CHANGEs (prepared blocks
// included) and send a ROUND-CHANGE for a round of the new sequence that never
// timed out. A round timer that fired as the core moved on could also cut the
// next sequence's first round short.

// catchUpBackend has a chain head the core may not have caught up with yet, and
// records the codes of what it broadcasts.
type catchUpBackend struct {
	viewBindingBackend
	head       istanbul.Proposal
	valSet     istanbul.ValidatorSet
	broadcasts []uint64
}

func (b *catchUpBackend) LastProposal() (istanbul.Proposal, common.Address) {
	return b.head, common.Address{}
}
func (b *catchUpBackend) Validators(istanbul.Proposal) istanbul.ValidatorSet { return b.valSet }
func (b *catchUpBackend) Broadcast(_ istanbul.ValidatorSet, code uint64, _ []byte) error {
	b.broadcasts = append(b.broadcasts, code)
	return nil
}

// newCatchUpCore returns a core at (seq, round) on a chain whose head is head,
// with the other validators' addresses.
func newCatchUpCore(t *testing.T, seq, round, head int64) (*core, *catchUpBackend, []common.Address) {
	t.Helper()
	c := newRoundChangeTestCore(4, seq, round)
	backend := &catchUpBackend{head: makeProposalBlock(t, head), valSet: c.valSet}
	backend.addr = c.address
	c.backend = backend
	c.config = istanbul.DefaultConfig
	c.currentMutex = new(sync.Mutex)
	t.Cleanup(c.stopTimer)
	var others []common.Address
	for _, v := range c.valSet.List() {
		if v.Address() != c.address {
			others = append(others, v.Address())
		}
	}
	return c, backend, others
}

func TestCatchUpLeavesOldSequenceBehind(t *testing.T) {
	const seq = 5
	for name, catchUp := range map[string]func(t *testing.T, c *core, rc func(common.Address) *qbfttypes.RoundChange, others []common.Address){
		// One ROUND-CHANGE for the next round arrives, then our timer fires.
		"round timeout": func(t *testing.T, c *core, rc func(common.Address) *qbfttypes.RoundChange, others []common.Address) {
			if err := c.handleRoundChange(rc(others[0])); err != nil {
				t.Fatal(err)
			}
			c.handleTimeoutMsg()
		},
		// F+1 ROUND-CHANGEs for the next round arrive and we jump to it.
		"round jump": func(t *testing.T, c *core, rc func(common.Address) *qbfttypes.RoundChange, others []common.Address) {
			for _, src := range others[:c.valSet.F()+1] {
				if err := c.handleRoundChange(rc(src)); err != nil {
					t.Fatal(err)
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			// We're in round 1 of seq, but the chain already has block seq.
			c, backend, others := newCatchUpCore(t, seq, 1, seq)
			prepared := makeProposalBlock(t, seq)
			prepares := makeAttackPrepares(append([]common.Address{c.address}, others...), c.QuorumSize(), seq, 0, prepared.Hash())
			c.QBFTPreparedPrepares = prepares
			// Round-2 ROUND-CHANGEs for seq, carrying the block prepared in round 0.
			rc := func(src common.Address) *qbfttypes.RoundChange {
				m := qbfttypes.NewRoundChange(big.NewInt(seq), big.NewInt(2), big.NewInt(0), prepared, false)
				m.SetSource(src)
				m.SetSignature([]byte{0x01})
				m.Justification = prepares
				return m
			}

			catchUp(t, c, rc, others)

			if have := c.currentView(); have.Sequence.Int64() != seq+1 || have.Round.Int64() != 0 {
				t.Fatalf("view %v, want round 0 of sequence %d", have, seq+1)
			}
			if n := c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(2)); n != 0 {
				t.Errorf("kept %d ROUND-CHANGEs from sequence %d", n, seq)
			}
			if _, block := c.highestPrepared(big.NewInt(2)); block != nil {
				t.Errorf("kept a prepared block at height %d for sequence %d", block.Number(), seq+1)
			}
			if c.QBFTPreparedPrepares != nil {
				t.Error("kept the PREPAREs we prepared on in the last sequence")
			}
			if len(backend.broadcasts) != 0 {
				t.Errorf("broadcast %v after catching up, want nothing", backend.broadcasts)
			}

			// A ROUND-CHANGE for the new sequence mustn't find enough old ones
			// to jump rounds with.
			if err := c.handleRoundChange(makeRoundChange(seq+1, 0, others[2])); err != nil {
				t.Fatal(err)
			}
			if have := c.currentView(); have.Round.Int64() != 0 {
				t.Errorf("jumped to round %v of sequence %d on old ROUND-CHANGEs", have.Round, seq+1)
			}
		})
	}
}

// A timeout from the last sequence is ignored, and one for the round we're in
// still changes round.
func TestHandleTimeout_OnlyForCurrentSequence(t *testing.T) {
	const seq = 6
	c, backend, _ := newCatchUpCore(t, seq, 0, seq-1)

	c.handleTimeout(timeoutEvent{sequence: seq - 1, round: 0})
	if have := c.currentView(); have.Sequence.Int64() != seq || have.Round.Int64() != 0 {
		t.Fatalf("timeout from sequence %d moved us to %v", seq-1, have)
	}
	if len(backend.broadcasts) != 0 {
		t.Fatalf("timeout from sequence %d broadcast %v", seq-1, backend.broadcasts)
	}

	c.handleTimeout(timeoutEvent{sequence: seq, round: 0})
	if have := c.currentView(); have.Sequence.Int64() != seq || have.Round.Int64() != 1 {
		t.Fatalf("timeout for the current round left us at %v", have)
	}
	if len(backend.broadcasts) != 1 || backend.broadcasts[0] != qbfttypes.RoundChangeCode {
		t.Fatalf("timeout for the current round broadcast %v, want one ROUND-CHANGE", backend.broadcasts)
	}
}
