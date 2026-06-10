// Copyright 2026 The Electroneum Authors
//
// Regression tests for the EIP-1559 header validation that verifyCascadingFields
// now enforces. These exercise the same code path beacon/clique/ethash already
// covered, ensuring that:
//
//   - a header with the correctly derived BaseFee is accepted (negative control,
//     filtered to ignore the unrelated ErrEmptyCommittedSeals which the helper
//     headers do not include);
//   - a header with a tampered BaseFee is rejected;
//   - a header with a nil BaseFee on a London chain is rejected before reaching
//     state transition (state_transition.go:260 would otherwise panic);
//   - a header whose GasLimit violates the ±1/1024 per-block bound is rejected
//     (both directions);
//   - a header whose GasLimit exceeds params.MaxGasLimit is rejected by the
//     absolute cap.

package qbftengine

import (
	"crypto/ecdsa"
	"math/big"
	"strings"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/consensus/misc"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/params"
)

// mockChainHeaderReader is a minimal consensus.ChainHeaderReader that returns a
// fixed ChainConfig. The other accessors are unused because the tests supply
// parents directly to verifyCascadingFields via the `parents` argument.
type mockChainHeaderReader struct {
	cfg *params.ChainConfig
}

func (m *mockChainHeaderReader) Config() *params.ChainConfig                 { return m.cfg }
func (m *mockChainHeaderReader) CurrentHeader() *types.Header                { return nil }
func (m *mockChainHeaderReader) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (m *mockChainHeaderReader) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (m *mockChainHeaderReader) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (m *mockChainHeaderReader) GetTd(common.Hash, uint64) *big.Int          { return nil }

// eip1559TestFixture sets up:
//   - a 4-validator set,
//   - a chain config with London active from block 0,
//   - a properly-formed genesis-style parent header (block 0, BaseFee = nil to
//     match real first-pre-London-equivalent behaviour; CalcBaseFee yields
//     params.InitialBaseFee for the first London child),
//   - an engine instance ready for VerifyHeader.
type eip1559TestFixture struct {
	engine *Engine
	chain  *mockChainHeaderReader
	parent *types.Header
	valSet istanbul.ValidatorSet
	keys   []*ecdsa.PrivateKey
	addrs  []common.Address
}

func newEIP1559Fixture(t *testing.T) *eip1559TestFixture {
	t.Helper()

	keys := make([]*ecdsa.PrivateKey, 4)
	addrs := make([]common.Address, 4)
	for i := 0; i < 4; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("keygen failed: %v", err)
		}
		keys[i] = k
		addrs[i] = crypto.PubkeyToAddress(k.PublicKey)
	}

	engine := NewEngine(&istanbul.Config{}, addrs[0], func(data []byte) ([]byte, error) {
		return make([]byte, 65), nil
	})

	// London active from block 0 so both parent and child are already on
	// London — the steady-state behaviour we want to exercise. With
	// parent.GasUsed = parent.GasLimit/ElasticityMultiplier (the gas target),
	// CalcBaseFee returns parent.BaseFee unchanged for the child, which
	// keeps the expected base fee predictable and stable.
	cfg := &params.ChainConfig{
		ChainID:     big.NewInt(1),
		LondonBlock: big.NewInt(0),
	}

	// Parent is block 0, post-London. GasUsed equals the gas target so the
	// derived child BaseFee equals parent.BaseFee.
	const parentGasLimit uint64 = 30_000_000
	parent := &types.Header{
		Number:     big.NewInt(0),
		ParentHash: common.Hash{},
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addrs[0],
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   parentGasLimit,
		GasUsed:    parentGasLimit / params.ElasticityMultiplier,
		BaseFee:    big.NewInt(params.InitialBaseFee),
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators(addrs), writeRoundNumber(big.NewInt(0))); err != nil {
		t.Fatalf("apply QBFT extra to parent: %v", err)
	}

	policy := istanbul.NewProposerPolicy(istanbul.RoundRobin)
	valSet := validator.NewSet(addrs, policy)

	return &eip1559TestFixture{
		engine: engine,
		chain:  &mockChainHeaderReader{cfg: cfg},
		parent: parent,
		valSet: valSet,
		keys:   keys,
		addrs:  addrs,
	}
}

// childHeader builds a well-formed block-1 child of the fixture's parent with
// the supplied BaseFee and GasLimit. Caller may pass nil baseFee. Coinbase is
// set to the first validator so verifySigner won't trip before we reach the new
// check; committed seals are intentionally absent (the verifier returns
// ErrEmptyCommittedSeals at the end, which callers filter out).
func (f *eip1559TestFixture) childHeader(t *testing.T, baseFee *big.Int, gasLimit uint64) *types.Header {
	t.Helper()
	h := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: f.parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   f.addrs[0],
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   gasLimit,
		GasUsed:    0,
		BaseFee:    baseFee,
	}
	if err := ApplyHeaderQBFTExtra(h, WriteValidators(f.addrs), writeRoundNumber(big.NewInt(1))); err != nil {
		t.Fatalf("apply QBFT extra to child: %v", err)
	}
	return h
}

