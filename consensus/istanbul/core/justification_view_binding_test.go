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

// -----------------------------------------------------------------------------
// View-binding regression tests for PRE-PREPARE / ROUND-CHANGE justification.
//
// These tests cover the requirement, from the EEA QBFT specification's
// validRoundChange predicate, that every ROUND-CHANGE message used to justify a
// PRE-PREPARE for view (sequence, round) must itself be for that same view. The
// missing binding allowed a proposer to replay validly-signed ROUND-CHANGE
// messages from an earlier round as the certificate for a new round.
//
// A ROUND-CHANGE message's own Round (the round it changes TO) is distinct from
// its PreparedRound (the round of any block it locked earlier). Only the former
// is bound to the PRE-PREPARE round; the latter is legitimately lower and keeps
// its own separate checks. The tests below assert both that stale rounds are
// rejected and that legitimate lower prepared rounds are still accepted.
// -----------------------------------------------------------------------------

// createRoundChangeMessageForRound builds a signed RC payload whose own
// sequence/round are explicitly set (unlike createRoundChangeMessage, which
// hard-codes round 1). Source is set directly, mirroring the post-verification
// state where verifySignatures has already recovered the signer.
func createRoundChangeMessageForRound(from common.Address, sequence, round, preparedRound int64, preparedBlock istanbul.Proposal) *qbfttypes.SignedRoundChangePayload {
	var pr *big.Int
	if preparedRound != 0 {
		pr = big.NewInt(preparedRound)
	}
	m := qbfttypes.NewRoundChange(big.NewInt(sequence), big.NewInt(round), pr, preparedBlock, false)
	m.SetSource(from)
	return &m.SignedRoundChangePayload
}

// --- isJustified: nil-prepared path -----------------------------------------

// A quorum of validly-formed nil-prepared ROUND-CHANGE messages whose own round
// differs from the target round must be rejected. This is the core of the
// security report: round-(N-1) ROUND-CHANGE messages replayed to justify a
// round-N PRE-PREPARE.
func TestIsJustified_RejectsStaleRoundChangeRoundNilPath(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(1)
	targetRound := int64(2)
	staleRound := int64(1) // RCs are for the previous round

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), targetSequence, staleRound, 0, nil))
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection of stale-round ROUND-CHANGE justification (nil path), but got nil")
	}
}

// A quorum of nil-prepared ROUND-CHANGE messages whose own sequence differs from
// the target sequence must be rejected (height binding).
func TestIsJustified_RejectsStaleRoundChangeSequenceNilPath(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(2)
	staleSequence := int64(1)
	targetRound := int64(2)

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), staleSequence, targetRound, 0, nil))
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection of stale-sequence ROUND-CHANGE justification (nil path), but got nil")
	}
}

// Positive control: a quorum of nil-prepared ROUND-CHANGE messages for the
// correct target view must still be accepted after the fix.
func TestIsJustified_AcceptsMatchingViewNilPath(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(1)
	targetRound := int64(2)

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), targetSequence, targetRound, 0, nil))
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet)
	if err != nil {
		t.Fatalf("expected matching-view nil-path justification to succeed; got: %v", err)
	}
}

// --- isJustified: prepared-round path ----------------------------------------

// ROUND-CHANGE messages carrying a valid prepared certificate but whose own
// round is stale must still be rejected: view binding is enforced before the
// prepared-round quorum logic runs.
func TestIsJustified_RejectsStaleRoundChangeRoundPreparedPath(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(1)
	targetRound := int64(5)
	staleRound := int64(4)    // RC's own round is one behind the target
	preparedRound := int64(2) // a legitimately-lower prepared round

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), targetSequence, staleRound, preparedRound, block))
	}

	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, createPrepareMessage(validatorSet.List()[i].Address(), preparedRound, block))
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, prepareMessages, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection of stale-round ROUND-CHANGE justification (prepared path), but got nil")
	}
}

