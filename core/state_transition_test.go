package core

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/state"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/crypto/secp256k1"
	"github.com/electroneum/electroneum-sc/params"
)

// testPriorityKey is a single priority private key used across these tests.
// Its corresponding uncompressed public key is registered in the mock
// priority transactor map that we inject into the statedb.
var testPriorityKey *ecdsa.PrivateKey

func init() {
	keyInt, _ := new(big.Int).SetString("3a1076bf45ab87712ad64ccb3b10217737f7faacbf2872e88fdd9a537d8fe266", 16)
	testPriorityKey = &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: secp256k1.S256(),
		},
		D: keyInt,
	}
	testPriorityKey.PublicKey.X, testPriorityKey.PublicKey.Y = testPriorityKey.PublicKey.Curve.ScalarBaseMult(keyInt.Bytes())
}

// testPriorityPubKey returns the uncompressed 65-byte public key for testPriorityKey
// as a common.PublicKey value suitable for use in the priority transactor map.
func testPriorityPubKey() common.PublicKey {
	pubBytes := crypto.FromECDSAPub(&testPriorityKey.PublicKey)
	var pk common.PublicKey
	copy(pk[:], pubBytes)
	return pk
}

// unknownPriorityPubKey returns a valid secp256k1 public key that is NOT
// registered in the priority transactor map, for testing unknown-key rejection.
func unknownPriorityPubKey() common.PublicKey {
	keyInt, _ := new(big.Int).SetString("caA076bf45ab87712ad64ccb3b10217737f7faacbf2872e88fdd9a537d8fe2ff", 16)
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: secp256k1.S256()},
		D:         keyInt,
	}
	priv.PublicKey.X, priv.PublicKey.Y = priv.PublicKey.Curve.ScalarBaseMult(keyInt.Bytes())
	pubBytes := crypto.FromECDSAPub(&priv.PublicKey)
	var pk common.PublicKey
	copy(pk[:], pubBytes)
	return pk
}

// newTestEVM creates a minimal EVM with the given priority transactor map
// injected into its statedb. The EVM has London active and a baseFee set.
func newTestEVM(transactors common.PriorityTransactorMap, baseFee *big.Int) *vm.EVM {
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	statedb.SetPriorityTransactors(transactors)

	chainConfig := &params.ChainConfig{
		ChainID:     big.NewInt(1),
		LondonBlock: big.NewInt(0),
	}

	blockCtx := vm.BlockContext{
		BlockNumber: big.NewInt(1),
		BaseFee:     baseFee,
	}

	return vm.NewEVM(blockCtx, vm.TxContext{}, statedb, chainConfig, vm.Config{})
}

// newTestMessage creates a types.Message with the given fee fields and priority sender.
// This uses types.NewMessage which sets the fields directly without signature verification,
// suitable for unit testing the validation logic in isolation.
func newTestMessage(feeCap, tipCap *big.Int, prioritySender common.PublicKey) types.Message {
	to := common.Address{}
	return types.NewMessage(
		common.Address{}, // from
		&to,              // to
		0,                // nonce
		big.NewInt(0),    // amount
		21000,            // gasLimit
		big.NewInt(0),    // gasPrice (not used by validatePriorityGasFields)
		feeCap,           // gasFeeCap
		tipCap,           // gasTipCap
		nil,              // data
		nil,              // accessList
		true,             // isFake
		prioritySender,   // prioritySender
	)
}

// ============================================================================
// Tests for validatePriorityGasFields
//
// These tests exercise the state-transition-level validation of priority
// transaction gas fields. This function is the last line of defence before
// a transaction actually executes, and operates on Message-level fields
// (GasFeeCap/GasTipCap) rather than the raw Transaction fields checked by
// the tx pool.
// ============================================================================

// TestValidatePriorityGasFields_NonPriorityTx verifies that a regular
// (non-priority) transaction passes through validatePriorityGasFields with
// hasWaiver=false and no error, regardless of its fee fields. The function
// should be a no-op for non-priority transactions.
func TestValidatePriorityGasFields_NonPriorityTx(t *testing.T) {
	t.Parallel()

	evm := newTestEVM(common.PriorityTransactorMap{}, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(1_000_000_000), big.NewInt(1), common.PublicKey{})

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err != nil {
		t.Fatalf("expected nil error for non-priority tx, got %v", err)
	}
	if hasWaiver {
		t.Fatal("expected hasWaiver=false for non-priority tx")
	}
}

// TestValidatePriorityGasFields_WaiverZeroFees verifies that a waiver priority
// sender with zero feeCap and zero tipCap is accepted and returns hasWaiver=true.
// This is the normal happy path for gas-waiver priority transactions.
func TestValidatePriorityGasFields_WaiverZeroFees(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Waiver Entity", IsGasPriceWaiver: true},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(0), big.NewInt(0), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err != nil {
		t.Fatalf("expected nil error for waiver tx with zero fees, got %v", err)
	}
	if !hasWaiver {
		t.Fatal("expected hasWaiver=true for waiver sender")
	}
}

