package qbftengine

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"reflect"
	"testing"

	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/crypto"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/hexutil"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/core/types"
)

func TestPrepareExtra(t *testing.T) {
	validators := make([]common.Address, 4)
	validators[0] = common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a"))
	validators[1] = common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212"))
	validators[2] = common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6"))
	validators[3] = common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440"))

	expectedResult := hexutil.MustDecode("0xf87aa00000000000000000000000000000000000000000000000000000000000000000f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c080c0")

	h := &types.Header{}
	err := ApplyHeaderQBFTExtra(
		h,
		WriteValidators(validators),
	)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}
	if !reflect.DeepEqual(h.Extra, expectedResult) {
		t.Errorf("payload mismatch: have %v, want %v", h.Extra, expectedResult)
	}
}

func TestWriteCommittedSeals(t *testing.T) {
	istRawData := hexutil.MustDecode("0xf85a80f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c080c0")
	expectedCommittedSeal := append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraSeal-3)...)
	expectedIstExtra := &types.QBFTExtra{
		VanityData: []byte{},
		Validators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
			common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		CommittedSeal: [][]byte{expectedCommittedSeal},
		Round:         0,
		Vote:          nil,
	}

	h := &types.Header{
		Extra: istRawData,
	}

	// normal case
	err := ApplyHeaderQBFTExtra(
		h,
		writeCommittedSeals([][]byte{expectedCommittedSeal}),
	)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}

	// verify istanbul extra-data
	istExtra, err := getExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedCommittedSeal := append(expectedCommittedSeal, make([]byte, 1)...)
	err = ApplyHeaderQBFTExtra(
		h,
		writeCommittedSeals([][]byte{unexpectedCommittedSeal}),
	)
	if err != istanbulcommon.ErrInvalidCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, istanbulcommon.ErrInvalidCommittedSeals)
	}
}

func TestWriteRoundNumber(t *testing.T) {
	istRawData := hexutil.MustDecode("0xf85a80f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c005c0")
	expectedIstExtra := &types.QBFTExtra{
		VanityData: []byte{},
		Validators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
			common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		CommittedSeal: [][]byte{},
		Round:         5,
		Vote:          nil,
	}

	var expectedErr error

	h := &types.Header{
		Extra: istRawData,
	}

	// normal case
	err := ApplyHeaderQBFTExtra(
		h,
		writeRoundNumber(big.NewInt(5)),
	)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := getExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra.VanityData, expectedIstExtra.VanityData)
	}
}

func TestWriteValidatorVote(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istRawData := hexutil.MustDecode("0xf85a80f8549444add0ec310f115a0e603b2d7db9f067778eaf8a94294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c005c0")
	vote := &types.ValidatorVote{RecipientAddress: common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f06777123456")), VoteType: types.QBFTAuthVote}
	expectedIstExtra := &types.QBFTExtra{
		VanityData:    vanity,
		Validators:    []common.Address{},
		CommittedSeal: [][]byte{},
		Round:         0,
		Vote:          vote,
	}

	var expectedErr error

	h := &types.Header{
		Extra: istRawData,
	}

	// normal case
	err := ApplyHeaderQBFTExtra(
		h,
		WriteVote(common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f06777123456")), true),
	)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := getExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra.Vote, expectedIstExtra.Vote) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}
}

