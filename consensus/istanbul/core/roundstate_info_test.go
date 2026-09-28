package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/consensus/istanbul"
)

// newRoundStateInfoTestCore builds a core with the fields publishRoundState
// reads: a round state, a validator set and a round change set.
func newRoundStateInfoTestCore(valSetSize int, seq, round int64) *core {
	c := newRoundChangeTestCore(valSetSize, seq, round)
	c.currentMutex = new(sync.Mutex)
	return c
}

// Nothing is published until there is a round state to describe.
func TestRoundState_NilBeforeFirstPublish(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	if got := c.RoundState(); got != nil {
		t.Fatalf("RoundState before any publish = %+v, want nil", got)
	}

	c.current = nil
	c.publishRoundState()
	if got := c.RoundState(); got != nil {
		t.Fatalf("publish without a round state stored %+v, want nothing", got)
	}
}

// A published snapshot reflects the round state, validator set and step.
func TestRoundState_ReflectsCurrentState(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 2)
	c.setStateForTest(StatePrepared)

	src := c.valSet.List()[1].Address()
	if err := c.current.QBFTPrepares.Add(makePrepare(10, 2, src)); err != nil {
		t.Fatalf("adding prepare: %v", err)
	}
	c.publishRoundState()

	got := c.RoundState()
	if got == nil {
		t.Fatal("RoundState is nil after publish")
	}
	if got.Sequence != 10 || got.Round != 2 {
		t.Errorf("sequence/round = %d/%d, want 10/2", got.Sequence, got.Round)
	}
	if got.State != StatePrepared.String() {
		t.Errorf("state = %q, want %q", got.State, StatePrepared.String())
	}
	if got.Validators != 4 || got.QuorumSize != 3 {
		t.Errorf("validators/quorum = %d/%d, want 4/3", got.Validators, got.QuorumSize)
	}
	if got.Prepares != 1 || got.Commits != 0 {
		t.Errorf("prepares/commits = %d/%d, want 1/0", got.Prepares, got.Commits)
	}
	if got.HasProposal {
		t.Error("hasProposal = true with no PRE-PREPARE accepted")
	}
	if got.PreparedRound != nil {
		t.Errorf("preparedRound = %d, want nil (no lock)", *got.PreparedRound)
	}
	if got.RoundStartedAt == 0 || got.UpdatedAt < got.RoundStartedAt {
		t.Errorf("timestamps roundStartedAt=%d updatedAt=%d are not set sensibly", got.RoundStartedAt, got.UpdatedAt)
	}
}

// A lock on a prepared block is reported.
func TestRoundState_ReportsPreparedRound(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 3)
	c.current.preparedRound = big.NewInt(1)
	c.publishRoundState()

	got := c.RoundState()
	if got.PreparedRound == nil || *got.PreparedRound != 1 {
		t.Fatalf("preparedRound = %v, want 1", got.PreparedRound)
	}
}

// ROUND-CHANGE messages are counted per target round; rounds below the
// current one are left out.
func TestRoundState_CountsRoundChangesAtOrAboveCurrentRound(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 1)
	vals := c.valSet.List()

	// Held messages are placed in the set directly: sending them through
	// handleRoundChange would reach F+1 for round 2 and move the node to that
	// round, which needs a full backend. Round 0 is a stale entry below the
	// current round, as if left over from before the round change.
	for _, rc := range []struct {
		round uint64
		src   int
	}{{0, 3}, {1, 1}, {2, 1}, {2, 2}} {
		if c.roundChangeSet.roundChanges[rc.round] == nil {
			c.roundChangeSet.roundChanges[rc.round] = newQBFTMsgSet(c.valSet)
		}
		msg := makeRoundChange(10, int64(rc.round), vals[rc.src].Address())
		if err := c.roundChangeSet.roundChanges[rc.round].Add(msg); err != nil {
			t.Fatalf("holding round change for round %d: %v", rc.round, err)
		}
	}

	c.publishRoundState()
	got := c.RoundState().RoundChanges
	if got[1] != 1 || got[2] != 2 {
		t.Errorf("roundChanges = %v, want round 1 -> 1 and round 2 -> 2", got)
	}
	if _, ok := got[0]; ok {
		t.Errorf("roundChanges = %v includes round 0, below the current round", got)
	}

	// A round with an empty slot (the set pre-creates one for the current
	// round) is left out rather than reported as zero.
	c.roundChangeSet.roundChanges[3] = newQBFTMsgSet(c.valSet)
	c.publishRoundState()
	if n, ok := c.RoundState().RoundChanges[3]; ok {
		t.Errorf("roundChanges reports empty round 3 as %d; want it left out", n)
	}
}

// RoundStartedAt stays put while the node stays in the same sequence and
// round, and moves when either changes.
func TestRoundState_RoundStartedAtFollowsTheRound(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	c.roundStateInfo.Store(&istanbul.RoundStateInfo{Sequence: 10, Round: 0, RoundStartedAt: 1})

	c.publishRoundState()
	if got := c.RoundState().RoundStartedAt; got != 1 {
		t.Fatalf("same round: roundStartedAt = %d, want the earlier 1", got)
	}

	c.current.SetRound(big.NewInt(1))
	c.publishRoundState()
	if got := c.RoundState().RoundStartedAt; got <= 1 {
		t.Fatalf("new round: roundStartedAt = %d, want the current time", got)
	}

	c.roundStateInfo.Store(&istanbul.RoundStateInfo{Sequence: 10, Round: 1, RoundStartedAt: 1})
	c.current.SetSequence(big.NewInt(11))
	c.publishRoundState()
	if got := c.RoundState().RoundStartedAt; got <= 1 {
		t.Fatalf("new sequence: roundStartedAt = %d, want the current time", got)
	}
}

// Snapshots handed out earlier are never modified by later publishes.
func TestRoundState_SnapshotsAreImmutable(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	c.publishRoundState()
	first := c.RoundState()

	c.current.SetRound(big.NewInt(5))
	c.publishRoundState()

	if first.Round != 0 {
		t.Fatalf("earlier snapshot changed: round = %d, want 0", first.Round)
	}
	if c.RoundState().Round != 5 {
		t.Fatalf("latest snapshot round = %d, want 5", c.RoundState().Round)
	}
}

// setStateForTest sets the step without the side effects of setState
// (pending request and backlog processing), which need more wiring.
func (c *core) setStateForTest(s State) { c.state = s }
