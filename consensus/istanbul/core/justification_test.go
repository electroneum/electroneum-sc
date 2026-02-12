package core

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
)

// Tests combinations of justifications that evaluate to true.
func TestJustifyTrue(t *testing.T) {
	for quorumSize := 3; quorumSize <= 10; quorumSize++ {
		// All ROUND-CHANGE messages have pr/pb nil
		testParameterizedCase(t, quorumSize, quorumSize, 0, 0, 0, 0, 0, true)

		// Some ROUND-CHANGE message has pr/pb not nil
		for equal := 1; equal <= quorumSize; equal++ {
			for less := 0; less <= quorumSize-equal; less++ {
				nil := quorumSize - equal - less
				testParameterizedCase(t, quorumSize, nil, equal, less, 0, quorumSize, 0, true)
			}
		}
	}
}

// Tests combinations of justifications that evaluate to false.
func TestJustifyFalse(t *testing.T) {
	for quorumSize := 3; quorumSize <= 10; quorumSize++ {
		// Total ROUND-CHANGE messages less than quorumSize
		// all have pr/pb nil
		for totalRoundChange := 0; totalRoundChange < quorumSize; totalRoundChange++ {
			testParameterizedCase(t, quorumSize, totalRoundChange, 0, 0, 0, 0, 0, false)
		}
		// some has pr/pb not nil
		for totalRoundChange := 0; totalRoundChange < quorumSize; totalRoundChange++ {
			for equal := 1; equal <= totalRoundChange; equal++ {
				for less := 0; less <= totalRoundChange-equal; less++ {
					nil := totalRoundChange - equal - less
					testParameterizedCase(t, quorumSize, nil, equal, less, 0, quorumSize, 0, false)
				}
			}
		}

		// Total ROUND-CHANGE messages equal to quorumSize
		for equal := 1; equal <= quorumSize; equal++ {
			for less := 0; less <= quorumSize-equal; less++ {
				nil := quorumSize - equal - less

				// Total PREPARE messages less than quorumSize
				for total := 0; total < quorumSize; total++ {
					testParameterizedCase(t, quorumSize, nil, equal, less, 0, total, quorumSize-total, false)
				}

				// Total PREPARE messages equal to quorumSize and some PREPARE message has round different than others
				for different := 1; different <= quorumSize; different++ {
					testParameterizedCase(t, quorumSize, nil, equal, less, 0, quorumSize-different, different, false)
				}
			}
		}
	}
}

func testParameterizedCase(
	t *testing.T,
	quorumSize int,
	rcForNil int,
	rcEqualToTargetRound int,
	rcLowerThanTargetRound int,
	rcHigherThanTargetRound int,
	preparesForTargetRound int,
	preparesNotForTargetRound int,
	messageJustified bool) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(quorumSize), pp)
	block := makeBlock(1)
	var round int64 = 10
	var targetPreparedRound int64 = 5

	rng := rand.New(rand.NewSource(time.Now().UTC().UnixNano()))

	if rcForNil+rcEqualToTargetRound+rcLowerThanTargetRound+rcHigherThanTargetRound > quorumSize {
		t.Errorf("rcForNil (%v) + rcEqualToTargetRound (%v) + rcLowerThanTargetRound (%v) + rcHigherThanTargetRound (%v) > quorumSize (%v)",
			rcForNil, rcEqualToTargetRound, rcLowerThanTargetRound, rcHigherThanTargetRound, quorumSize)
	}

	if preparesForTargetRound+preparesNotForTargetRound > quorumSize {
		t.Errorf("preparesForTargetRound (%v) + preparesNotForTargetRound (%v) > quorumSize (%v)", preparesForTargetRound, preparesNotForTargetRound, quorumSize)
	}

	// ROUND-CHANGE messages
	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0)
	for index, validator := range validatorSet.List() {
		var m *qbfttypes.SignedRoundChangePayload
		if index < rcForNil {
			m = createRoundChangeMessage(validator.Address(), round, 0, nil)
		} else if index >= rcForNil && index < rcForNil+rcEqualToTargetRound {
			m = createRoundChangeMessage(validator.Address(), round, targetPreparedRound, block)
		} else if index >= rcForNil+rcEqualToTargetRound && index < rcForNil+rcEqualToTargetRound+rcLowerThanTargetRound {
			m = createRoundChangeMessage(validator.Address(), round, int64(rng.Intn(int(targetPreparedRound)-1)+1), block)
		} else if index >= rcForNil+rcEqualToTargetRound+rcLowerThanTargetRound && index < rcForNil+rcEqualToTargetRound+rcLowerThanTargetRound+rcHigherThanTargetRound {
			m = createRoundChangeMessage(validator.Address(), round, int64(rng.Intn(int(targetPreparedRound))+int(targetPreparedRound)+1), block)
		} else {
			break
		}
		roundChangeMessages = append(roundChangeMessages, m)
	}

	// PREPARE messages
	prepareMessages := make([]*qbfttypes.Prepare, 0)
	for index, validator := range validatorSet.List() {
		var m *qbfttypes.Prepare
		if index < preparesForTargetRound {
			m = createPrepareMessage(validator.Address(), targetPreparedRound, block)
		} else if index >= preparesForTargetRound && index < preparesForTargetRound+preparesNotForTargetRound {
			notTargetPreparedRound := targetPreparedRound
			for notTargetPreparedRound == targetPreparedRound {
				notTargetPreparedRound = rng.Int63()
			}
			m = createPrepareMessage(validator.Address(), notTargetPreparedRound, block)
		} else {
			break
		}
		prepareMessages = append(prepareMessages, m)
	}

	for _, m := range roundChangeMessages {
		fmt.Printf("RC %v\n", m)
	}
	for _, m := range prepareMessages {
		fmt.Printf("PR %v\n", m)
	}
	fmt.Println("roundChangeMessages", roundChangeMessages, len(roundChangeMessages))
	if err := isJustified(block, roundChangeMessages, prepareMessages, quorumSize, validatorSet); err == nil && !messageJustified {
		t.Errorf("quorumSize = %v, rcForNil = %v, rcEqualToTargetRound = %v, rcLowerThanTargetRound = %v, rcHigherThanTargetRound = %v, preparesForTargetRound = %v, preparesNotForTargetRound = %v (Expected: %v, Actual: %v)",
			quorumSize, rcForNil, rcEqualToTargetRound, rcLowerThanTargetRound, rcHigherThanTargetRound, preparesForTargetRound, preparesNotForTargetRound, err == nil, !messageJustified)
	}
}

