package core

import (
	"strings"
	"sync"

	"github.com/electroneum/electroneum-sc/accounts/abi"
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/params"
)

// getTransactorsCall is the parsed contract ABI and the packed getTransactors
// call. Both are constant, and parsing the ABI JSON took longer than running
// the call, so it's done once rather than on every refresh.
var getTransactorsCall = sync.OnceValues(func() (abi.ABI, []byte) {
	contractABI, err := abi.JSON(strings.NewReader(prioritytransactors.ETNPriorityTransactorsInterfaceMetaData.ABI))
	if err != nil {
		log.Error("PriorityTransactors: failed to parse ABI", "err", err)
		return abi.ABI{}, nil
	}
	input, err := contractABI.Pack("getTransactors")
	if err != nil {
		log.Error("PriorityTransactors: failed to pack call data", "err", err)
		return abi.ABI{}, nil
	}
	return contractABI, input
})

// GetPriorityTransactors Gets the priority transactor list for the current state using the priority contract address for the block number passed
func GetPriorityTransactors(evm *vm.EVM) common.PriorityTransactorMap {
	return GetPriorityTransactorsAt(evm, evm.ChainConfig().GetPriorityTransactorsContractAddress(evm.Context.BlockNumber))
}

// GetPriorityTransactorsAt is GetPriorityTransactors with an explicitly supplied
// contract address, letting a caller read the list from a state snapshot while
// resolving the address for a different (e.g. the next) block number.
//
// The txpool needs this because it holds the state of head block N but admits
// transactions destined for block N+1: it must resolve the transition-aware
// contract address for N+1 while still executing against N's state. Resolving
// the address is pure config arithmetic, so this stays free of any synthetic
// block context that would perturb NUMBER/TIMESTAMP/BLOCKHASH inside the EVM.
func GetPriorityTransactorsAt(evm *vm.EVM, address common.Address) common.PriorityTransactorMap {
	var (
		blockNumber = evm.Context.BlockNumber
		contract    = vm.AccountRef(address)
		method      = "getTransactors"
		result      = make(common.PriorityTransactorMap)
	)

	// No contract configured => no priority transactors
	if address == (common.Address{}) {
		return result
	}

	// Contract not deployed yet => no priority transactors (not an error)
	byteCode := evm.StateDB.GetCode(address)
	if len(byteCode) == 0 {
		return result
	}

	contractABI, input := getTransactorsCall()
	if input == nil {
		// ABI parse failure is a software/config issue; safest is disable feature rather than crash the chain
		log.Error("PriorityTransactors: no getTransactors call data; disabling priority list for this block",
			"address", address, "block", blockNumber)
		return result
	}

	output, _, err := evm.StaticCall(contract, address, common.CopyBytes(input), params.MaxGasLimit)
	if err != nil {
		// IMPORTANT: on failure, return empty list (deny privileges) instead of panicking (halting chain)
		log.Error("PriorityTransactors: contract call failed; disabling priority list for this block",
			"err", err, "address", address, "block", blockNumber)
		return result
	}

	unpackResult, err := contractABI.Unpack(method, output)
	if err != nil {
		log.Error("PriorityTransactors: ABI unpack failed; disabling priority list for this block",
			"err", err, "address", address, "block", blockNumber)
		return result
	}

	// Defensive: avoid panic if return shape unexpected
	if len(unpackResult) == 0 {
		log.Error("PriorityTransactors: empty return data; disabling priority list for this block",
			"address", address, "block", blockNumber)
		return result
	}

	metas, ok := safeConvertTransactorsMeta(unpackResult[0])
	if !ok {
		log.Error("PriorityTransactors: unexpected return type; disabling priority list for this block",
			"address", address, "block", blockNumber)
		return result
	}

	for _, t := range metas {
		// Validate public key bytes are exactly 65 bytes (your PublicKey type length).
		pkBytes := common.FromHex(t.PublicKey)
		if len(pkBytes) != common.PublicKeyLength {
			log.Warn("PriorityTransactors: invalid public key length in contract data; skipping entry",
				"len", len(pkBytes),
				"name", t.Name,
				"address", address,
				"block", blockNumber)
			continue
		}

		// Optional: reject all-zero pubkeys (prevents silly/garbage entries)
		allZero := true
		for _, b := range pkBytes {
			if b != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			log.Warn("PriorityTransactors: all-zero public key in contract data; skipping entry",
				"name", t.Name,
				"address", address,
				"block", blockNumber)
			continue
		}

		pk := common.BytesToPublicKey(pkBytes)
		if !pk.IsValid() {
			log.Warn("PriorityTransactors: public key not on secp256k1 curve; skipping entry",
				"name", t.Name,
				"address", address,
				"block", blockNumber)
			continue
		}
		result[pk] = common.PriorityTransactor{
			IsGasPriceWaiver: t.IsGasPriceWaiver,
			EntityName:       t.Name,
		}
	}

	return result
}

func safeConvertTransactorsMeta(unpack0 any) (metas []prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()

	ptr := abi.ConvertType(
		unpack0,
		new([]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta),
	).(*[]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)

	if ptr == nil {
		return nil, false
	}
	return *ptr, true
}
