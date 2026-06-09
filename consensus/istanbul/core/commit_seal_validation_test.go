// Copyright 2024 The Electroneum Authors
// This file is part of the electroneum-sc library.
//
// Tests for committed-seal validation on the COMMIT path. These guard against a
// Byzantine validator getting a malformed CommitSeal counted toward quorum and
// copied verbatim into the block header by commitQBFT (the receive/verify path
// already rejects such seals in Engine.verifyCommittedSeals; these tests assert
// the local commit path is symmetric).

package core

import (
	"crypto/ecdsa"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/prque"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/event"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/rlp"
)

// commitCaptureBackend is a minimal istanbul.Backend stub for driving commitQBFT.
// It captures the seals handed to Commit so a test can recover them exactly as
// Engine.Signers does, and records whether Commit/Broadcast were reached so the
// commitQBFT length guard (which round-changes instead of committing) is observable.
type commitCaptureBackend struct {
	addr           common.Address
	committedSeals [][]byte
	committed      bool
	broadcasts     int
	signKey        *ecdsa.PrivateKey
}

func (b *commitCaptureBackend) Address() common.Address                            { return b.addr }
func (b *commitCaptureBackend) Validators(istanbul.Proposal) istanbul.ValidatorSet { return nil }
func (b *commitCaptureBackend) EventMux() *event.TypeMux                           { return new(event.TypeMux) }
func (b *commitCaptureBackend) Broadcast(istanbul.ValidatorSet, uint64, []byte) error {
	b.broadcasts++
	return nil
}
func (b *commitCaptureBackend) Gossip(istanbul.ValidatorSet, uint64, []byte) error { return nil }
func (b *commitCaptureBackend) Commit(_ istanbul.Proposal, seals [][]byte, _ *big.Int) error {
	b.committed = true
	b.committedSeals = seals
	return nil
}
func (b *commitCaptureBackend) Verify(istanbul.Proposal) (time.Duration, error) { return 0, nil }
func (b *commitCaptureBackend) Sign(data []byte) ([]byte, error) {
	return crypto.Sign(crypto.Keccak256(data), b.signKey)
}
func (b *commitCaptureBackend) SignWithoutHashing(data []byte) ([]byte, error) {
	return crypto.Sign(data, b.signKey)
}
func (b *commitCaptureBackend) CheckSignature([]byte, common.Address, []byte) error { return nil }
func (b *commitCaptureBackend) LastProposal() (istanbul.Proposal, common.Address) {
	return nil, common.Address{}
}
func (b *commitCaptureBackend) HasPropsal(common.Hash, *big.Int) bool { return false }
func (b *commitCaptureBackend) GetProposer(uint64) common.Address     { return common.Address{} }
func (b *commitCaptureBackend) ParentValidators(istanbul.Proposal) istanbul.ValidatorSet {
	return nil
}
func (b *commitCaptureBackend) HasBadProposal(common.Hash) bool { return false }
func (b *commitCaptureBackend) Close() error                    { return nil }
func (b *commitCaptureBackend) StartQBFTConsensus() error       { return nil }

// newKeyedValidatorSet returns a validator set of n validators along with their
// private keys, indexed so that keys[addr] gives the signing key for a validator.
func newKeyedValidatorSet(t *testing.T, n int) (istanbul.ValidatorSet, map[common.Address]*ecdsa.PrivateKey) {
	t.Helper()
	addrs := make([]common.Address, n)
	keys := make(map[common.Address]*ecdsa.PrivateKey, n)
	for i := 0; i < n; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("failed to generate key: %v", err)
		}
		addr := crypto.PubkeyToAddress(key.PublicKey)
		addrs[i] = addr
		keys[addr] = key
	}
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	return validator.NewSet(addrs, pp), keys
}

// makeProposalBlock builds a minimal block carrying a decodable QBFT extra so
// PrepareCommittedSeal (which RLP-hashes the filtered header) works.
func makeProposalBlock(t *testing.T, number int64) *types.Block {
	t.Helper()
	extra, err := rlp.EncodeToBytes(&types.QBFTExtra{
		VanityData:    make([]byte, types.IstanbulExtraVanity),
		Validators:    []common.Address{},
		Round:         0,
		CommittedSeal: [][]byte{},
	})
	if err != nil {
		t.Fatalf("failed to encode qbft extra: %v", err)
	}
	header := &types.Header{
		Number:     big.NewInt(number),
		GasLimit:   8000000,
		Difficulty: big.NewInt(1),
		MixDigest:  types.IstanbulDigest,
		Extra:      extra,
	}
	return types.NewBlockWithHeader(header)
}