func createRoundChangeMessage(from common.Address, round int64, preparedRound int64, preparedBlock istanbul.Proposal) *qbfttypes.SignedRoundChangePayload {
	m := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(preparedRound), preparedBlock, false)
	m.SetSource(from)
	return &m.SignedRoundChangePayload
}

func createPrepareMessage(from common.Address, round int64, preparedBlock istanbul.Proposal) *qbfttypes.Prepare {
	return qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(round), preparedBlock.Hash(), nil, from)
}

func generateValidators(n int) []common.Address {
	vals := make([]common.Address, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		vals = append(vals, crypto.PubkeyToAddress(privateKey.PublicKey))
	}
	return vals
}

func makeBlock(number int64) *types.Block {
	header := &types.Header{
		Difficulty: big.NewInt(0),
		Number:     big.NewInt(number),
		GasLimit:   0,
		GasUsed:    0,
		Time:       0,
	}
	block := &types.Block{}
	return block.WithSeal(header)
}

// -----------------------------------------------------------------------------
// Regression tests for PRE-PREPARE justification PREPAREs
// -----------------------------------------------------------------------------

func TestIsJustified_AllowsNilPreparedRoundWithoutPrepares(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)

	// Quorum=3 in a 4-validator set (typical IBFT/QBFT)
	quorumSize := 3

	// Build quorum ROUND-CHANGE messages indicating "no prepared certificate"
	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i, v := range validatorSet.List() {
		if i == quorumSize {
			break
		}
		roundChangeMessages = append(roundChangeMessages, createRoundChangeMessage(v.Address(), 10, 0, nil))
	}

	// No prepares is valid in the nil-preparedRound justification case
	prepareMessages := []*qbfttypes.Prepare{}

	if err := isJustified(block, roundChangeMessages, prepareMessages, quorumSize, validatorSet); err != nil {
		t.Fatalf("expected justification to succeed with nil preparedRound and no prepares; got: %v", err)
	}
}

func TestIsJustified_RejectsPrepareWithEmptySource(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetPreparedRound := int64(5)

	// Need quorum ROUND-CHANGE messages overall
	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i, v := range validatorSet.List() {
		if i == quorumSize {
			break
		}
		// PreparedRound nil/0 is fine; we’re testing prepareMessages validation.
		roundChangeMessages = append(roundChangeMessages, createRoundChangeMessage(v.Address(), 10, 0, nil))
	}

	// Create prepares that match round/digest BUT do NOT have Source set
	// (this simulates "signature was never verified / SetSource never called")
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		p := qbfttypes.NewPrepare(big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash())
		// Intentionally do NOT call p.SetSource(...)
		prepareMessages = append(prepareMessages, p)
	}

	err := isJustified(block, roundChangeMessages, prepareMessages, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected justification to fail due to empty prepare source, but got nil")
	}
}

