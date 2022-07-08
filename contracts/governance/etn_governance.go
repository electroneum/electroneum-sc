// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package governance

import (
	"errors"
	"math/big"
	"strings"

	electroneum "github.com/electroneum/electroneum-sc"
	"github.com/electroneum/electroneum-sc/accounts/abi"
	"github.com/electroneum/electroneum-sc/accounts/abi/bind"
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = electroneum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
)

// ETNGovernanceInterfaceMetaData contains all meta data concerning the ETNGovernanceInterface contract.
var ETNGovernanceInterfaceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"name\":\"candidateRegistration\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"validatorAddress\",\"type\":\"address\"}],\"name\":\"candidateSupport\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getValidators\",\"outputs\":[{\"internalType\":\"address[]\",\"name\":\"\",\"type\":\"address[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"}],\"name\":\"newEpoch\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"withdrawRegistration\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"validatorAddress\",\"type\":\"address\"}],\"name\":\"withdrawSupport\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
}

// ETNGovernanceInterfaceABI is the input ABI used to generate the binding from.
// Deprecated: Use ETNGovernanceInterfaceMetaData.ABI instead.
var ETNGovernanceInterfaceABI = ETNGovernanceInterfaceMetaData.ABI

// ETNGovernanceInterface is an auto generated Go binding around an Ethereum contract.
type ETNGovernanceInterface struct {
	ETNGovernanceInterfaceCaller     // Read-only binding to the contract
	ETNGovernanceInterfaceTransactor // Write-only binding to the contract
	ETNGovernanceInterfaceFilterer   // Log filterer for contract events
}

// ETNGovernanceInterfaceCaller is an auto generated read-only Go binding around an Ethereum contract.
type ETNGovernanceInterfaceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNGovernanceInterfaceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ETNGovernanceInterfaceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNGovernanceInterfaceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ETNGovernanceInterfaceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNGovernanceInterfaceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ETNGovernanceInterfaceSession struct {
	Contract     *ETNGovernanceInterface // Generic contract binding to set the session for
	CallOpts     bind.CallOpts           // Call options to use throughout this session
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// ETNGovernanceInterfaceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ETNGovernanceInterfaceCallerSession struct {
	Contract *ETNGovernanceInterfaceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                 // Call options to use throughout this session
}

// ETNGovernanceInterfaceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ETNGovernanceInterfaceTransactorSession struct {
	Contract     *ETNGovernanceInterfaceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                 // Transaction auth options to use throughout this session
}

// ETNGovernanceInterfaceRaw is an auto generated low-level Go binding around an Ethereum contract.
type ETNGovernanceInterfaceRaw struct {
	Contract *ETNGovernanceInterface // Generic contract binding to access the raw methods on
}

// ETNGovernanceInterfaceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ETNGovernanceInterfaceCallerRaw struct {
	Contract *ETNGovernanceInterfaceCaller // Generic read-only contract binding to access the raw methods on
}

// ETNGovernanceInterfaceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ETNGovernanceInterfaceTransactorRaw struct {
	Contract *ETNGovernanceInterfaceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewETNGovernanceInterface creates a new instance of ETNGovernanceInterface, bound to a specific deployed contract.
func NewETNGovernanceInterface(address common.Address, backend bind.ContractBackend) (*ETNGovernanceInterface, error) {
	contract, err := bindETNGovernanceInterface(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ETNGovernanceInterface{ETNGovernanceInterfaceCaller: ETNGovernanceInterfaceCaller{contract: contract}, ETNGovernanceInterfaceTransactor: ETNGovernanceInterfaceTransactor{contract: contract}, ETNGovernanceInterfaceFilterer: ETNGovernanceInterfaceFilterer{contract: contract}}, nil
}

// NewETNGovernanceInterfaceCaller creates a new read-only instance of ETNGovernanceInterface, bound to a specific deployed contract.
func NewETNGovernanceInterfaceCaller(address common.Address, caller bind.ContractCaller) (*ETNGovernanceInterfaceCaller, error) {
	contract, err := bindETNGovernanceInterface(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ETNGovernanceInterfaceCaller{contract: contract}, nil
}

// NewETNGovernanceInterfaceTransactor creates a new write-only instance of ETNGovernanceInterface, bound to a specific deployed contract.
func NewETNGovernanceInterfaceTransactor(address common.Address, transactor bind.ContractTransactor) (*ETNGovernanceInterfaceTransactor, error) {
	contract, err := bindETNGovernanceInterface(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ETNGovernanceInterfaceTransactor{contract: contract}, nil
}

// NewETNGovernanceInterfaceFilterer creates a new log filterer instance of ETNGovernanceInterface, bound to a specific deployed contract.
func NewETNGovernanceInterfaceFilterer(address common.Address, filterer bind.ContractFilterer) (*ETNGovernanceInterfaceFilterer, error) {
	contract, err := bindETNGovernanceInterface(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ETNGovernanceInterfaceFilterer{contract: contract}, nil
}

// bindETNGovernanceInterface binds a generic wrapper to an already deployed contract.
func bindETNGovernanceInterface(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ETNGovernanceInterfaceABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ETNGovernanceInterface.Contract.ETNGovernanceInterfaceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.ETNGovernanceInterfaceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.ETNGovernanceInterfaceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ETNGovernanceInterface.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.contract.Transact(opts, method, params...)
}

// GetValidators is a free data retrieval call binding the contract method 0xb7ab4db5.
//
// Solidity: function getValidators() view returns(address[])
func (_ETNGovernanceInterface *ETNGovernanceInterfaceCaller) GetValidators(opts *bind.CallOpts) ([]common.Address, error) {
	var out []interface{}
	err := _ETNGovernanceInterface.contract.Call(opts, &out, "getValidators")

	if err != nil {
		return *new([]common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new([]common.Address)).(*[]common.Address)

	return out0, err

}

// GetValidators is a free data retrieval call binding the contract method 0xb7ab4db5.
//
// Solidity: function getValidators() view returns(address[])
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) GetValidators() ([]common.Address, error) {
	return _ETNGovernanceInterface.Contract.GetValidators(&_ETNGovernanceInterface.CallOpts)
}

// GetValidators is a free data retrieval call binding the contract method 0xb7ab4db5.
//
// Solidity: function getValidators() view returns(address[])
func (_ETNGovernanceInterface *ETNGovernanceInterfaceCallerSession) GetValidators() ([]common.Address, error) {
	return _ETNGovernanceInterface.Contract.GetValidators(&_ETNGovernanceInterface.CallOpts)
}

// CandidateRegistration is a paid mutator transaction binding the contract method 0x398ebe64.
//
// Solidity: function candidateRegistration() payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactor) CandidateRegistration(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNGovernanceInterface.contract.Transact(opts, "candidateRegistration")
}

// CandidateRegistration is a paid mutator transaction binding the contract method 0x398ebe64.
//
// Solidity: function candidateRegistration() payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) CandidateRegistration() (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.CandidateRegistration(&_ETNGovernanceInterface.TransactOpts)
}

// CandidateRegistration is a paid mutator transaction binding the contract method 0x398ebe64.
//
// Solidity: function candidateRegistration() payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorSession) CandidateRegistration() (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.CandidateRegistration(&_ETNGovernanceInterface.TransactOpts)
}

// CandidateSupport is a paid mutator transaction binding the contract method 0x098a3e56.
//
// Solidity: function candidateSupport(address validatorAddress) payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactor) CandidateSupport(opts *bind.TransactOpts, validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.contract.Transact(opts, "candidateSupport", validatorAddress)
}