// newCommitTestCore builds a core with the given proposal installed as the
// current round's PRE-PREPARE, at the given round.
func newCommitTestCore(valSet istanbul.ValidatorSet, proposal istanbul.Proposal, seq, round int64) *core {
	view := &istanbul.View{Sequence: big.NewInt(seq), Round: big.NewInt(round)}
	preprepare := qbfttypes.NewPreprepare(big.NewInt(seq), big.NewInt(round), proposal)
	return &core{
		address:           valSet.List()[0].Address(),
		state:             StateAcceptRequest,
		logger:            log.New(),
		valSet:            valSet,
		backlogs:          make(map[common.Address]*prque.Prque),
		backlogsMu:        new(sync.Mutex),
		pendingRequests:   prque.New(nil),
		pendingRequestsMu: new(sync.Mutex),
		current:           newRoundState(view, valSet, preprepare, nil, nil, nil, func(common.Hash) bool { return false }),
	}
}

// signCommitSeal produces a valid committed seal for the proposal at the round.
func signCommitSeal(t *testing.T, key *ecdsa.PrivateKey, block *types.Block, round uint32) []byte {
	t.Helper()
	seal, err := crypto.Sign(PrepareCommittedSeal(block.Header(), round), key)
	if err != nil {
		t.Fatalf("failed to sign commit seal: %v", err)
	}
	return seal
}

// makeCommit builds a COMMIT message with the given seal, source already set.
func makeCommit(seq, round int64, block *types.Block, src common.Address, seal []byte) *qbfttypes.Commit {
	c := qbfttypes.NewCommit(big.NewInt(seq), big.NewInt(round), block.Hash(), seal)
	c.SetSource(src)
	c.SetSignature([]byte{0x01})
	return c
}

// TestVerifyCommittedSeal_AcceptsValidSeal confirms an honest, correctly signed
// seal passes verification at the current round.
func TestVerifyCommittedSeal_AcceptsValidSeal(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	seal := signCommitSeal(t, keys[src], block, 0)
	commit := makeCommit(1, 0, block, src, seal)

	if err := c.verifyCommittedSeal(commit); err != nil {
		t.Fatalf("valid seal was rejected: %v", err)
	}
}

// TestVerifyCommittedSeal_RejectsMalformedSeal is the regression test for the
// reported bug: a validator with a correct outer signature/digest but a
// malformed CommitSeal must be rejected before counting toward quorum.
func TestVerifyCommittedSeal_RejectsMalformedSeal(t *testing.T) {
	valSet, _ := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	malformed := make([]byte, types.IstanbulExtraSeal) // right length, all-zero garbage
	commit := makeCommit(1, 0, block, src, malformed)

	if err := c.verifyCommittedSeal(commit); err != errInvalidCommittedSeal {
		t.Fatalf("expected errInvalidCommittedSeal, got %v", err)
	}
}

// TestVerifyCommittedSeal_RejectsWrongLength ensures a seal of the wrong length
// can never reach the header.
func TestVerifyCommittedSeal_RejectsWrongLength(t *testing.T) {
	valSet, _ := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	commit := makeCommit(1, 0, block, src, []byte{0x01, 0x02, 0x03})

	if err := c.verifyCommittedSeal(commit); err != errInvalidCommittedSeal {
		t.Fatalf("expected errInvalidCommittedSeal for short seal, got %v", err)
	}
}

// TestVerifyCommittedSeal_RejectsSealFromAnotherValidator ensures a seal that
// recovers to a validator other than the message source is rejected (a
// Byzantine validator cannot replay another validator's seal under its own
// COMMIT to manufacture a vote).
func TestVerifyCommittedSeal_RejectsSealFromAnotherValidator(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	other := valSet.List()[2].Address()
	// Seal validly signed by `other`, but attributed to `src`.
	seal := signCommitSeal(t, keys[other], block, 0)
	commit := makeCommit(1, 0, block, src, seal)

	if err := c.verifyCommittedSeal(commit); err != errInvalidCommittedSeal {
		t.Fatalf("expected errInvalidCommittedSeal for mismatched signer, got %v", err)
	}
}

