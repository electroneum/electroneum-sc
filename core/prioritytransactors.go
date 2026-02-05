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
// GetPriorityTransactors gets the priority transactor list for the current state using the priority contract address for the block number passed.
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

	transactorsMeta := abi.ConvertType(
		unpackResult[0],
		new([]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta),
	).(*[]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)

	for _, t := range *transactorsMeta {
		result[common.HexToPublicKey(t.PublicKey)] = common.PriorityTransactor{
			IsGasPriceWaiver: t.IsGasPriceWaiver,
			EntityName:       t.Name,
		}
	}

	return result
}
