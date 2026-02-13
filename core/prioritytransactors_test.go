package core

import (
	"testing"

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