// TestVerifyCommittedSeal_RoundChangeSealAccepted guards against a liveness
// regression: when consensus advances to a higher round, an honest seal is
// signed over that round and must still be accepted (the check keys off the
// COMMIT's own round, not round 0).
func TestVerifyCommittedSeal_RoundChangeSealAccepted(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	const round int64 = 3
	c := newCommitTestCore(valSet, block, 1, round)

	src := valSet.List()[1].Address()
	seal := signCommitSeal(t, keys[src], block, uint32(round))
	commit := makeCommit(1, round, block, src, seal)

	if err := c.verifyCommittedSeal(commit); err != nil {
		t.Fatalf("valid round-%d seal was rejected: %v", round, err)
	}
}

// TestVerifyCommittedSeal_WrongRoundSealRejected confirms a seal signed for a
// different round than the COMMIT claims is rejected.
func TestVerifyCommittedSeal_WrongRoundSealRejected(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 2)

	src := valSet.List()[1].Address()
	// Seal signed for round 0 but the COMMIT is for round 2.
	seal := signCommitSeal(t, keys[src], block, 0)
	commit := makeCommit(1, 2, block, src, seal)

	if err := c.verifyCommittedSeal(commit); err != errInvalidCommittedSeal {
		t.Fatalf("expected errInvalidCommittedSeal for wrong-round seal, got %v", err)
	}
}

// TestHandleCommitMsg_RejectsMalformedSeal exercises the full handler path and
// asserts the malformed COMMIT is neither counted toward quorum nor able to
// trigger a commit.
func TestHandleCommitMsg_RejectsMalformedSeal(t *testing.T) {
	valSet, _ := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	malformed := make([]byte, types.IstanbulExtraSeal)
	commit := makeCommit(1, 0, block, src, malformed)

	err := c.handleCommitMsg(commit)
	if err != errInvalidCommittedSeal {
		t.Fatalf("expected errInvalidCommittedSeal from handleCommitMsg, got %v", err)
	}
	if got := c.current.QBFTCommits.Size(); got != 0 {
		t.Fatalf("malformed COMMIT was counted toward quorum: size=%d", got)
	}
}

// TestHandleCommitMsg_AcceptsValidSeal confirms an honest COMMIT is counted.
func TestHandleCommitMsg_AcceptsValidSeal(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	c := newCommitTestCore(valSet, block, 1, 0)

	src := valSet.List()[1].Address()
	seal := signCommitSeal(t, keys[src], block, 0)
	commit := makeCommit(1, 0, block, src, seal)

	if err := c.handleCommitMsg(commit); err != nil {
		t.Fatalf("valid COMMIT rejected by handler: %v", err)
	}
	if got := c.current.QBFTCommits.Size(); got != 1 {
		t.Fatalf("valid COMMIT not counted: size=%d", got)
	}
}

// newCommitQBFTTestCore is like newCommitTestCore but fully wires the fields
// commitQBFT and broadcastNextRoundChange depend on: a backend (for Commit and
// the round-change broadcast) and a roundChangeSet/currentMutex.
func newCommitQBFTTestCore(valSet istanbul.ValidatorSet, backend istanbul.Backend, proposal istanbul.Proposal, seq, round int64) *core {
	view := &istanbul.View{Sequence: big.NewInt(seq), Round: big.NewInt(round)}
	preprepare := qbfttypes.NewPreprepare(big.NewInt(seq), big.NewInt(round), proposal)
	return &core{
		address:           backend.Address(),
		state:             StatePrepared,
		logger:            log.New(),
		cleanLogger:       log.New(),
		backend:           backend,
		valSet:            valSet,
		backlogs:          make(map[common.Address]*prque.Prque),
		backlogsMu:        new(sync.Mutex),
		currentMutex:      new(sync.Mutex),
		pendingRequests:   prque.New(nil),
		pendingRequestsMu: new(sync.Mutex),
		roundChangeSet:    newRoundChangeSet(valSet),
		current:           newRoundState(view, valSet, preprepare, nil, nil, nil, func(common.Hash) bool { return false }),
	}
}

