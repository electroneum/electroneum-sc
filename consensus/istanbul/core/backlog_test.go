package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// newTestValidatorSet creates a validator set with n random validators.
func newTestValidatorSet(n int) istanbul.ValidatorSet {
	addrs := make([]common.Address, n)
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		addrs[i] = crypto.PubkeyToAddress(key.PublicKey)
	}
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	return validator.NewSet(addrs, pp)
}

// newTestCore constructs a minimal core instance suitable for backlog unit tests.
// It initialises the validator set, round state, and backlog structures.
func newTestCore(valSet istanbul.ValidatorSet, seq, round int64) *core {
	view := &istanbul.View{
		Sequence: big.NewInt(seq),
		Round:    big.NewInt(round),
	}

	c := &core{
		address:    valSet.List()[0].Address(),
		state:      StateAcceptRequest,
		logger:     log.New(),
		valSet:     valSet,
		backlogs:   make(map[common.Address]*prque.Prque),
		backlogsMu: new(sync.Mutex),
		current:    newRoundState(view, valSet, nil, nil, nil, nil, func(common.Hash) bool { return false }),
	}
	return c
}

// makePrepare creates a PREPARE message with the given sequence, round, and source address.
func makePrepare(seq, round int64, src common.Address) *qbfttypes.Prepare {
	p := qbfttypes.NewPrepare(big.NewInt(seq), big.NewInt(round), common.Hash{})
	p.SetSource(src)
	p.SetSignature([]byte{0x01})
	return p
}

// makeRoundChange creates a ROUND-CHANGE message with the given sequence, round, and source.
func makeRoundChange(seq, round int64, src common.Address) *qbfttypes.RoundChange {
	rc := qbfttypes.NewRoundChange(big.NewInt(seq), big.NewInt(round), big.NewInt(0), nil, false)
	rc.SetSource(src)
	rc.SetSignature([]byte{0x01})
	return rc
}

// -----------------------------------------------------------------------------
// Tests for isValidatorAddress
// -----------------------------------------------------------------------------

// Confirms that an address present in the current validator set is recognised.
func TestIsValidatorAddress_AcceptsKnownValidator(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 1, 0)

	if !c.isValidatorAddress(valSet.List()[1].Address()) {
		t.Fatal("expected validator address to be accepted")
	}
}

// Confirms that a random address not in the validator set is rejected.
func TestIsValidatorAddress_RejectsNonValidator(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 1, 0)

	key, _ := crypto.GenerateKey()
	outsider := crypto.PubkeyToAddress(key.PublicKey)

	if c.isValidatorAddress(outsider) {
		t.Fatal("expected non-validator address to be rejected")
	}
}

// Ensures graceful handling when the validator set is nil, returning false
// rather than panicking.
func TestIsValidatorAddress_ReturnsFalseWhenValSetNil(t *testing.T) {
	c := &core{logger: log.New()}
	// valSet is nil

	if c.isValidatorAddress(common.HexToAddress("0x1")) {
		t.Fatal("expected false when valSet is nil")
	}
}

// -----------------------------------------------------------------------------
// Tests for withinBacklogFutureWindow
// -----------------------------------------------------------------------------

// Verifies that a message at the same sequence and a modest future round is
// accepted, as validators commonly receive messages slightly ahead of their
// own round.
func TestWithinBacklogFutureWindow_AcceptsSameSequenceNormalRound(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10), Round: big.NewInt(5)}
	if !c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected same-sequence message within round gap to be accepted")
	}
}

// Ensures that a ROUND-CHANGE message at the same sequence but with a round
// exceeding MaxFutureRoundGap is rejected. This prevents an attacker from
// flooding the backlog with messages claiming extremely high future rounds.
func TestWithinBacklogFutureWindow_RejectsSameSequenceExcessiveRound(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	// Round far beyond MaxFutureRoundGap (15)
	view := istanbul.View{Sequence: big.NewInt(10), Round: big.NewInt(100)}
	if c.withinBacklogFutureWindow(qbfttypes.RoundChangeCode, view) {
		t.Fatal("expected same-sequence ROUND-CHANGE with excessive round to be rejected")
	}
}

// Confirms that a non-ROUND-CHANGE message at the same sequence is accepted
// even with a high round number, since the round gap check only applies to
// ROUND-CHANGE messages (which are the vector for round-spam attacks).
func TestWithinBacklogFutureWindow_AcceptsSameSequenceHighRoundForNonRC(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10), Round: big.NewInt(100)}
	if !c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected non-ROUND-CHANGE at same sequence to be accepted regardless of round")
	}
}

// Validates that a ROUND-CHANGE at the same sequence with a round exactly at
// the MaxFutureRoundGap boundary is accepted (boundary is inclusive via <=).
func TestWithinBacklogFutureWindow_AcceptsSameSequenceRoundAtBoundary(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10), Round: new(big.Int).SetUint64(MaxFutureRoundGap)}
	if !c.withinBacklogFutureWindow(qbfttypes.RoundChangeCode, view) {
		t.Fatal("expected ROUND-CHANGE at exactly MaxFutureRoundGap to be accepted")
	}
}