// TestValidatePriorityGasFields_WaiverNonZeroFeeCap verifies that a waiver
// priority sender is rejected if they submit a transaction with a non-zero
// feeCap. Waiver senders must have ALL fee fields set to zero — any non-zero
// value suggests a malformed or tampered transaction.
func TestValidatePriorityGasFields_WaiverNonZeroFeeCap(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Waiver Entity", IsGasPriceWaiver: true},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(1), big.NewInt(0), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for waiver tx with non-zero feeCap")
	}
	if !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver, got %v", err)
	}
	if !hasWaiver {
		t.Fatal("expected hasWaiver=true even on error (sender IS a waiver sender)")
	}
}

// TestValidatePriorityGasFields_WaiverNonZeroTipCap verifies that a waiver
// priority sender is rejected if they submit a transaction with a non-zero
// tipCap, even if feeCap is zero. Both fields must be zero for waiver senders.
func TestValidatePriorityGasFields_WaiverNonZeroTipCap(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Waiver Entity", IsGasPriceWaiver: true},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(0), big.NewInt(1), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for waiver tx with non-zero tipCap")
	}
	if !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver, got %v", err)
	}
	if !hasWaiver {
		t.Fatal("expected hasWaiver=true even on error")
	}
}

// TestValidatePriorityGasFields_NonWaiverPositiveFeeCap verifies that a
// non-waiver priority sender with a positive feeCap passes validation and
// returns hasWaiver=false. This is the happy path for non-waiver priority
// transactions — they pay fees like regular users.
func TestValidatePriorityGasFields_NonWaiverPositiveFeeCap(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Paying Entity", IsGasPriceWaiver: false},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(2_000_000_000), big.NewInt(1), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err != nil {
		t.Fatalf("expected nil error for non-waiver tx with positive feeCap, got %v", err)
	}
	if hasWaiver {
		t.Fatal("expected hasWaiver=false for non-waiver sender")
	}
}

// TestValidatePriorityGasFields_NonWaiverZeroFeeCap verifies that a non-waiver
// priority sender is rejected if they submit a transaction with feeCap=0. Even
// though they have priority status, non-waiver senders must pay — a zero feeCap
// means they're trying to transact for free without authorisation.
func TestValidatePriorityGasFields_NonWaiverZeroFeeCap(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Paying Entity", IsGasPriceWaiver: false},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(0), big.NewInt(0), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for non-waiver tx with zero feeCap")
	}
	if !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver, got %v", err)
	}
	if hasWaiver {
		t.Fatal("expected hasWaiver=false for non-waiver sender")
	}
}

// TestValidatePriorityGasFields_UnknownPubkey verifies that a priority
// transaction signed with a public key that is NOT in the priority transactor
// map is rejected with errBadPriorityKey. This catches the case where a
// transaction carries a priority signature from an unregistered key.
func TestValidatePriorityGasFields_UnknownPubkey(t *testing.T) {
	t.Parallel()

	// Register only the known key; use a different key in the message
	knownPK := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		knownPK: {EntityName: "Known Entity", IsGasPriceWaiver: false},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))

	unknownPK := unknownPriorityPubKey()
	msg := newTestMessage(big.NewInt(1_000_000_000), big.NewInt(1), unknownPK)

	_, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for unknown priority pubkey")
	}
	if !errors.Is(err, errBadPriorityKey) {
		t.Fatalf("expected errBadPriorityKey, got %v", err)
	}
}

// TestValidatePriorityGasFields_EmptyTransactorMap verifies that a priority
// transaction is rejected when the priority transactor map is completely empty
// (e.g. if the contract returned no registered transactors). The function
// should return errBadPriorityKey since no key can be found.
func TestValidatePriorityGasFields_EmptyTransactorMap(t *testing.T) {
	t.Parallel()

	evm := newTestEVM(common.PriorityTransactorMap{}, big.NewInt(1_000_000_000))
	pk := testPriorityPubKey()
	msg := newTestMessage(big.NewInt(1_000_000_000), big.NewInt(1), pk)

	_, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for priority tx with empty transactor map")
	}
	if !errors.Is(err, errBadPriorityKey) {
		t.Fatalf("expected errBadPriorityKey, got %v", err)
	}
}

