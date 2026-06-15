package qbftengine

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"reflect"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/hexutil"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rlp"
)

// forkAwareChainReader implements consensus.ChainHeaderReader for ProposerSeal
// tests. Unlike the simpler mockChainHeaderReader in engine_eip1559_test.go,
// this one can store/retrieve parent headers (needed by Seal).
type forkAwareChainReader struct {
	config  *params.ChainConfig
	headers map[common.Hash]*types.Header
}

func newForkAwareChain(cfg *params.ChainConfig) *forkAwareChainReader {
	return &forkAwareChainReader{
		config:  cfg,
		headers: make(map[common.Hash]*types.Header),
	}
}

func (m *forkAwareChainReader) Config() *params.ChainConfig  { return m.config }
func (m *forkAwareChainReader) CurrentHeader() *types.Header { return nil }
func (m *forkAwareChainReader) GetHeader(hash common.Hash, number uint64) *types.Header {
	return m.headers[hash]
}
func (m *forkAwareChainReader) GetHeaderByNumber(number uint64) *types.Header { return nil }
func (m *forkAwareChainReader) GetHeaderByHash(hash common.Hash) *types.Header {
	return m.headers[hash]
}
func (m *forkAwareChainReader) GetTd(hash common.Hash, number uint64) *big.Int { return nil }

func (m *forkAwareChainReader) addHeader(h *types.Header) {
	m.headers[h.Hash()] = h
}

// futureForkConfig returns a ChainConfig where FutureFork is always active (block 0).
func futureForkConfig() *params.ChainConfig {
	return &params.ChainConfig{
		ChainID:             big.NewInt(1),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		FutureForkBlock:     big.NewInt(0),
	}
}

// preForkConfig returns a ChainConfig where FutureFork is never active.
func preForkConfig() *params.ChainConfig {
	cfg := futureForkConfig()
	cfg.FutureForkBlock = nil
	return cfg
}

func TestPrepareExtra(t *testing.T) {
	validators := make([]common.Address, 4)
	validators[0] = common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a"))
	validators[1] = common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212"))
	validators[2] = common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6"))
	validators[3] = common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440"))

	h := &types.Header{}
	err := ApplyHeaderQBFTExtra(
		h,
		WriteValidators(validators),
	)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}

	// Verify by round-tripping through decode
	extra, err := types.ExtractQBFTExtra(h)
	if err != nil {
		t.Fatalf("failed to extract extra: %v", err)
	}
	if len(extra.Validators) != 4 {
		t.Errorf("validator count mismatch: have %d, want 4", len(extra.Validators))
	}
	for i, v := range validators {
		if extra.Validators[i] != v {
			t.Errorf("validator %d mismatch: have %s, want %s", i, extra.Validators[i].Hex(), v.Hex())
		}
	}
}

func TestWriteCommittedSeals(t *testing.T) {
	expectedCommittedSeal := append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraSeal-3)...)

	validators := []common.Address{
		common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
		common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
		common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
		common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
	}

	// Build a header with validators first, then use its encoded form
	setupHeader := &types.Header{}
	if err := ApplyHeaderQBFTExtra(setupHeader, WriteValidators(validators)); err != nil {
		t.Fatalf("failed to setup header: %v", err)
	}
	istRawData := setupHeader.Extra

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
	if len(istExtra.Validators) != len(validators) {
		t.Errorf("validator count mismatch: have %d, want %d", len(istExtra.Validators), len(validators))
	}
	if len(istExtra.CommittedSeal) != 1 || !bytes.Equal(istExtra.CommittedSeal[0], expectedCommittedSeal) {
		t.Errorf("committed seal mismatch")
	}
	if istExtra.Round != 0 {
		t.Errorf("round mismatch: have %d, want 0", istExtra.Round)
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
	validators := []common.Address{
		common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
		common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
		common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
		common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
	}

	// Build a header with validators and round=5
	h := &types.Header{}
	if err := ApplyHeaderQBFTExtra(h, WriteValidators(validators)); err != nil {
		t.Fatalf("failed to setup header: %v", err)
	}

	// normal case
	err := ApplyHeaderQBFTExtra(
		h,
		writeRoundNumber(big.NewInt(5)),
	)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// verify istanbul extra-data
	istExtra, err := getExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if istExtra.Round != 5 {
		t.Errorf("round mismatch: have %d, want 5", istExtra.Round)
	}
	if len(istExtra.Validators) != 4 {
		t.Errorf("validator count mismatch: have %d, want 4", len(istExtra.Validators))
	}
}