func TestIsJustified_RejectsPrepareSignerNotInValidatorSet(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetPreparedRound := int64(5)

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i, v := range validatorSet.List() {
		if i == quorumSize {
			break
		}
		roundChangeMessages = append(roundChangeMessages, createRoundChangeMessage(v.Address(), 10, 0, nil))
	}

	// Build quorum prepares but make one signer NOT in validator set
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i, v := range validatorSet.List() {
		if i == quorumSize-1 {
			break
		}
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash(), []byte{0x01}, v.Address(),
		))
	}

	// Add a prepare from a random address not in the set
	privateKey, _ := crypto.GenerateKey()
	notInSet := crypto.PubkeyToAddress(privateKey.PublicKey)

	prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
		big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash(), []byte{0x02}, notInSet,
	))

	err := isJustified(block, roundChangeMessages, prepareMessages, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected justification to fail due to non-validator prepare signer, but got nil")
	}
}

func TestIsJustified_RejectsDuplicatePrepareSignersNotDistinctQuorum(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)

	block := makeBlock(1)
	quorumSize := 3
	targetPreparedRound := int64(5)

	roundChangeMessages := make([]*qbfttypes.SignedRoundChangePayload, 0, quorumSize)
	for i, v := range validatorSet.List() {
		if i == quorumSize {
			break
		}
		roundChangeMessages = append(roundChangeMessages, createRoundChangeMessage(v.Address(), 10, 0, nil))
	}

	// Use only 2 unique validator addresses but supply 3 prepare messages (quorumSize=3)
	v0 := validatorSet.List()[0].Address()
	v1 := validatorSet.List()[1].Address()

	prepareMessages := []*qbfttypes.Prepare{
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash(), []byte{0x01}, v0),
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash(), []byte{0x02}, v0), // duplicate signer
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(targetPreparedRound), block.Hash(), []byte{0x03}, v1),
	}

	err := isJustified(block, roundChangeMessages, prepareMessages, quorumSize, validatorSet)
	if err == nil {
		t.Fatalf("expected justification to fail due to insufficient distinct prepare signers, but got nil")
	}
}

// -----------------------------------------------------------------------------
// Tests for hasMatchingRoundChangeAndPrepares
// -----------------------------------------------------------------------------

// Helper: creates a *RoundChange with a non-nil PreparedRound and PreparedDigest matching the block.
func createFullRoundChange(from common.Address, round int64, preparedRound int64, preparedBlock istanbul.Proposal) *qbfttypes.RoundChange {
	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(round), big.NewInt(preparedRound), preparedBlock, false)
	rc.SetSource(from)
	return rc
}

// Helper: creates a valid set of fixtures for hasMatchingRoundChangeAndPrepares tests.
// Returns validatorSet, roundChange, prepareMessages, quorumSize.
func setupMatchingFixtures() (istanbul.ValidatorSet, *qbfttypes.RoundChange, []*qbfttypes.Prepare, int) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)
	block := makeBlock(1)
	quorumSize := 3
	preparedRound := int64(5)

	rc := createFullRoundChange(validatorSet.List()[0].Address(), 10, preparedRound, block)

	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(preparedRound), block.Hash(), []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	return validatorSet, rc, prepareMessages, quorumSize
}

// Verifies that a well-formed justification with quorum-sized distinct validator
// PREPARE messages matching the ROUND-CHANGE's prepared round and digest is accepted.
func TestHasMatchingRoundChangeAndPrepares_HappyPath(t *testing.T) {
	validatorSet, rc, prepareMessages, quorumSize := setupMatchingFixtures()

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err != nil {
		t.Fatalf("expected success for valid inputs; got: %v", err)
	}
}

// Guards against a DoS vector where an attacker assembles a justification containing
// more PREPARE messages than there are validators in the set. Since each validator can
// only produce one valid PREPARE per round, any count exceeding the validator set size
// is inherently fraudulent and must be rejected before further processing.
func TestHasMatchingRoundChangeAndPrepares_RejectsTooManyPrepares(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	// 4 validators but we'll supply 5 prepares
	validatorSet := validator.NewSet(generateValidators(4), pp)
	block := makeBlock(1)
	preparedRound := int64(5)

	rc := createFullRoundChange(validatorSet.List()[0].Address(), 10, preparedRound, block)

	// Create 5 prepare messages (more than 4 validators)
	prepareMessages := make([]*qbfttypes.Prepare, 0, 5)
	for i := 0; i < 4; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(preparedRound), block.Hash(), []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}
	// Add a 5th prepare from a random address
	extraKey, _ := crypto.GenerateKey()
	extraAddr := crypto.PubkeyToAddress(extraKey.PublicKey)
	prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
		big.NewInt(1), big.NewInt(preparedRound), block.Hash(), []byte{0x05}, extraAddr,
	))

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, 3, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to too many prepare messages, but got nil")
	}
}