// TestCommitQBFT_SealsMatchNetworkVerification is the end-to-end symmetry proof:
// a real quorum of validly-signed COMMITs is driven through handleCommitMsg, and
// the seals commitQBFT hands to backend.Commit are recovered exactly as
// Engine.Signers does (over PrepareCommittedSeal at the committed round) and must
// all map to distinct validators. This is what guarantees a block this node
// builds will pass Engine.verifyCommittedSeals network-wide.
func TestCommitQBFT_SealsMatchNetworkVerification(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	const round int64 = 0

	proposerKey := keys[valSet.List()[0].Address()]
	backend := &commitCaptureBackend{addr: valSet.List()[0].Address(), signKey: proposerKey}
	c := newCommitQBFTTestCore(valSet, backend, block, 1, round)

	// Feed a quorum of valid COMMITs from distinct validators.
	quorum := c.QuorumSize()
	signers := make(map[common.Address]struct{})
	for i := 0; i < quorum; i++ {
		src := valSet.List()[i].Address()
		signers[src] = struct{}{}
		seal := signCommitSeal(t, keys[src], block, uint32(round))
		if err := c.handleCommitMsg(makeCommit(1, round, block, src, seal)); err != nil {
			t.Fatalf("valid COMMIT %d rejected: %v", i, err)
		}
	}

	if !backend.committed {
		t.Fatalf("quorum of valid COMMITs did not trigger commit")
	}
	if len(backend.committedSeals) != quorum {
		t.Fatalf("expected %d committed seals, got %d", quorum, len(backend.committedSeals))
	}

	// Recover each committed seal exactly as Engine.Signers / verifyCommittedSeals
	// does: over PrepareCommittedSeal(header, committedRound). Each must resolve to
	// one of the validators that sent a COMMIT.
	proposalSeal := PrepareCommittedSeal(block.Header(), uint32(round))
	recovered := make(map[common.Address]struct{})
	for i, seal := range backend.committedSeals {
		addr, err := istanbul.GetSignatureAddressNoHashing(proposalSeal, seal)
		if err != nil {
			t.Fatalf("committed seal %d failed to recover (would fail verifyCommittedSeals): %v", i, err)
		}
		if _, ok := signers[addr]; !ok {
			t.Fatalf("committed seal %d recovered to %s, not a committing validator", i, addr.Hex())
		}
		recovered[addr] = struct{}{}
	}
	if len(recovered) != quorum {
		t.Fatalf("expected %d distinct seal signers, got %d", quorum, len(recovered))
	}
}

// TestCommitQBFT_RefusesMalformedSeal exercises the defense-in-depth length guard
// inside commitQBFT: if a malformed seal somehow reaches QBFTCommits without going
// through handleCommitMsg's check, commitQBFT must refuse to commit and instead
// round-change, so a malformed seal can never be copied into the block header.
func TestCommitQBFT_RefusesMalformedSeal(t *testing.T) {
	valSet, keys := newKeyedValidatorSet(t, 4)
	block := makeProposalBlock(t, 1)
	const round int64 = 0

	backend := &commitCaptureBackend{addr: valSet.List()[0].Address(), signKey: keys[valSet.List()[0].Address()]}
	c := newCommitQBFTTestCore(valSet, backend, block, 1, round)

	// Add a quorum-worth of COMMITs directly to the set, bypassing handleCommitMsg,
	// with one carrying a wrong-length (malformed) seal.
	quorum := c.QuorumSize()
	for i := 0; i < quorum; i++ {
		src := valSet.List()[i].Address()
		seal := signCommitSeal(t, keys[src], block, uint32(round))
		if i == quorum-1 {
			seal = []byte{0x01, 0x02, 0x03} // malformed length
		}
		if err := c.current.QBFTCommits.Add(makeCommit(1, round, block, src, seal)); err != nil {
			t.Fatalf("failed to seed QBFTCommits: %v", err)
		}
	}

	c.commitQBFT()

	if backend.committed {
		t.Fatalf("commitQBFT committed a block containing a malformed seal")
	}
	if backend.broadcasts == 0 {
		t.Fatalf("commitQBFT did not round-change after seeing a malformed seal")
	}
}