func TestWriteValidatorVote(t *testing.T) {
	validators := []common.Address{
		common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
		common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
		common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
		common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
	}
	expectedVote := &types.ValidatorVote{RecipientAddress: common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f06777123456")), VoteType: types.QBFTAuthVote}

	h := &types.Header{}
	if err := ApplyHeaderQBFTExtra(h, WriteValidators(validators)); err != nil {
		t.Fatalf("failed to setup header: %v", err)
	}

	// normal case
	err := ApplyHeaderQBFTExtra(
		h,
		WriteVote(common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f06777123456")), true),
	)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// verify istanbul extra-data
	istExtra, err := getExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra.Vote, expectedVote) {
		t.Errorf("vote mismatch: have %v, want %v", istExtra.Vote, expectedVote)
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

// ---- Post-FutureFork ProposerSeal tests ----

// TestPostFork_SealProducesProposerSeal verifies that after FutureFork,
// Seal() embeds a valid proposer seal in the header's QBFTExtra.
func TestPostFork_SealProducesProposerSeal(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	signFn := func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	}

	engine := NewEngine(&istanbul.Config{}, addr, signFn)
	chain := newForkAwareChain(futureForkConfig())

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}
	block := types.NewBlockWithHeader(child)

	valSet := validator.NewSet([]common.Address{addr}, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	sealedBlock, err := engine.Seal(chain, block, valSet)
	if err != nil {
		t.Fatalf("Seal() failed: %v", err)
	}

	extra, err := types.ExtractQBFTExtra(sealedBlock.Header())
	if err != nil {
		t.Fatalf("extract extra: %v", err)
	}
	if len(extra.ProposerSeal) != types.IstanbulExtraSeal {
		t.Fatalf("ProposerSeal length: have %d, want %d", len(extra.ProposerSeal), types.IstanbulExtraSeal)
	}
}

// TestPostFork_AuthorRecoversFromSeal verifies that Author() recovers the
// correct address from a ProposerSeal-bearing header.
func TestPostFork_AuthorRecoversFromSeal(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	signFn := func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	}

	engine := NewEngine(&istanbul.Config{}, addr, signFn)
	chain := newForkAwareChain(futureForkConfig())

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}
	block := types.NewBlockWithHeader(child)

	valSet := validator.NewSet([]common.Address{addr}, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	sealedBlock, err := engine.Seal(chain, block, valSet)
	if err != nil {
		t.Fatalf("Seal() failed: %v", err)
	}

	author, err := engine.Author(sealedBlock.Header())
	if err != nil {
		t.Fatalf("Author() failed: %v", err)
	}
	if author != addr {
		t.Fatalf("Author() mismatch: have %s, want %s", author.Hex(), addr.Hex())
	}
}

// TestPostFork_VerifySignerRejectsForgedCoinbase verifies that after
// FutureFork, verifySigner rejects a block whose Coinbase has been tampered
// with (it won't match the ProposerSeal recovery).
func TestPostFork_VerifySignerRejectsForgedCoinbase(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	proposer := crypto.PubkeyToAddress(key.PublicKey)
	forged := common.HexToAddress("0xDEADBEEFDEADBEEFDEADBEEFDEADBEEFDEADBEEF")

	signFn := func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	}

	engine := NewEngine(&istanbul.Config{}, proposer, signFn)
	chain := newForkAwareChain(futureForkConfig())

	validators := []common.Address{proposer, forged}
	valSet := validator.NewSet(validators, istanbul.NewProposerPolicy(istanbul.RoundRobin))

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   proposer,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators(validators)); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	// Build and seal a legitimate block
	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators(validators)); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}
	block := types.NewBlockWithHeader(child)
	sealedBlock, err := engine.Seal(chain, block, valSet)
	if err != nil {
		t.Fatalf("Seal() failed: %v", err)
	}

	// Now tamper with Coinbase — swap it to the forged address.
	// The ProposerSeal was computed over a header with Coinbase=proposer,
	// so recovery will return proposer, which won't match forged.
	tamperedHeader := types.CopyHeader(sealedBlock.Header())
	tamperedHeader.Coinbase = forged

	err = engine.verifySigner(chain, tamperedHeader, []*types.Header{parent}, valSet)
	if err == nil {
		t.Fatal("verifySigner should have rejected forged Coinbase, but accepted it")
	}
	if err != istanbulcommon.ErrInvalidCoinbase {
		t.Fatalf("expected ErrInvalidCoinbase, got: %v", err)
	}
}

