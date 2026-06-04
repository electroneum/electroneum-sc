package core

import (
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/log"
)

// newRoundChangeTestCore builds a core wired for handleRoundChange tests:
// it adds the cleanLogger and roundChangeSet that newTestCore leaves unset.
func newRoundChangeTestCore(valSetSize int, seq, round int64) *core {
	valSet := newTestValidatorSet(valSetSize)
	c := newTestCore(valSet, seq, round)
	c.cleanLogger = log.New()
	c.roundChangeSet = newRoundChangeSet(valSet)
	c.roundChangeSet.NewRound(big.NewInt(round))
	return c
}

// roundChangeSetEntries reports how many distinct round slots are currently
// allocated in the round change set. This is the quantity an attacker tries to
// grow without bound.
func roundChangeSetEntries(c *core) int {
	c.roundChangeSet.mu.Lock()
	defer c.roundChangeSet.mu.Unlock()
	return len(c.roundChangeSet.roundChanges)
}

// TestHandleRoundChange_DropsFarFutureRound proves the fix for the ROUND-CHANGE
// flood OOM: a single malicious validator sending ROUND-CHANGE messages with an
// unbounded range of distinct rounds can no longer grow roundChangeSet beyond
// the MaxFutureRoundGap window. Without the cap, each distinct round number
// allocated a new qbftMsgSet with no eviction (cleanup needs F+1 for one round,
// which a single attacker never reaches).
func TestHandleRoundChange_DropsFarFutureRound(t *testing.T) {
	c := newRoundChangeTestCore(4, 5000, 0)
	attacker := c.valSet.List()[1].Address()

	// One ROUND-CHANGE per distinct round, mimicking the flood. The attacker is
	// a single validator, so no round ever reaches F+1 and no cleanup runs.
	const flood = 100000
	for round := int64(0); round < flood; round++ {
		rc := makeRoundChange(5000, round, attacker)
		if err := c.handleRoundChange(rc); err != nil {
			t.Fatalf("handleRoundChange returned error for round %d: %v", round, err)
		}
	}

	// Only rounds within [current, current+MaxFutureRoundGap] may be retained.
	maxEntries := int(MaxFutureRoundGap) + 1
	if got := roundChangeSetEntries(c); got > maxEntries {
		t.Fatalf("roundChangeSet grew to %d entries after %d flood messages; want <= %d",
			got, flood, maxEntries)
	}
}

// TestHandleRoundChange_RejectsRoundBeyondGap checks the exact boundary: a round
// exactly MaxFutureRoundGap ahead is accepted, one beyond is dropped without
// allocating a slot. This matches withinBacklogFutureWindow's boundary so the
// bound is identical on both delivery paths.
func TestHandleRoundChange_RejectsRoundBeyondGap(t *testing.T) {
	c := newRoundChangeTestCore(4, 5000, 0)
	attacker := c.valSet.List()[1].Address()

	atBoundary := int64(MaxFutureRoundGap)
	if err := c.handleRoundChange(makeRoundChange(5000, atBoundary, attacker)); err != nil {
		t.Fatalf("round at boundary should be accepted, got err: %v", err)
	}
	if c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(atBoundary)) != 1 {
		t.Fatalf("round at boundary (%d) should have been stored", atBoundary)
	}

	beyond := int64(MaxFutureRoundGap) + 1
	if err := c.handleRoundChange(makeRoundChange(5000, beyond, attacker)); err != nil {
		t.Fatalf("round beyond boundary should be dropped silently, got err: %v", err)
	}
	if c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(beyond)) != 0 {
		t.Fatalf("round beyond boundary (%d) should not have been stored", beyond)
	}
}

// TestHandleRoundChange_DropsHugeRoundWithoutTruncation guards a latent aliasing
// bug: roundChangeSet.Add keys its map with big.Int.Uint64(), which truncates a
// round larger than 2^64. A round of 2^64+current would otherwise alias onto a
// legitimate low round's slot. The cap rejects it (in full big.Int precision)
// before any truncation occurs.
func TestHandleRoundChange_DropsHugeRoundWithoutTruncation(t *testing.T) {
	c := newRoundChangeTestCore(4, 5000, 0)
	attacker := c.valSet.List()[1].Address()

	// Baseline: the legitimate current round (0) slot is pre-allocated by NewRound.
	before := roundChangeSetEntries(c)

	// 2^64, which silently truncates to 0 via Uint64() inside roundChangeSet.Add.
	// If the guard let this through, it would alias onto round 0's slot rather
	// than allocate a new one, so we also check round 0 still holds zero messages.
	huge := new(big.Int).Lsh(big.NewInt(1), 64)
	rc := makeRoundChange(5000, 0, attacker)
	rc.Round = huge // override with an oversized round

	if err := c.handleRoundChange(rc); err != nil {
		t.Fatalf("oversized round should be dropped silently, got err: %v", err)
	}
	if got := roundChangeSetEntries(c); got != before {
		t.Fatalf("oversized round must not allocate a slot; entries went %d -> %d", before, got)
	}
	if c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(0)) != 0 {
		t.Fatalf("oversized round must not alias onto round 0's message set")
	}
}

// TestHandleRoundChange_AcceptsLegitimateNextRound confirms the cap does not
// break normal consensus: a validator advancing one round at a time is always
// within the window and its messages are stored.
func TestHandleRoundChange_AcceptsLegitimateNextRound(t *testing.T) {
	c := newRoundChangeTestCore(4, 5000, 3)
	peer := c.valSet.List()[1].Address()

	next := int64(4) // current round 3 + 1
	if err := c.handleRoundChange(makeRoundChange(5000, next, peer)); err != nil {
		t.Fatalf("legitimate next-round ROUND-CHANGE should be accepted, got err: %v", err)
	}
	if c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(next)) != 1 {
		t.Fatalf("legitimate next-round ROUND-CHANGE should have been stored")
	}
}
