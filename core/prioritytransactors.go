package core

import (
	"github.com/electroneum/electroneum-sc/accounts/abi"
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/contracts/prioritytransactors"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/params"
	"math/big"
	"strings"
)

func GetPriorityTransactors(blockNumber *big.Int, config *params.ChainConfig, evm *vm.EVM) common.PriorityTransactorMap {
	var (
		address  = config.GetPriorityTransactorsContractAddress(blockNumber)
		contract = vm.AccountRef(address)
		method   = "getTransactors"
		result   = make(common.PriorityTransactorMap)
	)

	if address != (common.Address{}) {
		contractABI, _ := abi.JSON(strings.NewReader(prioritytransactors.ETNPriorityTransactorsInterfaceABI))
		input, _ := contractABI.Pack(method)
		output, _, err := evm.StaticCall(contract, address, input, params.MaxGasLimit)
		if err != nil {
			return result
		}
		unpackResult, err := contractABI.Unpack(method, output)
		if err != nil {
			return result
		}
		transactorsMeta := abi.ConvertType(unpackResult[0], new([]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)).(*[]prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)
		for _, t := range *transactorsMeta {
			// Only add transactors that are within start/end height range
			if t.StartHeight <= blockNumber.Uint64() && (t.EndHeight >= blockNumber.Uint64() || t.EndHeight == 0) {
				result[common.HexToPublicKey(t.PublicKey)] = common.PriorityTransactor{
					IsGasPriceWaiver: t.IsGasPriceWaiver,
					EntityName:       t.Name,
				}
			}
		}
	}
	return result
}

func getPriorityTransactorByKey(blockNumber *big.Int, publicKey common.PublicKey, config *params.ChainConfig, evm *vm.EVM) (common.PriorityTransactor, bool) {
	var (
		address  = config.GetPriorityTransactorsContractAddress(blockNumber)
		contract = vm.AccountRef(address)
		method   = "getTransactorByKey"
	)

	if address != (common.Address{}) {
		contractABI, _ := abi.JSON(strings.NewReader(prioritytransactors.ETNPriorityTransactorsInterfaceABI))
		input, _ := contractABI.Pack(method, publicKey.ToUnprefixedHexString())
		output, _, err := evm.StaticCall(contract, address, input, params.MaxGasLimit)
		if err != nil {
			return common.PriorityTransactor{}, false
		}
		unpackResult, err := contractABI.Unpack(method, output)
		if err != nil {
			return common.PriorityTransactor{}, false
		}
		transactorMeta := abi.ConvertType(unpackResult[0], new(prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)).(*prioritytransactors.ETNPriorityTransactorsInterfaceTransactorMeta)
		// Only add transactors that are within start/end height range
		if transactorMeta.StartHeight <= blockNumber.Uint64() && (transactorMeta.EndHeight >= blockNumber.Uint64() || transactorMeta.EndHeight == 0) {
			return common.PriorityTransactor{
				EntityName:       transactorMeta.Name,
				IsGasPriceWaiver: transactorMeta.IsGasPriceWaiver,
			}, true
		}
	}
	return common.PriorityTransactor{}, false
}