// ---- Quorum threshold regression tests ---
// For validator set sizes N=4 and N=7, it constructs two conflicting blocks at the same height, each signed by
// only F+1 validators (a minority). It then calls VerifyHeader on both.
// If both blocks verify successfully, it proves the engine is vulnerable — meaning your chain can finalize two
// different blocks, causing a chain split. This test should show that both blocks are rejected, confirming safety.
func TestQuorum_DoubleFinalization_MinoritySets(t *testing.T) {
	for _, N := range []int{4, 7} {
		t.Run("N="+big.NewInt(int64(N)).String(), func(t *testing.T) {
			F := (N - 1) / 3    // IBFT F
			minority := F + 1   // buggy rule accepts this
			if 2*minority > N { // sanity
				t.Fatalf("invalid test params for N=%d", N)
			}

			// Generate N validators
			keys := make([]*ecdsa.PrivateKey, N)
			addrs := make([]common.Address, N)
			for i := 0; i < N; i++ {
				k, err := crypto.GenerateKey()
				if err != nil {
					t.Fatalf("keygen failed: %v", err)
				}
				keys[i] = k
				addrs[i] = crypto.PubkeyToAddress(k.PublicKey)
			}

			engine := NewEngine(&istanbul.Config{}, addrs[0], func(data []byte) ([]byte, error) {
				// signer fn isn't used in this verification-path test
				return make([]byte, 65), nil
			})

			// Parent header at height 0
			parent := &types.Header{
				Number:     big.NewInt(0),
				ParentHash: common.Hash{},
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       1,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(parent, WriteValidators(addrs), writeRoundNumber(big.NewInt(0))); err != nil {
				t.Fatalf("failed to apply QBFT extra to parent: %v", err)
			}
			parentHash := parent.Hash()

			// Two conflicting children at height 1 (different coinbase => different hash)
			hdrA := &types.Header{
				Number:     big.NewInt(1),
				ParentHash: parentHash,
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       2,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(hdrA, WriteValidators(addrs), writeRoundNumber(big.NewInt(1))); err != nil {
				t.Fatalf("failed to apply QBFT extra to hdrA: %v", err)
			}

			hdrB := &types.Header{
				Number:     big.NewInt(1),
				ParentHash: parentHash,
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[1%N],
				UncleHash:  types.EmptyUncleHash,
				Time:       2,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(hdrB, WriteValidators(addrs), writeRoundNumber(big.NewInt(1))); err != nil {
				t.Fatalf("failed to apply QBFT extra to hdrB: %v", err)
			}

			if hdrA.Hash() == hdrB.Hash() {
				t.Fatalf("headers identical; adjust coinbase or other header field")
			}

			// Prepare proposal seals
			propSealA := PrepareCommittedSeal(hdrA, 1)
			propSealB := PrepareCommittedSeal(hdrB, 1)

			// Group A = indices [0 .. minority-1], Group B = [minority .. 2*minority-1]
			var sealsA [][]byte
			for i := 0; i < minority; i++ {
				sig, err := crypto.Sign(propSealA, keys[i])
				if err != nil {
					t.Fatalf("sign hdrA: %v", err)
				}
				sealsA = append(sealsA, sig)
			}
			var sealsB [][]byte
			for i := minority; i < 2*minority; i++ {
				sig, err := crypto.Sign(propSealB, keys[i])
				if err != nil {
					t.Fatalf("sign hdrB: %v", err)
				}
				sealsB = append(sealsB, sig)
			}

			if err := ApplyHeaderQBFTExtra(hdrA, WriteValidators(addrs), writeRoundNumber(big.NewInt(1)), writeCommittedSeals(sealsA)); err != nil {
				t.Fatalf("write seals A: %v", err)
			}
			if err := ApplyHeaderQBFTExtra(hdrB, WriteValidators(addrs), writeRoundNumber(big.NewInt(1)), writeCommittedSeals(sealsB)); err != nil {
				t.Fatalf("write seals B: %v", err)
			}

			// Real validator set
			policy := istanbul.NewProposerPolicy(istanbul.RoundRobin)
			valSet := validator.NewSet(addrs, policy)

			// Verify both; with the BUG (F+1), both pass ⇒ test must FAIL.
			// After FIX (2F+1), both should error ⇒ test PASSES.
			errA := engine.VerifyHeader(nil, hdrA, []*types.Header{parent}, valSet)
			errB := engine.VerifyHeader(nil, hdrB, []*types.Header{parent}, valSet)

			if errA == nil && errB == nil {
				t.Fatalf("[VULNERABLE] N=%d: both conflicting headers verified with minority (F+1=%d) seals", N, minority)
			} else {
				t.Logf("[SECURE] N=%d: double-finalization prevented; errA=%v errB=%v", N, errA, errB)
			}
		})
	}
}

func TestUnderQuorumBlockIsRejected(t *testing.T) {
	// We check a couple of validator set sizes to cover F=1 and F=2
	for _, N := range []int{4, 7} {
		t.Run("N="+big.NewInt(int64(N)).String(), func(t *testing.T) {
			F := (N - 1) / 3
			minority := F + 1 // this is < 2F+1, i.e. under-quorum

			// Sanity: for IBFT we must have 3F+1 validators
			if 3*F+1 != N {
				t.Fatalf("invalid test params: N=%d is not 3F+1 (F=%d)", N, F)
			}

			// --- 1) Generate N validators (keys + addresses) ---
			keys := make([]*ecdsa.PrivateKey, N)
			addrs := make([]common.Address, N)
			for i := 0; i < N; i++ {
				k, err := crypto.GenerateKey()
				if err != nil {
					t.Fatalf("keygen failed: %v", err)
				}
				keys[i] = k
				addrs[i] = crypto.PubkeyToAddress(k.PublicKey)
			}

			// --- 2) Build a QBFT engine instance ---
			engine := NewEngine(&istanbul.Config{}, addrs[0], func(data []byte) ([]byte, error) {
				// signer fn isn't used in this verification-path test
				return make([]byte, 65), nil
			})

			// --- 3) Build parent header at height 0 with validator set in extra-data ---
			parent := &types.Header{
				Number:     big.NewInt(0),
				ParentHash: common.Hash{},
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       1,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(parent, WriteValidators(addrs), writeRoundNumber(big.NewInt(0))); err != nil {
				t.Fatalf("failed to apply QBFT extra to parent: %v", err)
			}

			// --- 4) Build child header at height 1 ---
			hdr := &types.Header{
				Number:     big.NewInt(1),
				ParentHash: parent.Hash(),
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       2,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(hdr, WriteValidators(addrs), writeRoundNumber(big.NewInt(1))); err != nil {
				t.Fatalf("failed to apply QBFT extra to child header: %v", err)
			}

			// --- 5) Prepare commit hash and sign it with only F+1 validators (under quorum) ---
			propSeal := PrepareCommittedSeal(hdr, 1) // round 1 commit hash

			var seals [][]byte
			for i := 0; i < minority; i++ {
				sig, err := crypto.Sign(propSeal, keys[i])
				if err != nil {
					t.Fatalf("sign failed: %v", err)
				}
				seals = append(seals, sig)
			}

			// Write the under-quorum committed seals into the header extra-data
			if err := ApplyHeaderQBFTExtra(
				hdr,
				WriteValidators(addrs),
				writeRoundNumber(big.NewInt(1)),
				writeCommittedSeals(seals),
			); err != nil {
				t.Fatalf("failed to write under-quorum committed seals: %v", err)
			}

			// --- 6) Build validator set and run VerifyHeader ---
			valSet := validator.NewSet(addrs, istanbul.NewProposerPolicy(istanbul.RoundRobin))

			err := engine.VerifyHeader(nil, hdr, []*types.Header{parent}, valSet)
			if err == nil {
				t.Fatalf(
					"BUG: N=%d (F=%d): block with only F+1 (%d) committed seals was ACCEPTED; should be rejected (requires 2F+1=%d)",
					N, F, minority, 2*F+1,
				)
			}
		})
	}
}

// For N=4 and N=7, constructs a block signed by ≥ 2F+1 validators (the correct IBFT quorum)
// and checks that VerifyHeader accepts it.
// Ensures that after fixing the quorum rule, valid blocks still finalize correctly,
// so we do not break normal consensus behavior.
func TestQuorum_ProperThreshold_Passes(t *testing.T) {
	for _, N := range []int{4, 7} {
		t.Run("N="+big.NewInt(int64(N)).String(), func(t *testing.T) {
			F := (N - 1) / 3
			required := 2*F + 1

			keys := make([]*ecdsa.PrivateKey, N)
			addrs := make([]common.Address, N)
			for i := 0; i < N; i++ {
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

			parent := &types.Header{
				Number:     big.NewInt(0),
				ParentHash: common.Hash{},
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       1,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(parent, WriteValidators(addrs), writeRoundNumber(big.NewInt(0))); err != nil {
				t.Fatalf("apply extra parent: %v", err)
			}

			hdr := &types.Header{
				Number:     big.NewInt(1),
				ParentHash: parent.Hash(),
				MixDigest:  types.IstanbulDigest,
				Difficulty: istanbulcommon.DefaultDifficulty,
				Coinbase:   addrs[0],
				UncleHash:  types.EmptyUncleHash,
				Time:       2,
				GasLimit:   30_000_000,
				GasUsed:    0,
			}
			if err := ApplyHeaderQBFTExtra(hdr, WriteValidators(addrs), writeRoundNumber(big.NewInt(1))); err != nil {
				t.Fatalf("apply extra hdr: %v", err)
			}

			propSeal := PrepareCommittedSeal(hdr, 1)
			var seals [][]byte
			for i := 0; i < required; i++ {
				sig, err := crypto.Sign(propSeal, keys[i])
				if err != nil {
					t.Fatalf("sign: %v", err)
				}
				seals = append(seals, sig)
			}
			if err := ApplyHeaderQBFTExtra(hdr, WriteValidators(addrs), writeRoundNumber(big.NewInt(1)), writeCommittedSeals(seals)); err != nil {
				t.Fatalf("write seals: %v", err)
			}

			valSet := validator.NewSet(addrs, istanbul.NewProposerPolicy(istanbul.RoundRobin))
			err := engine.VerifyHeader(nil, hdr, []*types.Header{parent}, valSet)
			if err != nil {
				t.Fatalf("N=%d: expected ≥2F+1 (%d) to pass, got error: %v", N, required, err)
			}
		})
	}
}
