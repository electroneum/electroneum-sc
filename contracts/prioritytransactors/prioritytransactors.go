// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package prioritytransactors

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

// ETNPriorityTransactorsInterfaceTransactorMeta is an auto generated low-level Go binding around an user-defined struct.
type ETNPriorityTransactorsInterfaceTransactorMeta struct {
	IsGasPriceWaiver bool
	PublicKey        string
	Name             string
}

// ETNPriorityTransactorsInterfaceMetaData contains all meta data concerning the ETNPriorityTransactorsInterface contract.
var ETNPriorityTransactorsInterfaceMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_publicKey\",\"type\":\"string\"}],\"name\":\"getTransactorByKey\",\"outputs\":[{\"components\":[{\"internalType\":\"bool\",\"name\":\"isGasPriceWaiver\",\"type\":\"bool\"},{\"internalType\":\"string\",\"name\":\"publicKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structETNPriorityTransactorsInterface.TransactorMeta\",\"name\":\"\",\"type\":\"tuple\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getTransactors\",\"outputs\":[{\"components\":[{\"internalType\":\"bool\",\"name\":\"isGasPriceWaiver\",\"type\":\"bool\"},{\"internalType\":\"string\",\"name\":\"publicKey\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"}],\"internalType\":\"structETNPriorityTransactorsInterface.TransactorMeta[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"}]",
}

// ETNPriorityTransactorsInterfaceABI is the input ABI used to generate the binding from.
// Deprecated: Use ETNPriorityTransactorsInterfaceMetaData.ABI instead.
var ETNPriorityTransactorsInterfaceABI = ETNPriorityTransactorsInterfaceMetaData.ABI

// ETNPriorityTransactorsInterface is an auto generated Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterface struct {
	ETNPriorityTransactorsInterfaceCaller     // Read-only binding to the contract
	ETNPriorityTransactorsInterfaceTransactor // Write-only binding to the contract
	ETNPriorityTransactorsInterfaceFilterer   // Log filterer for contract events
}

// ETNPriorityTransactorsInterfaceCaller is an auto generated read-only Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterfaceCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNPriorityTransactorsInterfaceTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterfaceTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNPriorityTransactorsInterfaceFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ETNPriorityTransactorsInterfaceFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ETNPriorityTransactorsInterfaceSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ETNPriorityTransactorsInterfaceSession struct {
	Contract     *ETNPriorityTransactorsInterface // Generic contract binding to set the session for
	CallOpts     bind.CallOpts                    // Call options to use throughout this session
	TransactOpts bind.TransactOpts                // Transaction auth options to use throughout this session
}

// ETNPriorityTransactorsInterfaceCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ETNPriorityTransactorsInterfaceCallerSession struct {
	Contract *ETNPriorityTransactorsInterfaceCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts                          // Call options to use throughout this session
}

// ETNPriorityTransactorsInterfaceTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ETNPriorityTransactorsInterfaceTransactorSession struct {
	Contract     *ETNPriorityTransactorsInterfaceTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts                          // Transaction auth options to use throughout this session
}

// ETNPriorityTransactorsInterfaceRaw is an auto generated low-level Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterfaceRaw struct {
	Contract *ETNPriorityTransactorsInterface // Generic contract binding to access the raw methods on
}

// ETNPriorityTransactorsInterfaceCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterfaceCallerRaw struct {
	Contract *ETNPriorityTransactorsInterfaceCaller // Generic read-only contract binding to access the raw methods on
}

// ETNPriorityTransactorsInterfaceTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ETNPriorityTransactorsInterfaceTransactorRaw struct {
	Contract *ETNPriorityTransactorsInterfaceTransactor // Generic write-only contract binding to access the raw methods on
}