// TestValidatePriorityGasFields_NonWaiverZeroFeeCapPositiveTipCap verifies
// that a non-waiver priority sender with feeCap=0 but tipCap>0 is still
// rejected. The feeCap is the binding constraint — if it's zero, the sender
// cannot cover the base fee regardless of their tip.
func TestValidatePriorityGasFields_NonWaiverZeroFeeCapPositiveTipCap(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Paying Entity", IsGasPriceWaiver: false},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	// feeCap=0, tipCap=1 — note: in practice feeCap >= tipCap is enforced
	// earlier in preCheck, but validatePriorityGasFields only checks feeCap > 0.
	msg := newTestMessage(big.NewInt(0), big.NewInt(1), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for non-waiver tx with zero feeCap")
	}
	if !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver, got %v", err)
	}
	if hasWaiver {
		t.Fatal("expected hasWaiver=false for non-waiver sender")
	}
}

// TestValidatePriorityGasFields_WaiverBothNonZero verifies that a waiver
// priority sender is rejected when BOTH feeCap and tipCap are non-zero.
// This covers the case where a waiver sender submits what looks like a
// regular fee-paying transaction.
func TestValidatePriorityGasFields_WaiverBothNonZero(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Waiver Entity", IsGasPriceWaiver: true},
	}
	evm := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(2_000_000_000), big.NewInt(1_000_000_000), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error for waiver tx with both fee fields non-zero")
	}
	if !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver, got %v", err)
	}
	if !hasWaiver {
		t.Fatal("expected hasWaiver=true even on error (sender IS a waiver sender)")
	}
}

// TestValidatePriorityGasFields_NonWaiverFeeCapBelowBaseFeeStillPasses
// verifies that validatePriorityGasFields does NOT check whether feeCap meets
// the base fee — that comparison happens in the caller (preCheck). The function
// only validates that a non-waiver sender has feeCap > 0, even if feeCap is
// below the current base fee. Separation of concerns: validatePriorityGasFields
// handles priority-specific rules, preCheck handles the universal baseFee gate.
func TestValidatePriorityGasFields_NonWaiverFeeCapBelowBaseFeeStillPasses(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Paying Entity", IsGasPriceWaiver: false},
	}
	baseFee := big.NewInt(10_000_000_000) // 10 gwei
	evm := newTestEVM(transactors, baseFee)
	// feeCap = 1 wei, far below baseFee — but validatePriorityGasFields
	// should still pass because it only checks feeCap > 0.
	msg := newTestMessage(big.NewInt(1), big.NewInt(0), pk)

	hasWaiver, err := validatePriorityGasFields(evm, msg)
	if err != nil {
		t.Fatalf("expected nil error (baseFee comparison is not this function's job), got %v", err)
	}
	if hasWaiver {
		t.Fatal("expected hasWaiver=false for non-waiver sender")
	}
}

// TestValidatePriorityGasFields_ErrorMessageContainsPubkey verifies that error
// messages from validatePriorityGasFields include the priority public key for
// debugging/logging. Without the pubkey in the error, operators would have no
// way to identify which priority sender caused a rejection.
func TestValidatePriorityGasFields_ErrorMessageContainsPubkey(t *testing.T) {
	t.Parallel()

	pk := testPriorityPubKey()

	// Test 1: unknown key error includes the key
	evm := newTestEVM(common.PriorityTransactorMap{}, big.NewInt(1_000_000_000))
	msg := newTestMessage(big.NewInt(1), big.NewInt(0), pk)

	_, err := validatePriorityGasFields(evm, msg)
	if err == nil {
		t.Fatal("expected error")
	}
	pkHex := hex.EncodeToString(pk[:])
	if got := err.Error(); !contains(got, pkHex) {
		t.Errorf("error message should contain pubkey hex\n  got:  %s\n  want substring: %s", got, pkHex)
	}

	// Test 2: waiver violation error includes the key
	transactors := common.PriorityTransactorMap{
		pk: {EntityName: "Waiver Entity", IsGasPriceWaiver: true},
	}
	evm2 := newTestEVM(transactors, big.NewInt(1_000_000_000))
	msg2 := newTestMessage(big.NewInt(1), big.NewInt(0), pk)

	_, err2 := validatePriorityGasFields(evm2, msg2)
	if err2 == nil {
		t.Fatal("expected error")
	}
	if got := err2.Error(); !contains(got, pkHex) {
		t.Errorf("error message should contain pubkey hex\n  got:  %s\n  want substring: %s", got, pkHex)
	}

	// Test 3: non-waiver zero fee error includes the key
	transactors3 := common.PriorityTransactorMap{
		pk: {EntityName: "Paying Entity", IsGasPriceWaiver: false},
	}
	evm3 := newTestEVM(transactors3, big.NewInt(1_000_000_000))
	msg3 := newTestMessage(big.NewInt(0), big.NewInt(0), pk)

	_, err3 := validatePriorityGasFields(evm3, msg3)
	if err3 == nil {
		t.Fatal("expected error")
	}
	if got := err3.Error(); !contains(got, pkHex) {
		t.Errorf("error message should contain pubkey hex\n  got:  %s\n  want substring: %s", got, pkHex)
	}
}

// contains is a helper that checks whether s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
