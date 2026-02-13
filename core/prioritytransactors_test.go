package core

import (
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
)

// Test that safeConvertTransactorsMeta returns ok=true and the correct slice
// when given a valid []ETNPriorityTransactorsInterfaceTransactorMeta value.
func TestSafeConvertTransactorsMeta_ValidInput(t *testing.T) {
	input := []prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta{
		{
			IsGasPriceWaiver: true,
			PublicKey:        "0x04abcd",
			Name:             "TestEntity",
		},
		{
			IsGasPriceWaiver: false,
			PublicKey:        "0x04ef01",
			Name:             "AnotherEntity",
		},
	}

	metas, ok := safeConvertTransactorsMeta(input)
	if !ok {
		t.Fatal("expected ok=true for valid input")
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 metas, got %d", len(metas))
	}
	if metas[0].Name != "TestEntity" {
		t.Errorf("expected Name 'TestEntity', got %q", metas[0].Name)
	}
	if !metas[0].IsGasPriceWaiver {
		t.Error("expected IsGasPriceWaiver=true for first entry")
	}
	if metas[1].Name != "AnotherEntity" {
		t.Errorf("expected Name 'AnotherEntity', got %q", metas[1].Name)
	}
	if metas[1].IsGasPriceWaiver {
		t.Error("expected IsGasPriceWaiver=false for second entry")
	}
}

// Test that safeConvertTransactorsMeta returns ok=true and an empty slice
// when given a valid but empty slice.
func TestSafeConvertTransactorsMeta_EmptySlice(t *testing.T) {
	input := []prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta{}

	metas, ok := safeConvertTransactorsMeta(input)
	if !ok {
		t.Fatal("expected ok=true for empty slice input")
	}
	if len(metas) != 0 {
		t.Fatalf("expected 0 metas, got %d", len(metas))
	}
}

// Test that safeConvertTransactorsMeta returns ok=false and recovers without
// panicking when given nil input.
func TestSafeConvertTransactorsMeta_NilInput(t *testing.T) {
	metas, ok := safeConvertTransactorsMeta(nil)
	if ok {
		t.Fatal("expected ok=false for nil input")
	}
	if metas != nil {
		t.Fatalf("expected nil metas for nil input, got %v", metas)
	}
}

// Test that safeConvertTransactorsMeta returns ok=false and recovers without
// panicking when given an incompatible type (string instead of the expected slice).
func TestSafeConvertTransactorsMeta_WrongType(t *testing.T) {
	metas, ok := safeConvertTransactorsMeta("not a slice")
	if ok {
		t.Fatal("expected ok=false for wrong type input")
	}
	if metas != nil {
		t.Fatalf("expected nil metas for wrong type, got %v", metas)
	}
}

// Test that safeConvertTransactorsMeta returns ok=false and recovers without
// panicking when given an integer (another incompatible type).
func TestSafeConvertTransactorsMeta_IntType(t *testing.T) {
	metas, ok := safeConvertTransactorsMeta(42)
	if ok {
		t.Fatal("expected ok=false for int input")
	}
	if metas != nil {
		t.Fatalf("expected nil metas for int input, got %v", metas)
	}
}

// Test that safeConvertTransactorsMeta returns ok=false and recovers without
// panicking when given an empty struct (incompatible type).
func TestSafeConvertTransactorsMeta_EmptyStruct(t *testing.T) {
	metas, ok := safeConvertTransactorsMeta(struct{}{})
	if ok {
		t.Fatal("expected ok=false for empty struct input")
	}
	if metas != nil {
		t.Fatalf("expected nil metas for struct input, got %v", metas)
	}
}

// Test that PublicKey.IsValid rejects a 65-byte key that starts with 0x04
// but whose (x, y) coordinates do not lie on the secp256k1 curve.
func TestPublicKeyIsValid_InvalidCurvePoint(t *testing.T) {
	// 65 bytes starting with 0x04, but x=1, y=1 is not on secp256k1
	var pk common.PublicKey
	pk[0] = 0x04
	pk[1] = 0x01  // x = 1
	pk[33] = 0x01 // y = 1

	if pk.IsValid() {
		t.Error("expected IsValid=false for point not on secp256k1 curve")
	}
}

// Test that PublicKey.IsValid rejects a 65-byte key that does not start
// with the uncompressed prefix 0x04.
func TestPublicKeyIsValid_WrongPrefix(t *testing.T) {
	// Take a known-good key and corrupt the prefix byte
	validHex := "04efb99d9860f4dec4cb548a5722c27e9ef58e37fbab9719c5b33d55c216db49311221a01f638ce5f255875b194e0acaa58b19a89d2e56a864427298f826a7f887"
	pk := common.HexToPublicKey(validHex)
	pk[0] = 0x02 // compressed prefix — invalid for uncompressed key

	if pk.IsValid() {
		t.Error("expected IsValid=false for key with wrong prefix byte")
	}
}

// Test that PublicKey.IsValid accepts a known-good uncompressed secp256k1 key.
func TestPublicKeyIsValid_ValidKey(t *testing.T) {
	validHex := "04efb99d9860f4dec4cb548a5722c27e9ef58e37fbab9719c5b33d55c216db49311221a01f638ce5f255875b194e0acaa58b19a89d2e56a864427298f826a7f887"
	pk := common.HexToPublicKey(validHex)

	if !pk.IsValid() {
		t.Error("expected IsValid=true for known-good secp256k1 public key")
	}
}