// Positive control for the prepared-round path: ROUND-CHANGE messages whose own
// round matches the target round, but whose prepared round is legitimately
// lower, must be accepted. This guards against the fix being too strict and
// rejecting valid re-proposals of an earlier-prepared block.
func TestIsJustified_AcceptsMatchingRoundWithLowerPreparedRound(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(1)
	targetRound := int64(5)
	preparedRound := int64(2) // lower than target round — legitimate

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), targetSequence, targetRound, preparedRound, block))
	}

	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, createPrepareMessage(validatorSet.List()[i].Address(), preparedRound, block))
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, prepareMessages, quorumSize, validatorSet)
	if err != nil {
		t.Fatalf("expected matching-round justification with lower prepared round to succeed; got: %v", err)
	}
}

// --- isJustified: nil-field and nil-target guards ----------------------------

// A ROUND-CHANGE message with a nil own-round must be rejected rather than
// panicking on the big.Int comparison.
func TestIsJustified_RejectsNilRoundChangeRound(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize-1; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), 1, 2, 0, nil))
	}
	// One RC with a nil Round (and nil Sequence) built directly.
	nilRC := &qbfttypes.SignedRoundChangePayload{}
	nilRC.SetSource(validatorSet.List()[quorumSize-1].Address())
	roundChangeMessages = append(roundChangeMessages, nilRC)

	err := isJustified(big.NewInt(1), big.NewInt(2), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection of ROUND-CHANGE with nil round, but got nil")
	}
}

// A nil target round or sequence must be rejected up front.
func TestIsJustified_RejectsNilTargetView(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		roundChangeMessages = append(roundChangeMessages,
			createRoundChangeMessageForRound(validatorSet.List()[i].Address(), 1, 2, 0, nil))
	}

	if err := isJustified(nil, big.NewInt(2), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet); err == nil {
		t.Fatalf("expected rejection of nil target sequence, but got nil")
	}
	if err := isJustified(big.NewInt(1), nil, block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet); err == nil {
		t.Fatalf("expected rejection of nil target round, but got nil")
	}
}

// A mixed set where only some ROUND-CHANGE messages are for the target round
// must be rejected: every message in the justification must bind to the view,
// so a single stale message anywhere in the set fails the whole certificate.
func TestIsJustified_RejectsMixedStaleAndCurrentRounds(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetSequence := int64(1)
	targetRound := int64(2)

	roundChangeMessages := []*qbfttypes.SignedRoundChangePayload{
		createRoundChangeMessageForRound(validatorSet.List()[0].Address(), targetSequence, targetRound, 0, nil),
		createRoundChangeMessageForRound(validatorSet.List()[1].Address(), targetSequence, targetRound, 0, nil),
		// stale one mixed in
		createRoundChangeMessageForRound(validatorSet.List()[2].Address(), targetSequence, targetRound-1, 0, nil),
	}

	err := isJustified(big.NewInt(targetSequence), big.NewInt(targetRound), block, roundChangeMessages, []*qbfttypes.Prepare{}, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection when a stale ROUND-CHANGE is mixed into an otherwise-valid set, but got nil")
	}
}

// -----------------------------------------------------------------------------
// End-to-end test through the production handleEncodedMsg path.
//
// This mirrors the attack vector in the security report exactly: a fully signed,
// RLP-encoded PRE-PREPARE for round 2, justified by validly-signed round-1
// ROUND-CHANGE payloads, submitted through the same entry point a real peer
// message would take. It asserts the node rejects the message, does not advance
// to StatePreprepared, and does not broadcast a PREPARE.
// -----------------------------------------------------------------------------

type viewBindingBackend struct {
	addr       common.Address
	broadcasts int
}