// CandidateSupport is a paid mutator transaction binding the contract method 0x098a3e56.
//
// Solidity: function candidateSupport(address validatorAddress) payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) CandidateSupport(validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.CandidateSupport(&_ETNGovernanceInterface.TransactOpts, validatorAddress)
}

// CandidateSupport is a paid mutator transaction binding the contract method 0x098a3e56.
//
// Solidity: function candidateSupport(address validatorAddress) payable returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorSession) CandidateSupport(validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.CandidateSupport(&_ETNGovernanceInterface.TransactOpts, validatorAddress)
}

// NewEpoch is a paid mutator transaction binding the contract method 0xc4ed916b.
//
// Solidity: function newEpoch(uint256 blockNumber) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactor) NewEpoch(opts *bind.TransactOpts, blockNumber *big.Int) (*types.Transaction, error) {
	return _ETNGovernanceInterface.contract.Transact(opts, "newEpoch", blockNumber)
}

// NewEpoch is a paid mutator transaction binding the contract method 0xc4ed916b.
//
// Solidity: function newEpoch(uint256 blockNumber) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) NewEpoch(blockNumber *big.Int) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.NewEpoch(&_ETNGovernanceInterface.TransactOpts, blockNumber)
}

// NewEpoch is a paid mutator transaction binding the contract method 0xc4ed916b.
//
// Solidity: function newEpoch(uint256 blockNumber) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorSession) NewEpoch(blockNumber *big.Int) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.NewEpoch(&_ETNGovernanceInterface.TransactOpts, blockNumber)
}

// WithdrawRegistration is a paid mutator transaction binding the contract method 0x593b1b9c.
//
// Solidity: function withdrawRegistration() returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactor) WithdrawRegistration(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNGovernanceInterface.contract.Transact(opts, "withdrawRegistration")
}

// WithdrawRegistration is a paid mutator transaction binding the contract method 0x593b1b9c.
//
// Solidity: function withdrawRegistration() returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) WithdrawRegistration() (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.WithdrawRegistration(&_ETNGovernanceInterface.TransactOpts)
}

// WithdrawRegistration is a paid mutator transaction binding the contract method 0x593b1b9c.
//
// Solidity: function withdrawRegistration() returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorSession) WithdrawRegistration() (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.WithdrawRegistration(&_ETNGovernanceInterface.TransactOpts)
}

// WithdrawSupport is a paid mutator transaction binding the contract method 0x0d83ce65.
//
// Solidity: function withdrawSupport(address validatorAddress) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactor) WithdrawSupport(opts *bind.TransactOpts, validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.contract.Transact(opts, "withdrawSupport", validatorAddress)
}

// WithdrawSupport is a paid mutator transaction binding the contract method 0x0d83ce65.
//
// Solidity: function withdrawSupport(address validatorAddress) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceSession) WithdrawSupport(validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.WithdrawSupport(&_ETNGovernanceInterface.TransactOpts, validatorAddress)
}

// WithdrawSupport is a paid mutator transaction binding the contract method 0x0d83ce65.
//
// Solidity: function withdrawSupport(address validatorAddress) returns()
func (_ETNGovernanceInterface *ETNGovernanceInterfaceTransactorSession) WithdrawSupport(validatorAddress common.Address) (*types.Transaction, error) {
	return _ETNGovernanceInterface.Contract.WithdrawSupport(&_ETNGovernanceInterface.TransactOpts, validatorAddress)
}
