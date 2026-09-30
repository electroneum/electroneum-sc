// Copyright 2026 The Electroneum Authors
//
// Regression tests for transition-scoped QBFT timing parameters.
//
// Reported via Bugcrowd: BlockPeriodSeconds and AllowedFutureBlockTime can be
// changed by a params.Transition, but the engine didn't apply them
// consistently. Prepare stamped block N with the period from N's own config
// while verifyCascadingFields checked it against the parent's (N-1), so a
// transition that lowered the period made honest proposers build a block that
// honest validators rejected. verifyHeader also read the static
// AllowedFutureBlockTime, ignoring any transition to it.

package qbftengine

import (
	"math/big"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/params"
)

func newTimingEngine(cfg *istanbul.Config) *Engine {
	return NewEngine(cfg, common.Address{}, func(data []byte) ([]byte, error) {
		return make([]byte, 65), nil
	})
}

func timingParent(ts uint64) *types.Header {
	return &types.Header{
		Number:     big.NewInt(0),
		Time:       ts,
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
	}
}

// TestBlockPeriodTransition_LoweredPeriodAgreesWithPrepare is the reported PoC:
// with the period dropping from 10s to 1s at block 1, the block Prepare builds
// must pass the verifier's timestamp check.
func TestBlockPeriodTransition_LoweredPeriodAgreesWithPrepare(t *testing.T) {
	cfg := &istanbul.Config{
		BlockPeriod:            10,
		AllowedFutureBlockTime: 30,
		Transitions:            []params.Transition{{Block: big.NewInt(1), BlockPeriodSeconds: 1}},
	}
	engine := newTimingEngine(cfg)

	// Parent slightly ahead of wall clock so Prepare never bumps the child's
	// time up to "now", even if a second ticks over mid-test.
	parent := timingParent(uint64(time.Now().Unix()) + 30)
	chain := &gasUsedChainReader{mockChainHeaderReader: mockChainHeaderReader{cfg: &params.ChainConfig{ChainID: big.NewInt(1)}}, parent: parent}
	valSet := validator.NewSet([]common.Address{{0x1}}, istanbul.NewRoundRobinProposerPolicy())

	header := &types.Header{Number: big.NewInt(1), ParentHash: parent.Hash()}
	if err := engine.Prepare(chain, header, valSet); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if header.Time != parent.Time+1 {
		t.Fatalf("Prepare should use the transitioned period: got time %d, want %d", header.Time, parent.Time+1)
	}
	if err := engine.verifyCascadingFields(chain, header, valSet, []*types.Header{parent}); err == istanbulcommon.ErrInvalidTimestamp {
		t.Fatal("verifier rejected the block Prepare built at the transition height")
	}
}

// TestBlockPeriodTransition_RaisedPeriodEnforcedAtTransitionBlock pins the
// other direction: a raised period applies to the transition block itself, so
// a block spaced only by the old period is rejected there.
func TestBlockPeriodTransition_RaisedPeriodEnforcedAtTransitionBlock(t *testing.T) {
	cfg := &istanbul.Config{
		BlockPeriod:            1,
		AllowedFutureBlockTime: 30,
		Transitions:            []params.Transition{{Block: big.NewInt(1), BlockPeriodSeconds: 10}},
	}
	engine := newTimingEngine(cfg)
	parent := timingParent(1)
	chain := &mockChainHeaderReader{cfg: &params.ChainConfig{ChainID: big.NewInt(1)}}

	header := &types.Header{Number: big.NewInt(1), ParentHash: parent.Hash(), Time: parent.Time + 1}
	if err := engine.verifyCascadingFields(chain, header, nil, []*types.Header{parent}); err != istanbulcommon.ErrInvalidTimestamp {
		t.Fatalf("expected ErrInvalidTimestamp for a block under the transitioned period, got %v", err)
	}

	header.Time = parent.Time + 10
	if err := engine.verifyCascadingFields(chain, header, nil, []*types.Header{parent}); err == istanbulcommon.ErrInvalidTimestamp {
		t.Fatal("block spaced by the transitioned period was rejected")
	}
}

func futureHeader(t *testing.T, ts uint64) *types.Header {
	t.Helper()
	h := &types.Header{
		Number:     big.NewInt(0),
		Time:       ts,
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
	}
	if err := ApplyHeaderQBFTExtra(h, WriteValidators(nil), writeRoundNumber(big.NewInt(0))); err != nil {
		t.Fatalf("apply QBFT extra: %v", err)
	}
	return h
}

// TestAllowedFutureBlockTimeTransition_Tightened is the reported PoC: a
// transition cutting the window from 5s to 1s must reject a header 3s ahead.
func TestAllowedFutureBlockTimeTransition_Tightened(t *testing.T) {
	engine := newTimingEngine(&istanbul.Config{
		AllowedFutureBlockTime: 5,
		Transitions:            []params.Transition{{Block: big.NewInt(0), AllowedFutureBlockTime: 1}},
	})
	h := futureHeader(t, uint64(time.Now().Unix())+3)
	if err := engine.verifyHeader(nil, h, nil, nil); err != consensus.ErrFutureBlock {
		t.Fatalf("expected ErrFutureBlock under the tightened window, got %v", err)
	}
}

// TestAllowedFutureBlockTimeTransition_Loosened checks a widened window is
// honoured too, rather than the static base value.
func TestAllowedFutureBlockTimeTransition_Loosened(t *testing.T) {
	engine := newTimingEngine(&istanbul.Config{
		AllowedFutureBlockTime: 1,
		Transitions:            []params.Transition{{Block: big.NewInt(0), AllowedFutureBlockTime: 10}},
	})
	h := futureHeader(t, uint64(time.Now().Unix())+5)
	if err := engine.verifyHeader(nil, h, nil, nil); err == consensus.ErrFutureBlock {
		t.Fatal("header inside the loosened window was treated as a future block")
	}
}