func (b *viewBindingBackend) Address() common.Address { return b.addr }
func (b *viewBindingBackend) Validators(istanbul.Proposal) istanbul.ValidatorSet {
	return nil
}
func (b *viewBindingBackend) EventMux() *event.TypeMux { return new(event.TypeMux) }
func (b *viewBindingBackend) Broadcast(istanbul.ValidatorSet, uint64, []byte) error {
	b.broadcasts++
	return nil
}
func (b *viewBindingBackend) Gossip(istanbul.ValidatorSet, uint64, []byte) error { return nil }
func (b *viewBindingBackend) Commit(istanbul.Proposal, [][]byte, *big.Int) error { return nil }
func (b *viewBindingBackend) Verify(istanbul.Proposal) (time.Duration, error)    { return 0, nil }
func (b *viewBindingBackend) Sign([]byte) ([]byte, error)                        { return nil, nil }
func (b *viewBindingBackend) SignWithoutHashing([]byte) ([]byte, error)          { return nil, nil }
func (b *viewBindingBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}
func (b *viewBindingBackend) LastProposal() (istanbul.Proposal, common.Address) {
	return nil, common.Address{}
}
func (b *viewBindingBackend) HasPropsal(common.Hash, *big.Int) bool { return false }
func (b *viewBindingBackend) GetProposer(uint64) common.Address     { return common.Address{} }
func (b *viewBindingBackend) ParentValidators(istanbul.Proposal) istanbul.ValidatorSet {
	return nil
}
func (b *viewBindingBackend) HasBadProposal(common.Hash) bool { return false }
func (b *viewBindingBackend) Close() error                    { return nil }
func (b *viewBindingBackend) StartQBFTConsensus() error       { return nil }

func signQBFTMessage(t *testing.T, msg qbfttypes.QBFTMessage, key *ecdsa.PrivateKey) {
	t.Helper()
	payload, err := msg.EncodePayloadForSigning()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(crypto.Keccak256(payload), key)
	if err != nil {
		t.Fatal(err)
	}
	msg.SetSignature(sig)
}

func encodePreprepare(t *testing.T, preprepare *qbfttypes.Preprepare) ([]byte, error) {
	t.Helper()
	return rlp.EncodeToBytes(preprepare)
}

func blockWithCoinbase(number int64, timestamp uint64, coinbase common.Address) *types.Block {
	return new(types.Block).WithSeal(&types.Header{
		Difficulty: big.NewInt(0),
		Number:     big.NewInt(number),
		GasLimit:   1,
		Time:       timestamp,
		Coinbase:   coinbase,
	})
}

// buildViewBindingCore wires up a core positioned at view (sequence=1, round=2)
// with an existing locked block, returning the core, backend, the elected
// round-2 proposer address, and a key-by-address map for signing.
func buildViewBindingCore(t *testing.T) (*core, *viewBindingBackend, common.Address, map[common.Address]*ecdsa.PrivateKey, *types.Block) {
	t.Helper()

	keys := make([]*ecdsa.PrivateKey, 4)
	addrs := make([]common.Address, 4)
	keyByAddr := make(map[common.Address]*ecdsa.PrivateKey, 4)
	for i := range keys {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
		addrs[i] = crypto.PubkeyToAddress(key.PublicKey)
		keyByAddr[addrs[i]] = key
	}

	policy := istanbul.NewRoundRobinProposerPolicy()
	policy.Use(istanbul.ValidatorSortByByte())
	valSet := validator.NewSet(addrs, policy)
	valSet.CalcProposer(common.Address{}, 2)

	lockedBlock := makeBlock(1)
	view := &istanbul.View{Sequence: big.NewInt(1), Round: big.NewInt(2)}
	backend := &viewBindingBackend{addr: valSet.List()[0].Address()}

	c := &core{
		address:     backend.addr,
		state:       StateAcceptRequest,
		logger:      log.New(),
		cleanLogger: log.New(),
		backend:     backend,
		valSet:      valSet,
		validateFn: func(data []byte, sig []byte) (common.Address, error) {
			return istanbul.CheckValidatorSignature(valSet, data, sig)
		},
		backlogs:          make(map[common.Address]*prque.Prque),
		backlogsMu:        new(sync.Mutex),
		currentMutex:      new(sync.Mutex),
		current:           newRoundState(view, valSet, nil, big.NewInt(1), lockedBlock, nil, func(common.Hash) bool { return false }),
		roundChangeSet:    newRoundChangeSet(valSet),
		pendingRequests:   prque.New(nil),
		pendingRequestsMu: new(sync.Mutex),
	}

	return c, backend, valSet.GetProposer().Address(), keyByAddr, lockedBlock
}