// TestPostFork_VerifySignerRejectsMissingSeal verifies that after FutureFork,
// verifySigner rejects a block that has no ProposerSeal.
func TestPostFork_VerifySignerRejectsMissingSeal(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	engine := NewEngine(&istanbul.Config{}, addr, func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	})
	chain := newForkAwareChain(futureForkConfig())

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	// Build a block WITHOUT calling Seal() — no ProposerSeal
	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}

	valSet := validator.NewSet([]common.Address{addr}, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	err = engine.verifySigner(chain, child, []*types.Header{parent}, valSet)
	if err == nil {
		t.Fatal("verifySigner should have rejected header without ProposerSeal")
	}
	if err != istanbulcommon.ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

// TestPreFork_SealDoesNotProduceProposerSeal verifies that before FutureFork,
// Seal() does NOT embed a ProposerSeal — existing behaviour is preserved.
func TestPreFork_SealDoesNotProduceProposerSeal(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	engine := NewEngine(&istanbul.Config{}, addr, func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), key)
	})
	chain := newForkAwareChain(preForkConfig())

	parent := &types.Header{
		Number:     big.NewInt(0),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(parent, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply parent extra: %v", err)
	}
	chain.addHeader(parent)

	child := &types.Header{
		Number:     big.NewInt(1),
		ParentHash: parent.Hash(),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		UncleHash:  types.EmptyUncleHash,
		Time:       2,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(child, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply child extra: %v", err)
	}
	block := types.NewBlockWithHeader(child)

	valSet := validator.NewSet([]common.Address{addr}, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	sealedBlock, err := engine.Seal(chain, block, valSet)
	if err != nil {
		t.Fatalf("Seal() failed: %v", err)
	}

	extra, err := types.ExtractQBFTExtra(sealedBlock.Header())
	if err != nil {
		t.Fatalf("extract extra: %v", err)
	}
	if len(extra.ProposerSeal) != 0 {
		t.Fatalf("pre-fork Seal() should not produce ProposerSeal, got %d bytes", len(extra.ProposerSeal))
	}
}

// TestPreFork_BlockHashStableAfterRoundTrip verifies that decoding a pre-fork
// header's Extra (5-field RLP) and re-encoding it does NOT change the block
// hash. This is a regression test for a bug where EncodeRLP always emitted 6
// fields, adding an extra 0x80 byte for the empty ProposerSeal.
func TestPreFork_BlockHashStableAfterRoundTrip(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	header := &types.Header{
		Number:     big.NewInt(1),
		MixDigest:  types.IstanbulDigest,
		Difficulty: istanbulcommon.DefaultDifficulty,
		Coinbase:   addr,
		UncleHash:  types.EmptyUncleHash,
		Time:       1,
		GasLimit:   30_000_000,
	}
	if err := ApplyHeaderQBFTExtra(header, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply extra: %v", err)
	}

	hashBefore := header.Hash()

	// Simulate what QBFTFilteredHeaderWithRound does: decode + re-encode
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		t.Fatalf("extract extra: %v", err)
	}
	extra.CommittedSeal = [][]byte{}
	extra.ProposerSeal = []byte{}
	extra.Round = 0
	reencoded, err := rlp.EncodeToBytes(extra)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	header.Extra = reencoded

	hashAfter := header.Hash()
	if hashBefore != hashAfter {
		t.Fatalf("block hash changed after Extra round-trip: before=%s after=%s", hashBefore.Hex(), hashAfter.Hex())
	}
}

// TestPreFork_AuthorReturnsCoinbase verifies that for pre-fork headers
// (no ProposerSeal), Author() returns header.Coinbase as before.
func TestPreFork_AuthorReturnsCoinbase(t *testing.T) {
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	engine := NewEngine(&istanbul.Config{}, addr, func(data []byte) ([]byte, error) {
		return make([]byte, 65), nil
	})

	header := &types.Header{
		Number:   big.NewInt(1),
		Coinbase: addr,
	}
	if err := ApplyHeaderQBFTExtra(header, WriteValidators([]common.Address{addr})); err != nil {
		t.Fatalf("apply extra: %v", err)
	}

	author, err := engine.Author(header)
	if err != nil {
		t.Fatalf("Author() failed: %v", err)
	}
	if author != addr {
		t.Fatalf("Author() mismatch: have %s, want %s", author.Hex(), addr.Hex())
	}
}
