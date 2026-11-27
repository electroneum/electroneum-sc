package core

import (
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/state"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/params"
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

// returnCode is contract code that returns data for any call.
func returnCode(data []byte) []byte {
	size := []byte{byte(len(data) >> 8), byte(len(data))}
	code := []byte{0x61, size[0], size[1], 0x61, 0x00, 0x0f, 0x60, 0x00, 0x39} // CODECOPY(0, 15, size)
	code = append(code, 0x61, size[0], size[1], 0x60, 0x00, 0xf3)              // RETURN(0, size)
	return append(code, data...)
}

// The ABI and call data are prepared once and shared by every refresh, which
// runs concurrently in block processing, the miner and the txpool. Each
// refresh must still read the list from the contract.
func TestGetPriorityTransactorsAt_Repeated(t *testing.T) {
	key, _ := crypto.GenerateKey()
	pub := crypto.FromECDSAPub(&key.PublicKey)
	contractABI, _ := getTransactorsCall()
	output, err := contractABI.Methods["getTransactors"].Outputs.Pack([]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta{
		{IsGasPriceWaiver: true, PublicKey: common.Bytes2Hex(pub), Name: "Oracle"},
	})
	if err != nil {
		t.Fatal(err)
	}
	addr := common.HexToAddress("0x9999999999999999999999999999999999999999")
	newEVM := func() *vm.EVM {
		statedb, _ := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
		statedb.SetCode(addr, returnCode(output))
		ctx := vm.BlockContext{CanTransfer: CanTransfer, Transfer: Transfer, BlockNumber: big.NewInt(1), Time: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: 30_000_000}
		return vm.NewEVM(ctx, vm.TxContext{}, statedb, params.TestChainConfig, vm.Config{})
	}
	check := func(list common.PriorityTransactorMap) error {
		got, ok := list[common.BytesToPublicKey(pub)]
		if len(list) != 1 || !ok || !got.IsGasPriceWaiver || got.EntityName != "Oracle" {
			return fmt.Errorf("have %v, want the one transactor", list)
		}
		return nil
	}
	evm := newEVM()
	for i := 0; i < 3; i++ {
		if err := check(GetPriorityTransactorsAt(evm, addr)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- check(GetPriorityTransactorsAt(newEVM(), addr))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
