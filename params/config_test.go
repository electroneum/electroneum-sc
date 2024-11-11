// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
)

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new *ChainConfig
		head        uint64
		wantErr     *ConfigCompatError
	}
	tests := []test{
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, head: 0, wantErr: nil},
		{stored: AllEthashProtocolChanges, new: AllEthashProtocolChanges, head: 100, wantErr: nil},
		{
			stored:  &ChainConfig{EIP150Block: big.NewInt(10)},
			new:     &ChainConfig{EIP150Block: big.NewInt(20)},
			head:    9,
			wantErr: nil,
		},
		{
			stored: AllEthashProtocolChanges,
			new:    &ChainConfig{HomesteadBlock: nil},
			head:   3,
			wantErr: &ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(0),
				NewConfig:    nil,
				RewindTo:     0,
			},
		},
		{
			stored: AllEthashProtocolChanges,
			new:    &ChainConfig{HomesteadBlock: big.NewInt(1)},
			head:   3,
			wantErr: &ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(0),
				NewConfig:    big.NewInt(1),
				RewindTo:     0,
			},
		},
		{
			stored: &ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)},
			new:    &ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)},
			head:   25,
			wantErr: &ConfigCompatError{
				What:         "EIP150 fork block",
				StoredConfig: big.NewInt(10),
				NewConfig:    big.NewInt(20),
				RewindTo:     9,
			},
		},
		{
			stored:  &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:     &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(30)},
			head:    40,
			wantErr: nil,
		},
		{
			stored: &ChainConfig{ConstantinopleBlock: big.NewInt(30)},
			new:    &ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(31)},
			head:   40,
			wantErr: &ConfigCompatError{
				What:         "Petersburg fork block",
				StoredConfig: nil,
				NewConfig:    big.NewInt(31),
				RewindTo:     30,
			},
		},
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.head)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nhead: %v\nerr: %v\nwant: %v", test.stored, test.new, test.head, err, test.wantErr)
		}
	}
}

func TestCheckTransitionsData(t *testing.T) {
	type test struct {
		stored  *ChainConfig
		wantErr error
	}
	var ibftTransitionsConfig, invalidBlockOrder []Transition
	tranI0 := Transition{big.NewInt(0), 30000, 5, 10, 60, common.Address{}, 0}
	tranI5 := Transition{big.NewInt(5), 30000, 5, 10, 60, common.Address{}, 0}
	tranI8 := Transition{big.NewInt(8), 30000, 5, 10, 60, common.Address{}, 0}
	tranI10 := Transition{big.NewInt(10), 30000, 5, 10, 60, common.Address{}, 0}

	ibftTransitionsConfig = append(ibftTransitionsConfig, tranI0, tranI5, tranI8, tranI10)
	invalidBlockOrder = append(invalidBlockOrder, tranI8, tranI5)

	tests := []test{
		{stored: MainnetChainConfig, wantErr: nil},
		{stored: AllEthashProtocolChanges, wantErr: nil},
		{stored: AllCliqueProtocolChanges, wantErr: nil},
		{stored: TestChainConfig, wantErr: nil},
		{
			stored:  &ChainConfig{IBFT: &IBFTConfig{}},
			wantErr: nil,
		},
		{
			stored:  &ChainConfig{IBFT: &IBFTConfig{}, Transitions: ibftTransitionsConfig},
			wantErr: nil,
		},
		{
			stored:  &ChainConfig{Transitions: ibftTransitionsConfig},
			wantErr: nil,
		},
		{
			stored:  &ChainConfig{Transitions: invalidBlockOrder},
			wantErr: ErrBlockOrder,
		},
		{
			stored:  &ChainConfig{Transitions: []Transition{{nil, 30000, 5, 10, 60, common.Address{}, 0}}},
			wantErr: ErrBlockNumberMissing,
		},
		{
			stored:  &ChainConfig{Transitions: []Transition{{Block: big.NewInt(0)}}},
			wantErr: nil,
		},
	}

	for _, test := range tests {
		err := test.stored.CheckTransitionsData()
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nerr: %v\nwant: %v", test.stored, err, test.wantErr)
		}
	}
}

func TestGetPriorityTransactorsContractAddress(t *testing.T) {
	address, address1, address2, address3 := common.Address{}, common.Address{0x2}, common.Address{0x4}, common.Address{0x6}

	config := TestChainConfig
	config.Transitions = []Transition{{
		Block:                              big.NewInt(2),
		PriorityTransactorsContractAddress: address1,
	}, {
		Block:                              big.NewInt(4),
		PriorityTransactorsContractAddress: address2,
	}, {
		Block:                              big.NewInt(6),
		PriorityTransactorsContractAddress: address3,
	}}

	type test struct {
		blockNumber     int64
		expectedAddress common.Address
	}
	tests := []test{
		{0, address},
		{1, address},
		{2, address1},
		{3, address1},
		{4, address2},
		{5, address2},
		{10, address3},
		{100, address3},
	}

	for _, test := range tests {
		c := config.GetPriorityTransactorsContractAddress(big.NewInt(test.blockNumber))
		if !reflect.DeepEqual(c, test.expectedAddress) {
			t.Errorf("error mismatch:\nexpected: %v\ngot: %v\n", test.expectedAddress, c)
		}
	}
}