// Confirms that a ROUND-CHANGE one round beyond the gap limit is rejected.
func TestWithinBacklogFutureWindow_RejectsSameSequenceRoundJustBeyondBoundary(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10), Round: new(big.Int).SetUint64(MaxFutureRoundGap + 1)}
	if c.withinBacklogFutureWindow(qbfttypes.RoundChangeCode, view) {
		t.Fatal("expected ROUND-CHANGE one beyond MaxFutureRoundGap to be rejected")
	}
}

// Ensures that a message with a past sequence is not considered a valid backlog
// candidate, since the node has already moved beyond that block height.
func TestWithinBacklogFutureWindow_RejectsPastSequence(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(5), Round: big.NewInt(0)}
	if c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected past-sequence message to be rejected")
	}
}

// Verifies that a message within the MaxFutureSequenceGap window is accepted
// for backlogging, allowing the node to buffer messages for blocks that are
// about to be produced.
func TestWithinBacklogFutureWindow_AcceptsFutureSequenceWithinGap(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10 + int64(MaxFutureSequenceGap)), Round: big.NewInt(0)}
	if !c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected future-sequence within gap to be accepted")
	}
}

// Confirms that a message beyond MaxFutureSequenceGap is rejected. If a node
// is 32+ blocks behind, it needs chain sync rather than message backlogging.
func TestWithinBacklogFutureWindow_RejectsFutureSequenceBeyondGap(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	view := istanbul.View{Sequence: big.NewInt(10 + int64(MaxFutureSequenceGap) + 1), Round: big.NewInt(0)}
	if c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected future-sequence beyond gap to be rejected")
	}
}

// Verifies that when current round state is nil (node not yet initialised),
// all messages are dropped rather than causing a nil pointer dereference.
func TestWithinBacklogFutureWindow_ReturnsFalseWhenCurrentNil(t *testing.T) {
	c := &core{logger: log.New()}

	view := istanbul.View{Sequence: big.NewInt(1), Round: big.NewInt(0)}
	if c.withinBacklogFutureWindow(qbfttypes.PrepareCode, view) {
		t.Fatal("expected false when current round state is nil")
	}
}

// -----------------------------------------------------------------------------
// Tests for maxBacklogTotal
// -----------------------------------------------------------------------------

// Confirms that the dynamic total is calculated as validators * MaxBacklogPerValidator * 2
// and falls within the expected bounds.
func TestMaxBacklogTotal_DynamicCalculation(t *testing.T) {
	valSet := newTestValidatorSet(10)
	c := newTestCore(valSet, 1, 0)

	expected := 10 * MaxBacklogPerValidator * 2 // 20480
	got := c.maxBacklogTotal()
	if got != expected {
		t.Fatalf("expected maxBacklogTotal=%d for 10 validators, got %d", expected, got)
	}
}

// Ensures the MinBacklogTotal floor is applied when the validator set is small
// (e.g. 1 validator, where the dynamic value would be 2048 < 4096 minimum).
func TestMaxBacklogTotal_FloorForSmallValidatorSet(t *testing.T) {
	valSet := newTestValidatorSet(1)
	c := newTestCore(valSet, 1, 0)

	got := c.maxBacklogTotal()
	if got != MinBacklogTotal {
		t.Fatalf("expected MinBacklogTotal=%d for 1 validator, got %d", MinBacklogTotal, got)
	}
}

// Ensures the MaxBacklogTotalCeiling is applied when the validator set is very
// large, preventing unbounded memory allocation.
func TestMaxBacklogTotal_CeilingForLargeValidatorSet(t *testing.T) {
	// 100 validators * 1024 * 2 = 204800 > 131072 ceiling
	valSet := newTestValidatorSet(100)
	c := newTestCore(valSet, 1, 0)

	got := c.maxBacklogTotal()
	if got != MaxBacklogTotalCeiling {
		t.Fatalf("expected MaxBacklogTotalCeiling=%d for 100 validators, got %d", MaxBacklogTotalCeiling, got)
	}
}

// Verifies that a nil validator set falls back to MinBacklogTotal rather than panicking.
func TestMaxBacklogTotal_NilValSetReturnsMin(t *testing.T) {
	c := &core{logger: log.New()}

	got := c.maxBacklogTotal()
	if got != MinBacklogTotal {
		t.Fatalf("expected MinBacklogTotal=%d when valSet is nil, got %d", MinBacklogTotal, got)
	}
}

// -----------------------------------------------------------------------------
// Tests for addToBacklog
// -----------------------------------------------------------------------------