// Ensures a nil ROUND-CHANGE message is rejected early rather than causing a nil
// pointer dereference when the function attempts to read PreparedRound or PreparedDigest.
func TestHasMatchingRoundChangeAndPrepares_RejectsNilRoundChange(t *testing.T) {
	validatorSet, _, prepareMessages, quorumSize := setupMatchingFixtures()

	err := hasMatchingRoundChangeAndPrepares(nil, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to nil roundChange, but got nil")
	}
}

// Validates that a ROUND-CHANGE with a nil PreparedRound is rejected. The function
// compares each PREPARE's round against PreparedRound, so a nil value would panic
// on the big.Int comparison. This also catches malformed ROUND-CHANGE messages that
// claim to carry a prepared certificate but omit the round.
func TestHasMatchingRoundChangeAndPrepares_RejectsNilPreparedRound(t *testing.T) {
	validatorSet, _, prepareMessages, quorumSize := setupMatchingFixtures()

	// Create a RoundChange with nil PreparedRound
	rc := &qbfttypes.RoundChange{}
	rc.SetSource(validatorSet.List()[0].Address())
	// PreparedRound is nil by default

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to nil prepared round on roundChange, but got nil")
	}
}

// Confirms that a nil entry within the PREPARE messages slice is caught. A malicious
// node could craft an RLP-decoded justification containing nil entries to trigger a
// nil pointer dereference when accessing Digest, Round, or Source().
func TestHasMatchingRoundChangeAndPrepares_RejectsNilPrepareMessage(t *testing.T) {
	validatorSet, rc, prepareMessages, quorumSize := setupMatchingFixtures()

	// Replace one prepare with nil
	prepareMessages[1] = nil

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to nil prepare message, but got nil")
	}
}

// Verifies that PREPARE messages whose block digest differs from the ROUND-CHANGE's
// PreparedDigest are rejected. This prevents an attacker from reusing PREPARE messages
// from one block to justify a proposal for a different block.
func TestHasMatchingRoundChangeAndPrepares_RejectsDigestMismatch(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)
	block := makeBlock(1)
	wrongBlock := makeBlock(2) // different block => different digest
	quorumSize := 3
	preparedRound := int64(5)

	rc := createFullRoundChange(validatorSet.List()[0].Address(), 10, preparedRound, block)

	// Prepares use wrongBlock's digest
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(preparedRound), wrongBlock.Hash(), []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to digest mismatch, but got nil")
	}
}

// Confirms that the digest mismatch check is intentionally bypassed when a quorum of
// validators has flagged the proposal as bad (hasBadProposal=true). In this scenario
// the network needs to move past a faulty proposal, so the digest constraint is relaxed
// to allow consensus to proceed.
func TestHasMatchingRoundChangeAndPrepares_AllowsDigestMismatchWithBadProposal(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)
	block := makeBlock(1)
	wrongBlock := makeBlock(2)
	quorumSize := 3
	preparedRound := int64(5)

	rc := createFullRoundChange(validatorSet.List()[0].Address(), 10, preparedRound, block)

	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(preparedRound), wrongBlock.Hash(), []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	// With hasBadProposal=true, digest mismatch should be allowed
	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, true, validatorSet)
	if err != nil {
		t.Fatalf("expected success with hasBadProposal=true despite digest mismatch; got: %v", err)
	}
}

// Ensures that PREPARE messages for a different round than the ROUND-CHANGE's
// PreparedRound are rejected. Without this check, an attacker could attach PREPARE
// messages from round N to a ROUND-CHANGE claiming preparation at round M, fabricating
// a justification from unrelated consensus evidence.
func TestHasMatchingRoundChangeAndPrepares_RejectsRoundMismatch(t *testing.T) {
	pp := istanbul.NewRoundRobinProposerPolicy()
	pp.Use(istanbul.ValidatorSortByByte())
	validatorSet := validator.NewSet(generateValidators(4), pp)
	block := makeBlock(1)
	quorumSize := 3
	preparedRound := int64(5)
	wrongRound := int64(7)

	rc := createFullRoundChange(validatorSet.List()[0].Address(), 10, preparedRound, block)

	// Prepares use a different round than roundChange.PreparedRound
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), big.NewInt(wrongRound), block.Hash(), []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to round mismatch, but got nil")
	}
}

