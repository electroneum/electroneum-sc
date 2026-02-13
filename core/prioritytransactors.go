package core

import (
	"strings"

	"github.com/electroneum/electroneum-sc/accounts/abi"
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/params"
)

// GetPriorityTransactors Gets the priority transactor list for the current state using the priority contract address for the block number passed
func GetPriorityTransactors(evm *vm.EVM) common.PriorityTransactorMap {
	var (
		blockNumber = evm.Context.BlockNumber
		config      = evm.ChainConfig()
		address     = config.GetPriorityTransactorsContractAddress(blockNumber)
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

	contractABI, err := abi.JSON(strings.NewReader(prioritytransactors.ETNPriorityTransactorsInterfaceMetaData.ABI))
	if err != nil {
		// ABI parse failure is a software/config issue; safest is disable feature rather than crash the chain
		log.Error("PriorityTransactors: failed to parse ABI; disabling priority list for this block",
			"err", err, "address", address, "block", blockNumber)
		return result
	}

	input, err := contractABI.Pack(method)
	if err != nil {
		log.Error("PriorityTransactors: failed to pack call data; disabling priority list for this block",
			"err", err, "address", address, "block", blockNumber)
		return result
	}

	output, _, err := evm.StaticCall(contract, address, input, params.MaxGasLimit)
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
