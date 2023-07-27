package core

import (
	"fmt"
	"strings"

	"github.com/electroneum/electroneum-sc/accounts/abi"
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/params"
)

// GetPriorityTransactors Gets the priority transactor list for the current state using the priority contract address for the block number passed
func MustGetPriorityTransactors(evm *vm.EVM) common.PriorityTransactorMap {
	var (
		blockNumber = evm.Context.BlockNumber
		config      = evm.ChainConfig()
		address     = config.GetPriorityTransactorsContractAddress(blockNumber)
		contract    = vm.AccountRef(address)
		method      = "getTransactors"
		result      = make(common.PriorityTransactorMap)
	)

	if address != (common.Address{}) {
		// Check if contract code exists at the address. If it doesn't. We haven't deployed the contract yet, so no error needed.
		byteCode := evm.StateDB.GetCode(address)
		if len(byteCode) == 0 {
			return result
		}

		contractABI, _ := abi.JSON(strings.NewReader(prioritytransactors.ETNPriorityTransactorsInterfaceMetaData.ABI))
		input, _ := contractABI.Pack(method)
		output, _, err := evm.StaticCall(contract, address, input, params.MaxGasLimit)
		// if there is an issue pulling the contract panic as something must be very
		// wrong, and we don't want an accidental fork or potentially try again and have
		// an incorrect flow
		if err != nil {
			panic(fmt.Errorf("error getting the priority transactors from the EVM/contract: %s", err))
		}

		unpackResult, err := contractABI.Unpack(method, output)
		// if there is an issue pulling the contract panic as something must be very
		// wrong, and we don't want an accidental fork or potentially try again and have
		// an incorrect flow
		if err != nil {
			panic(fmt.Errorf("error getting the priority transactors from the EVM/contract: %s", err))
		}

		transactorsMeta := abi.ConvertType(unpackResult[0], new([]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)).(*[]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)
		for _, t := range *transactorsMeta {
			result[common.HexToPublicKey(t.PublicKey)] = common.PriorityTransactor{
				IsGasPriceWaiver: t.IsGasPriceWaiver,
				EntityName:       t.Name,
			}
		}
	}
	return result
}