// NewETNPriorityTransactorsInterface creates a new instance of ETNPriorityTransactorsInterface, bound to a specific deployed contract.
func NewETNPriorityTransactorsInterface(address common.Address, backend bind.ContractBackend) (*ETNPriorityTransactorsInterface, error) {
	contract, err := bindETNPriorityTransactorsInterface(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ETNPriorityTransactorsInterface{ETNPriorityTransactorsInterfaceCaller: ETNPriorityTransactorsInterfaceCaller{contract: contract}, ETNPriorityTransactorsInterfaceTransactor: ETNPriorityTransactorsInterfaceTransactor{contract: contract}, ETNPriorityTransactorsInterfaceFilterer: ETNPriorityTransactorsInterfaceFilterer{contract: contract}}, nil
}

// NewETNPriorityTransactorsInterfaceCaller creates a new read-only instance of ETNPriorityTransactorsInterface, bound to a specific deployed contract.
func NewETNPriorityTransactorsInterfaceCaller(address common.Address, caller bind.ContractCaller) (*ETNPriorityTransactorsInterfaceCaller, error) {
	contract, err := bindETNPriorityTransactorsInterface(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ETNPriorityTransactorsInterfaceCaller{contract: contract}, nil
}

// NewETNPriorityTransactorsInterfaceTransactor creates a new write-only instance of ETNPriorityTransactorsInterface, bound to a specific deployed contract.
func NewETNPriorityTransactorsInterfaceTransactor(address common.Address, transactor bind.ContractTransactor) (*ETNPriorityTransactorsInterfaceTransactor, error) {
	contract, err := bindETNPriorityTransactorsInterface(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ETNPriorityTransactorsInterfaceTransactor{contract: contract}, nil
}

// NewETNPriorityTransactorsInterfaceFilterer creates a new log filterer instance of ETNPriorityTransactorsInterface, bound to a specific deployed contract.
func NewETNPriorityTransactorsInterfaceFilterer(address common.Address, filterer bind.ContractFilterer) (*ETNPriorityTransactorsInterfaceFilterer, error) {
	contract, err := bindETNPriorityTransactorsInterface(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ETNPriorityTransactorsInterfaceFilterer{contract: contract}, nil
}

// bindETNPriorityTransactorsInterface binds a generic wrapper to an already deployed contract.
func bindETNPriorityTransactorsInterface(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(ETNPriorityTransactorsInterfaceABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ETNPriorityTransactorsInterface.Contract.ETNPriorityTransactorsInterfaceCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNPriorityTransactorsInterface.Contract.ETNPriorityTransactorsInterfaceTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ETNPriorityTransactorsInterface.Contract.ETNPriorityTransactorsInterfaceTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ETNPriorityTransactorsInterface.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ETNPriorityTransactorsInterface.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ETNPriorityTransactorsInterface.Contract.contract.Transact(opts, method, params...)
}

// GetTransactorByKey is a free data retrieval call binding the contract method 0x41e2829b.
//
// Solidity: function getTransactorByKey(string _publicKey) view returns((uint64,uint64,bool,string,string))
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceCaller) GetTransactorByKey(opts *bind.CallOpts, _publicKey string) (ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	var out []interface{}
	err := _ETNPriorityTransactorsInterface.contract.Call(opts, &out, "getTransactorByKey", _publicKey)

	if err != nil {
		return *new(ETNPriorityTransactorsInterfaceTransactorMeta), err
	}

	out0 := *abi.ConvertType(out[0], new(ETNPriorityTransactorsInterfaceTransactorMeta)).(*ETNPriorityTransactorsInterfaceTransactorMeta)

	return out0, err

}

// GetTransactorByKey is a free data retrieval call binding the contract method 0x41e2829b.
//
// Solidity: function getTransactorByKey(string _publicKey) view returns((uint64,uint64,bool,string,string))
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceSession) GetTransactorByKey(_publicKey string) (ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	return _ETNPriorityTransactorsInterface.Contract.GetTransactorByKey(&_ETNPriorityTransactorsInterface.CallOpts, _publicKey)
}

// GetTransactorByKey is a free data retrieval call binding the contract method 0x41e2829b.
//
// Solidity: function getTransactorByKey(string _publicKey) view returns((uint64,uint64,bool,string,string))
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceCallerSession) GetTransactorByKey(_publicKey string) (ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	return _ETNPriorityTransactorsInterface.Contract.GetTransactorByKey(&_ETNPriorityTransactorsInterface.CallOpts, _publicKey)
}

// GetTransactors is a free data retrieval call binding the contract method 0x2d26b309.
//
// Solidity: function getTransactors() view returns((uint64,uint64,bool,string,string)[])
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceCaller) GetTransactors(opts *bind.CallOpts) ([]ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	var out []interface{}
	err := _ETNPriorityTransactorsInterface.contract.Call(opts, &out, "getTransactors")

	if err != nil {
		return *new([]ETNPriorityTransactorsInterfaceTransactorMeta), err
	}

	out0 := *abi.ConvertType(out[0], new([]ETNPriorityTransactorsInterfaceTransactorMeta)).(*[]ETNPriorityTransactorsInterfaceTransactorMeta)

	return out0, err

}

// GetTransactors is a free data retrieval call binding the contract method 0x2d26b309.
//
// Solidity: function getTransactors() view returns((uint64,uint64,bool,string,string)[])
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceSession) GetTransactors() ([]ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	return _ETNPriorityTransactorsInterface.Contract.GetTransactors(&_ETNPriorityTransactorsInterface.CallOpts)
}

// GetTransactors is a free data retrieval call binding the contract method 0x2d26b309.
//
// Solidity: function getTransactors() view returns((uint64,uint64,bool,string,string)[])
func (_ETNPriorityTransactorsInterface *ETNPriorityTransactorsInterfaceCallerSession) GetTransactors() ([]ETNPriorityTransactorsInterfaceTransactorMeta, error) {
	return _ETNPriorityTransactorsInterface.Contract.GetTransactors(&_ETNPriorityTransactorsInterface.CallOpts)
}