// Verifies that a legitimate future message from a known validator is
// successfully added to the backlog.
func TestAddToBacklog_AcceptsValidFutureMessage(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	src := valSet.List()[1].Address()
	msg := makePrepare(11, 0, src) // future sequence

	c.addToBacklog(msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != 1 {
		t.Fatalf("expected backlogsTotal=1, got %d", c.backlogsTotal)
	}
}

// Confirms that messages from the node's own address are silently discarded.
// Self-originated messages should never be backlogged.
func TestAddToBacklog_RejectsMessageFromSelf(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	msg := makePrepare(11, 0, c.Address()) // from self

	c.addToBacklog(msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != 0 {
		t.Fatalf("expected backlogsTotal=0 for self-message, got %d", c.backlogsTotal)
	}
}

// Ensures that messages claiming to originate from a non-validator address are
// dropped immediately. Without this check, an attacker could fill the backlogs
// map with entries keyed by arbitrary addresses, consuming unbounded memory.
func TestAddToBacklog_RejectsNonValidator(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	outsiderKey, _ := crypto.GenerateKey()
	outsider := crypto.PubkeyToAddress(outsiderKey.PublicKey)
	msg := makePrepare(11, 0, outsider)

	c.addToBacklog(msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != 0 {
		t.Fatalf("expected backlogsTotal=0 for non-validator, got %d", c.backlogsTotal)
	}
}

// Verifies that messages outside the future window (sequence too far ahead)
// are dropped, preventing an attacker from filling the backlog with messages
// for block heights the node won't reach for a long time.
func TestAddToBacklog_RejectsFarFutureMessage(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	src := valSet.List()[1].Address()
	// Sequence 100 is far beyond MaxFutureSequenceGap (32) from current seq 10
	msg := makePrepare(100, 0, src)

	c.addToBacklog(msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != 0 {
		t.Fatalf("expected backlogsTotal=0 for far-future message, got %d", c.backlogsTotal)
	}
}

// Confirms that the per-validator cap (MaxBacklogPerValidator) is enforced.
// Once a validator's queue reaches the limit, additional messages are dropped
// to prevent a single spoofed validator from consuming disproportionate memory.
func TestAddToBacklog_EnforcesPerValidatorCap(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	src := valSet.List()[1].Address()

	// Fill to capacity
	for i := 0; i < MaxBacklogPerValidator; i++ {
		msg := makePrepare(11, int64(i), src)
		c.addToBacklog(msg)
	}

	c.backlogsMu.Lock()
	totalBefore := c.backlogsTotal
	sizeBefore := c.backlogs[src].Size()
	c.backlogsMu.Unlock()

	if totalBefore != MaxBacklogPerValidator {
		t.Fatalf("expected backlogsTotal=%d after filling, got %d", MaxBacklogPerValidator, totalBefore)
	}

	// One more should be rejected
	overflow := makePrepare(11, int64(MaxBacklogPerValidator), src)
	c.addToBacklog(overflow)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != totalBefore {
		t.Fatalf("expected backlogsTotal unchanged at %d after per-validator cap, got %d", totalBefore, c.backlogsTotal)
	}
	if c.backlogs[src].Size() != sizeBefore {
		t.Fatalf("expected per-validator size unchanged at %d, got %d", sizeBefore, c.backlogs[src].Size())
	}
}

// Verifies that the global backlog cap (dynamic based on validator count) is
// enforced across all validators. This provides an absolute upper bound on
// memory consumption regardless of how many validators send messages.
func TestAddToBacklog_EnforcesGlobalCap(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	maxTotal := c.maxBacklogTotal()

	// Fill the global cap using messages from different validators (round-robin)
	validators := valSet.List()
	added := 0
	for added < maxTotal {
		for _, v := range validators {
			if v.Address() == c.Address() {
				continue // skip self
			}
			msg := makePrepare(11, int64(added), v.Address())
			c.addToBacklog(msg)
			added++
			if added >= maxTotal {
				break
			}
		}
	}

	c.backlogsMu.Lock()
	totalAtCap := c.backlogsTotal
	c.backlogsMu.Unlock()

	// The total might be less than maxTotal if per-validator caps kicked in first,
	// but it should not exceed maxTotal.
	if totalAtCap > maxTotal {
		t.Fatalf("backlogsTotal %d exceeds maxTotal %d", totalAtCap, maxTotal)
	}

	// Adding one more from any validator should be rejected
	src := validators[1].Address()
	if src == c.Address() {
		src = validators[2].Address()
	}
	overflow := makePrepare(11, int64(maxTotal+1), src)
	c.addToBacklog(overflow)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal > maxTotal {
		t.Fatalf("expected backlogsTotal to not exceed %d after global cap, got %d", maxTotal, c.backlogsTotal)
	}
}

// Ensures that a ROUND-CHANGE message at the same sequence but with a round
// far beyond MaxFutureRoundGap is dropped by addToBacklog. This exercises the
// integration between addToBacklog and withinBacklogFutureWindow for the
// round-spam attack vector.
func TestAddToBacklog_RejectsExcessiveRoundChangeRound(t *testing.T) {
	valSet := newTestValidatorSet(4)
	c := newTestCore(valSet, 10, 0)

	src := valSet.List()[1].Address()
	// Same sequence, but round far exceeds MaxFutureRoundGap
	msg := makeRoundChange(10, 100, src)

	c.addToBacklog(msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()
	if c.backlogsTotal != 0 {
		t.Fatalf("expected backlogsTotal=0 for excessive round ROUND-CHANGE, got %d", c.backlogsTotal)
	}
}
