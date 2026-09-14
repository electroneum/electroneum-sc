package main

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulbackend "github.com/electroneum/electroneum-sc/consensus/istanbul/backend"
	istanbulengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/params"
)

// Real Electroneum mainnet block 1000, captured 2026-09-08.
const (
	fixture     = "testdata/mainnet-header-1000.json"
	fixtureHash = "0x2e5bd3bdd73d7af8b0f5064b1216f68b3a1c8f81fdee46fc21f8dca7c71de8b6"
)

func loadHeader(t *testing.T) *types.Header {
	t.Helper()
	blob, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var h types.Header
	if err := json.Unmarshal(blob, &h); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return &h
}

func newEngine() *istanbulengine.Engine {
	return istanbulengine.NewEngine(istanbulConfig(params.MainnetChainConfig), common.Address{}, nil)
}

func valSetOf(t *testing.T, h *types.Header) istanbul.ValidatorSet {
	t.Helper()
	extra, err := types.ExtractQBFTExtra(h)
	if err != nil {
		t.Fatalf("ExtractQBFTExtra: %v", err)
	}
	return validator.NewSet(extra.Validators, istanbul.NewProposerPolicy(istanbul.RoundRobin))
}

// TestFixtureRoundTrips is the load-bearing assertion of the whole harness: a
// header rebuilt from JSON must hash to what the node reported. If our RLP
// encoding of a header ever drifts, every seal and every ParentHash check
// downstream is meaningless, so this failing invalidates all the rest.
func TestFixtureRoundTrips(t *testing.T) {
	h := loadHeader(t)
	if got := h.Hash().Hex(); got != fixtureHash {
		t.Fatalf("header hash mismatch:\n  rebuilt %s\n  want    %s", got, fixtureHash)
	}
	extra, err := types.ExtractQBFTExtra(h)
	if err != nil {
		t.Fatalf("ExtractQBFTExtra: %v", err)
	}
	if len(extra.Validators) == 0 {
		t.Fatal("no validators decoded from QBFT extra")
	}
	if len(extra.CommittedSeal) == 0 {
		t.Fatal("no committed seals decoded from QBFT extra")
	}
	// Quorum on this chain is ceil(2N/3); a header carrying fewer committed
	// seals than that should never have been accepted by the network.
	if n, seals := len(extra.Validators), len(extra.CommittedSeal); seals*3 < n*2 {
		t.Fatalf("quorum shortfall: %d seals for %d validators", seals, n)
	}
}

// TestVerifyAcceptsRealHeader guards the other direction: the checks below only
// mean something if the unmodified header passes.
func TestVerifyAcceptsRealHeader(t *testing.T) {
	h := loadHeader(t)
	if err := newEngine().VerifySeal(newChainReader(params.MainnetChainConfig), h, valSetOf(t, h)); err != nil {
		t.Fatalf("real mainnet header rejected: %v", err)
	}
}

// TestVerifyRejectsCorruption is the negative control. A verifier that cannot
// fail proves nothing, so each case corrupts one consensus-critical field and
// asserts the engine notices.
func TestVerifyRejectsCorruption(t *testing.T) {
	chain := newChainReader(params.MainnetChainConfig)
	eng := newEngine()

	cases := []struct {
		name    string
		corrupt func(*types.Header)
	}{
		{"difficulty", func(h *types.Header) { h.Difficulty = big.NewInt(2) }},
		{"mix digest", func(h *types.Header) { h.MixDigest = common.Hash{} }},
		{"uncle hash", func(h *types.Header) { h.UncleHash = common.Hash{} }},
		{"extra data", func(h *types.Header) { h.Extra = h.Extra[:len(h.Extra)-8] }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := loadHeader(t)
			valSet := valSetOf(t, h) // capture the set BEFORE corrupting
			tc.corrupt(h)
			// Time is checked before the fields we care about, so pin it.
			h.Time = 1
			if err := eng.VerifyHeader(chain, h, nil, valSet); err == nil {
				t.Fatalf("engine accepted a header with corrupted %s", tc.name)
			}
		})
	}
}

// TestRewardCurve pins the emission arithmetic against values read from the
// live chain via istanbul_getTotalEmission on 2026-09-08. These are consensus
// constants: if a rebase changes any of them, every node re-derives a different
// circulating supply and the chain silently splits.
func TestRewardCurve(t *testing.T) {
	var (
		cfg      = params.MainnetChainConfig
		rewarder = &istanbulbackend.Backend{}
		chain    = newChainReader(cfg)
		oneETN   = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	)
	supplyBelowCap := new(big.Int).Set(cfg.GenesisETN)

	cases := []struct {
		block   int64
		wantETN int64
	}{
		{1, 4},          // genesis era
		{2_575_607, 4},  // last block before the first halving
		{2_575_608, 2},  // first halving
		{15_762_000, 2}, // today
	}
	for _, tc := range cases {
		h := &types.Header{Number: big.NewInt(tc.block)}
		got := rewarder.GetBaseBlockReward(chain, h, supplyBelowCap)
		want := new(big.Int).Mul(big.NewInt(tc.wantETN), oneETN)
		if got.Cmp(want) != 0 {
			t.Errorf("block %d: reward %s, want %s", tc.block, got, want)
		}
	}

	// At the 21B cap the reward must clamp to zero rather than mint past it.
	atCap := new(big.Int).Set(maxSupply(t))
	if got := rewarder.GetBaseBlockReward(chain, &types.Header{Number: big.NewInt(3_000_000)}, atCap); got.Sign() != 0 {
		t.Errorf("reward at max supply = %s, want 0", got)
	}

	// The cap clamp is applied BEFORE the halving shift, not after. That
	// ordering is observable and consensus-critical: near the cap the reward is
	// (maxSupply - supply) >> halvings, so once halvings >= 1 the last wei can
	// never be minted -- supply approaches the cap asymptotically and stops
	// short. A rebase that "tidies" this into shift-then-clamp would mint
	// differently and split the chain, so both sides of the boundary are pinned
	// here.
	nearCap := new(big.Int).Sub(maxSupply(t), big.NewInt(1))
	// halvings == 0 (block < 2,575,608): the remaining wei is paid in full.
	if got := rewarder.GetBaseBlockReward(chain, &types.Header{Number: big.NewInt(1_000_000)}, nearCap); got.Cmp(big.NewInt(1)) != 0 {
		t.Errorf("reward one wei below cap at halvings=0 = %s, want 1", got)
	}
	// halvings == 1 (block >= 2,575,608): that same 1 wei is shifted away.
	if got := rewarder.GetBaseBlockReward(chain, &types.Header{Number: big.NewInt(3_000_000)}, nearCap); got.Sign() != 0 {
		t.Errorf("reward one wei below cap at halvings=1 = %s, want 0 (clamp precedes shift)", got)
	}
}

func maxSupply(t *testing.T) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(params.ETNMaxSupply, 0)
	if !ok {
		t.Fatalf("cannot parse ETNMaxSupply %q", params.ETNMaxSupply)
	}
	return v
}
