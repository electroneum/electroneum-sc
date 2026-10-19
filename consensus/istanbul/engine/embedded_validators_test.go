package qbftengine

import (
	"bytes"
	"math/big"
	"sort"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/params"
)

// embeddedValidatorsProposal builds a parent and a proposer-sealed child (no
// committed seals yet, i.e. a block at proposal stage) whose embedded validator
// list is `embedded`, and runs VerifyHeader on it against the real 4-validator
// set under `cfg`.
func embeddedValidatorsProposal(t *testing.T, cfg *params.ChainConfig, embedded func(sorted []common.Address) []common.Address) error {
	t.Helper()

	keys, addrs := genValidators(t, 4)
	sorted := append([]common.Address(nil), addrs...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i][:], sorted[j][:]) < 0 })

	key, proposer := keys[0], addrs[0]
	engine := NewEngine(&istanbul.Config{}, proposer, func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	})
	chain := newForkAwareChain(cfg)

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   proposer,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators(sorted)); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   proposer,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators(embedded(sorted))); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}
	if cfg.IsFutureFork(child.Number) {
		seal, err := engine.sign(sigHash(child).Bytes())
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		if err := ApplyHeaderQBFTExtra(child, writeProposerSeal(seal)); err != nil {
			t.Fatalf("apply proposer seal: %v", err)
		}
	}

	valSet := validator.NewSet(addrs, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	return engine.VerifyHeader(chain, child, []*types.Header{parent}, valSet)
}

// noLondon keeps the EIP-1559 header checks out of the way so these tests only
// exercise the validator-list rule.
func noLondon(cfg *params.ChainConfig) *params.ChainConfig {
	cfg.LondonBlock = nil
	return cfg
}

// TestPostFork_EmbeddedValidatorsMustMatchSnapshot checks that after FutureFork a
// proposal is rejected unless its embedded validator list is exactly the parent
// validator set in sorted order, and that the rejection happens at proposal
// stage, before any committed seals exist.
func TestPostFork_EmbeddedValidatorsMustMatchSnapshot(t *testing.T) {
	cfg := noLondon(futureForkConfig())

	// Exactly what Prepare writes: passes everything up to the (expected)
	// missing committed seals.
	if err := embeddedValidatorsProposal(t, cfg, func(s []common.Address) []common.Address { return s }); err != istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("matching list: have %v, want %v", err, istanbulcommon.ErrEmptyCommittedSeals)
	}

	bad := map[string]func(s []common.Address) []common.Address{
		"empty":   func(s []common.Address) []common.Address { return nil },
		"missing": func(s []common.Address) []common.Address { return s[:3] },
		"extra": func(s []common.Address) []common.Address {
			return append(append([]common.Address(nil), s...), common.HexToAddress("0x01"))
		},
		"replaced": func(s []common.Address) []common.Address {
			out := append([]common.Address(nil), s...)
			out[2] = common.HexToAddress("0x01")
			return out
		},
		"unsorted": func(s []common.Address) []common.Address {
			out := append([]common.Address(nil), s...)
			out[0], out[1] = out[1], out[0]
			return out
		},
		"duplicate": func(s []common.Address) []common.Address {
			out := append([]common.Address(nil), s...)
			out[3] = out[2]
			return out
		},
	}
	for name, embedded := range bad {
		t.Run(name, func(t *testing.T) {
			if err := embeddedValidatorsProposal(t, cfg, embedded); err != istanbulcommon.ErrInconsistentValidatorSet {
				t.Fatalf("have %v, want %v", err, istanbulcommon.ErrInconsistentValidatorSet)
			}
		})
	}
}

// TestPreFork_EmbeddedValidatorsNotChecked checks that before FutureFork the
// embedded validator list is still not part of header validity, so existing
// history replays unchanged.
func TestPreFork_EmbeddedValidatorsNotChecked(t *testing.T) {
	cfg := noLondon(preForkConfig())
	if err := embeddedValidatorsProposal(t, cfg, func(s []common.Address) []common.Address { return nil }); err != istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("pre-fork empty list: have %v, want %v", err, istanbulcommon.ErrEmptyCommittedSeals)
	}
}
