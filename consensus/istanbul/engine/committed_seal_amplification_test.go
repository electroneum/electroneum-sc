package qbftengine

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
)

// buildSealedHeader builds a parent/child header pair carrying an N-validator
// set, and writes `seals` as the child's committed-seal array. The returned
// engine, child header, parent and validator set are ready for VerifyHeader.
func buildSealedHeader(t *testing.T, addrs []common.Address, seals [][]byte) (*Engine, *types.Header, *types.Header, istanbul.ValidatorSet) {
	t.Helper()

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
	// writeCommittedSeals rejects an empty array, so only write seals when the
	// caller supplied some. Callers that pass no seals just want the header shell
	// (e.g. to compute the commit payload before signing).
	applies := []ApplyQBFTExtra{WriteValidators(addrs), writeRoundNumber(big.NewInt(1))}
	if len(seals) > 0 {
		applies = append(applies, writeCommittedSeals(seals))
	}
	if err := ApplyHeaderQBFTExtra(hdr, applies...); err != nil {
		t.Fatalf("apply extra hdr: %v", err)
	}

	valSet := validator.NewSet(addrs, istanbul.NewProposerPolicy(istanbul.RoundRobin))
	return engine, hdr, parent, valSet
}

func genValidators(t *testing.T, n int) ([]*ecdsa.PrivateKey, []common.Address) {
	t.Helper()
	keys := make([]*ecdsa.PrivateKey, n)
	addrs := make([]common.Address, n)
	for i := 0; i < n; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("keygen failed: %v", err)
		}
		keys[i] = k
		addrs[i] = crypto.PubkeyToAddress(k.PublicKey)
	}
	return keys, addrs
}

// TestCommittedSealAmplificationRejected is the regression test for the
// committed-seal amplification DoS: a header carrying far more committed seals
// than there are validators must be rejected by verifyCommittedSeals BEFORE
// Signers() pays one ecrecover per seal.
//
// The attack header sets Coinbase to a real validator (the only pre-fork gate)
// and stuffs the committed-seal array with thousands of well-formed throwaway
// signatures. Before the fix, verifyCommittedSeals recovered every one of them;
// after the fix the length cap rejects the header structurally.
func TestCommittedSealAmplificationRejected(t *testing.T) {
	const N = 4
	_, addrs := genValidators(t, N)

	// One throwaway key produces many well-formed, non-validator seals. Each is a
	// valid secp256k1 signature, so none short-circuits the recovery loop -- the
	// only thing that stops them is the length cap.
	attacker, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	// Build the header once to get the exact commit payload it will be signed over.
	_, hdr, _, _ := buildSealedHeader(t, addrs, [][]byte{})
	payload := PrepareCommittedSeal(hdr, 1)

	const fatCount = 1500 // ~ the 100 KB SanityCheck ceiling; >> N
	seals := make([][]byte, fatCount)
	for i := 0; i < fatCount; i++ {
		sig, err := crypto.Sign(payload, attacker)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		seals[i] = sig
	}

	engine, fatHdr, parent, valSet := buildSealedHeader(t, addrs, seals)

	err = engine.VerifyHeader(nil, fatHdr, []*types.Header{parent}, valSet)
	if err == nil {
		t.Fatalf("oversized committed-seal array (%d seals, %d validators) was ACCEPTED; expected rejection", fatCount, N)
	}
	if err != istanbulcommon.ErrInvalidCommittedSeals {
		t.Fatalf("expected ErrInvalidCommittedSeals, got %v", err)
	}
}

// TestCommittedSealAtValidatorCountPasses guards against a false positive: a
// legitimate header signed by all N validators has exactly N seals, which sits
// right at the cap boundary (len(seals) == N) and must still pass.
func TestCommittedSealAtValidatorCountPasses(t *testing.T) {
	const N = 4
	keys, addrs := genValidators(t, N)

	_, hdr, _, _ := buildSealedHeader(t, addrs, [][]byte{})
	payload := PrepareCommittedSeal(hdr, 1)

	seals := make([][]byte, N)
	for i := 0; i < N; i++ {
		sig, err := crypto.Sign(payload, keys[i])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		seals[i] = sig
	}

	engine, fullHdr, parent, valSet := buildSealedHeader(t, addrs, seals)
	if err := engine.VerifyHeader(nil, fullHdr, []*types.Header{parent}, valSet); err != nil {
		t.Fatalf("header with exactly N=%d seals (all validators) was rejected: %v", N, err)
	}
}

// TestCommittedSealsIgnoreEmbeddedValidatorList checks that a header's own
// extra.Validators list plays no part in committed-seal verification. That list
// is written by the proposer and not checked against the snapshot, so a proposer
// can ship it empty. Proposal verification passes such a block (it has no seals
// yet), so honest validators PREPARE and COMMIT it; the finalized block with
// their quorum seals attached must then verify too, or the network is left
// locked on a block nobody can import.
func TestCommittedSealsIgnoreEmbeddedValidatorList(t *testing.T) {
	const N = 4
	keys, addrs := genValidators(t, N)

	engine, hdr, parent, valSet := buildSealedHeader(t, addrs, nil)
	if err := ApplyHeaderQBFTExtra(hdr, WriteValidators(nil)); err != nil {
		t.Fatalf("clear embedded validators: %v", err)
	}

	// Proposal stage: no seals yet, so only the empty-seals error comes back.
	if err := engine.VerifyHeader(nil, hdr, []*types.Header{parent}, valSet); err != istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("proposal with empty embedded validators: have %v, want %v", err, istanbulcommon.ErrEmptyCommittedSeals)
	}

	// Commit stage: a 2F+1 quorum (3 of 4) of real validators seal it.
	payload := PrepareCommittedSeal(hdr, 1)
	quorum := 3
	seals := make([][]byte, quorum)
	for i := 0; i < quorum; i++ {
		sig, err := crypto.Sign(payload, keys[i])
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		seals[i] = sig
	}
	if err := engine.CommitHeader(hdr, seals, big.NewInt(1)); err != nil {
		t.Fatalf("commit header: %v", err)
	}

	if err := engine.VerifyHeader(nil, hdr, []*types.Header{parent}, valSet); err != nil {
		t.Fatalf("finalized block with quorum seals and empty embedded validators was rejected: %v", err)
	}
	signers, err := engine.Signers(hdr)
	if err != nil {
		t.Fatalf("Signers: %v", err)
	}
	if len(signers) != quorum {
		t.Fatalf("signer count: have %d, want %d", len(signers), quorum)
	}
	for i, addr := range signers {
		if addr != addrs[i] {
			t.Fatalf("signer %d: have %x, want %x", i, addr, addrs[i])
		}
	}
}