// Validates that a PREPARE message with a nil Round field is rejected before the
// big.Int comparison against PreparedRound, which would otherwise panic. This guards
// against malformed messages produced by buggy or malicious peers.
func TestHasMatchingRoundChangeAndPrepares_RejectsNilRoundOnPrepare(t *testing.T) {
	validatorSet, rc, prepareMessages, quorumSize := setupMatchingFixtures()

	// Set one prepare's Round to nil
	prepareMessages[1] = &qbfttypes.Prepare{
		Digest: rc.PreparedDigest,
	}
	prepareMessages[1].SetSource(validatorSet.List()[1].Address())
	// Round is nil by default on zero-value CommonPayload

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to nil round on prepare message, but got nil")
	}
}

// Checks that PREPARE messages with an empty (zero-address) Source are rejected. The
// Source field is populated by SetSource() after signature verification; an empty value
// indicates the signature was never checked. Accepting such messages would allow an
// attacker to forge unsigned prepares and claim arbitrary validator agreement.
func TestHasMatchingRoundChangeAndPrepares_RejectsEmptySource(t *testing.T) {
	validatorSet, rc, _, quorumSize := setupMatchingFixtures()

	// Create prepares without setting Source
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize; i++ {
		p := qbfttypes.NewPrepare(big.NewInt(1), rc.PreparedRound, rc.PreparedDigest)
		// Intentionally do NOT call p.SetSource(...)
		prepareMessages = append(prepareMessages, p)
	}

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to empty source on prepare, but got nil")
	}
}

// Verifies that a PREPARE message signed by an address not in the current validator set
// is rejected. Only active validators may participate in consensus; a signature from an
// outside address (even if cryptographically valid) must not count toward quorum.
func TestHasMatchingRoundChangeAndPrepares_RejectsNonValidatorSigner(t *testing.T) {
	validatorSet, rc, _, quorumSize := setupMatchingFixtures()

	// 2 valid signers + 1 non-validator signer
	prepareMessages := make([]*qbfttypes.Prepare, 0, quorumSize)
	for i := 0; i < quorumSize-1; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	notInSetKey, _ := crypto.GenerateKey()
	notInSet := crypto.PubkeyToAddress(notInSetKey.PublicKey)
	prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
		big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{0x99}, notInSet,
	))

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to non-validator signer, but got nil")
	}
}

// Confirms that duplicate signers within the PREPARE messages are detected and rejected.
// Without this check, a single compromised validator could submit multiple PREPARE
// messages with the same address to artificially inflate the signer count and meet quorum
// requirements on its own (or with fewer accomplices than quorum demands).
func TestHasMatchingRoundChangeAndPrepares_RejectsDuplicateSigner(t *testing.T) {
	validatorSet, rc, _, quorumSize := setupMatchingFixtures()

	v0 := validatorSet.List()[0].Address()
	v1 := validatorSet.List()[1].Address()

	// 3 prepares but v0 signs twice
	prepareMessages := []*qbfttypes.Prepare{
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{0x01}, v0),
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{0x02}, v0),
		qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{0x03}, v1),
	}

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to duplicate signer, but got nil")
	}
}

// Validates that the function rejects a justification with fewer PREPARE messages than
// the required quorum size. This is the fundamental BFT liveness/safety check: without
// agreement from at least ceil(2N/3) + 1 validators, the prepared certificate is invalid.
func TestHasMatchingRoundChangeAndPrepares_RejectsInsufficientPrepares(t *testing.T) {
	validatorSet, rc, _, quorumSize := setupMatchingFixtures()

	// Only 2 prepares when quorum requires 3
	prepareMessages := make([]*qbfttypes.Prepare, 0, 2)
	for i := 0; i < quorumSize-1; i++ {
		prepareMessages = append(prepareMessages, qbfttypes.NewPrepareWithSigAndSource(
			big.NewInt(1), rc.PreparedRound, rc.PreparedDigest, []byte{byte(i + 1)}, validatorSet.List()[i].Address(),
		))
	}

	err := hasMatchingRoundChangeAndPrepares(rc, prepareMessages, quorumSize, false, validatorSet)
	if err == nil {
		t.Fatalf("expected rejection due to insufficient prepare messages, but got nil")
	}
}