// The exact security-report scenario, hardened against: a round-2 PRE-PREPARE
// justified by round-1 ROUND-CHANGE payloads must be rejected at the production
// entry point, with no state advance and no PREPARE broadcast.
func TestHandleEncodedMsg_RejectsStaleRoundChangeJustification(t *testing.T) {
	c, backend, proposer, keyByAddr, _ := buildViewBindingCore(t)

	// Attacker (round-2 proposer) builds a conflicting block crediting itself as
	// coinbase, so the coinbase-binding check is satisfied and we test the
	// justification logic specifically.
	conflictingBlock := blockWithCoinbase(1, 2, proposer)

	preprepare := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(2), conflictingBlock)
	signQBFTMessage(t, preprepare, keyByAddr[proposer])

	// Embed validly-signed round-1 ROUND-CHANGE payloads (stale view).
	for i, v := range c.valSet.List() {
		if i == 3 {
			break
		}
		rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), nil, nil, false)
		signQBFTMessage(t, rc, keyByAddr[v.Address()])
		preprepare.JustificationRoundChanges = append(preprepare.JustificationRoundChanges, &rc.SignedRoundChangePayload)
	}

	payload, err := encodePreprepare(t, preprepare)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.handleEncodedMsg(qbfttypes.PreprepareCode, payload); err == nil {
		t.Fatalf("expected stale-round PRE-PREPARE to be rejected, but it was accepted")
	}
	if c.state == StatePreprepared {
		t.Fatalf("node advanced to StatePreprepared on a stale-round justification")
	}
	if c.current.Proposal() != nil && c.current.Proposal().Hash() == conflictingBlock.Hash() {
		t.Fatalf("node accepted the conflicting proposal")
	}
	if backend.broadcasts != 0 {
		t.Fatalf("node broadcast a PREPARE for a stale-justified PRE-PREPARE")
	}
}

// Positive control through the production path: a round-2 PRE-PREPARE justified
// by correctly-signed round-2 ROUND-CHANGE payloads must be accepted, advance to
// StatePreprepared, and broadcast a PREPARE. This proves the fix does not break
// legitimate round changes.
func TestHandleEncodedMsg_AcceptsCurrentRoundChangeJustification(t *testing.T) {
	c, backend, proposer, keyByAddr, _ := buildViewBindingCore(t)

	proposedBlock := blockWithCoinbase(1, 2, proposer)

	preprepare := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(2), proposedBlock)
	signQBFTMessage(t, preprepare, keyByAddr[proposer])

	// Embed validly-signed round-2 ROUND-CHANGE payloads (matching view).
	for i, v := range c.valSet.List() {
		if i == 3 {
			break
		}
		rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(2), nil, nil, false)
		signQBFTMessage(t, rc, keyByAddr[v.Address()])
		preprepare.JustificationRoundChanges = append(preprepare.JustificationRoundChanges, &rc.SignedRoundChangePayload)
	}

	payload, err := encodePreprepare(t, preprepare)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.handleEncodedMsg(qbfttypes.PreprepareCode, payload); err != nil {
		t.Fatalf("expected current-round PRE-PREPARE to be accepted, got: %v", err)
	}
	if c.state != StatePreprepared {
		t.Fatalf("expected state to advance to StatePreprepared, got %s", c.state)
	}
	if c.current.Proposal() == nil || c.current.Proposal().Hash() != proposedBlock.Hash() {
		t.Fatalf("expected the proposed block to be accepted")
	}
	if backend.broadcasts == 0 {
		t.Fatalf("expected a PREPARE broadcast for a valid PRE-PREPARE")
	}
}