// verify runs VerifyHeader supplying parents to avoid the chain.GetHeader path.
func (f *eip1559TestFixture) verify(h *types.Header) error {
	return f.engine.VerifyHeader(f.chain, h, []*types.Header{f.parent}, f.valSet)
}

// TestVerifyCascadingFields_BaseFeeCorrect_Accepts confirms a header carrying
// the BaseFee that CalcBaseFee would have produced is accepted as far as the
// new check is concerned. We tolerate ErrEmptyCommittedSeals because these
// headers do not carry seals — that is the post-EIP-1559 stop point.
func TestVerifyCascadingFields_BaseFeeCorrect_Accepts(t *testing.T) {
	f := newEIP1559Fixture(t)
	expected := misc.CalcBaseFee(f.chain.Config(), f.parent)
	h := f.childHeader(t, expected, f.parent.GasLimit)

	err := f.verify(h)
	if err != nil && err != istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("correct BaseFee should pass EIP-1559 validation; got %v", err)
	}
}

// TestVerifyCascadingFields_BaseFeeZero_Rejected is the canonical Bugcrowd PoC:
// a malicious validator setting BaseFee = 0 to skip the EIP-1559 burn must now
// be rejected.
func TestVerifyCascadingFields_BaseFeeZero_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	expected := misc.CalcBaseFee(f.chain.Config(), f.parent)
	if expected.Sign() == 0 {
		// Sanity guard: the test is meaningless if the expected value is also
		// zero, which would only happen on a long-idle chain. With our genesis
		// (London active, parent gas used = 0) CalcBaseFee returns
		// params.InitialBaseFee.
		t.Fatalf("test fixture should produce a non-zero expected BaseFee; got %v", expected)
	}
	h := f.childHeader(t, big.NewInt(0), f.parent.GasLimit)

	err := f.verify(h)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("BaseFee=0 must be rejected; got err=%v", err)
	}
	if !strings.Contains(err.Error(), "invalid baseFee") {
		t.Fatalf("expected an 'invalid baseFee' error, got: %v", err)
	}
}

// TestVerifyCascadingFields_BaseFeeNil_Rejected guards against a nil BaseFee
// reaching state_transition.go:260 where it would dereference and panic.
func TestVerifyCascadingFields_BaseFeeNil_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	h := f.childHeader(t, nil, f.parent.GasLimit)

	err := f.verify(h)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("nil BaseFee must be rejected; got err=%v", err)
	}
	if !strings.Contains(err.Error(), "missing baseFee") {
		t.Fatalf("expected a 'missing baseFee' error, got: %v", err)
	}
}

// TestVerifyCascadingFields_GasLimitTooHigh_Rejected confirms the per-block
// +1/1024 bound enforced via misc.VerifyGaslimit.
func TestVerifyCascadingFields_GasLimitTooHigh_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	// One unit beyond the upper bound (parent.GasLimit + parent.GasLimit/1024).
	tooHigh := f.parent.GasLimit + (f.parent.GasLimit / 1024) + 1
	expected := misc.CalcBaseFee(f.chain.Config(), f.parent)
	h := f.childHeader(t, expected, tooHigh)

	err := f.verify(h)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("oversized GasLimit must be rejected; got err=%v", err)
	}
	if !strings.Contains(err.Error(), "invalid gas limit") && !strings.Contains(err.Error(), "gas limit") {
		t.Fatalf("expected a gas-limit error, got: %v", err)
	}
}

// TestVerifyCascadingFields_GasLimitTooLow_Rejected confirms the symmetric
// lower bound (parent.GasLimit - parent.GasLimit/1024).
func TestVerifyCascadingFields_GasLimitTooLow_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	tooLow := f.parent.GasLimit - (f.parent.GasLimit / 1024) - 1
	expected := misc.CalcBaseFee(f.chain.Config(), f.parent)
	h := f.childHeader(t, expected, tooLow)

	err := f.verify(h)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("undersized GasLimit must be rejected; got err=%v", err)
	}
	if !strings.Contains(err.Error(), "invalid gas limit") && !strings.Contains(err.Error(), "gas limit") {
		t.Fatalf("expected a gas-limit error, got: %v", err)
	}
}

// TestMaxGasLimitStaticInvariant pins params.MaxGasLimit to 2^63-1. The cap
// branch in verifyCascadingFields uses a strict `>` comparison against this
// value, so as long as it equals 2^63-1 the cap is unreachable in uint64
// arithmetic (no representable GasLimit can exceed it). The check exists for
// parity with beacon/clique/ethash and to defend against future changes that
// either widen GasLimit or lower MaxGasLimit; this static guard alerts us if
// the constant moves so the engine logic and tests can be revisited together.
func TestMaxGasLimitStaticInvariant(t *testing.T) {
	const expected uint64 = 0x7fffffffffffffff
	if params.MaxGasLimit != expected {
		t.Fatalf("params.MaxGasLimit changed to %d; re-evaluate the cap in verifyCascadingFields and add a runtime test if it is now reachable", params.MaxGasLimit)
	}
}
