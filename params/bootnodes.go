// Copyright 2015 The go-ethereum Authors
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

import "github.com/electroneum/electroneum-sc/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Electroneum network.
var MainnetBootnodes = []string{}

// StagenetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the stage Electroneum Testnet network.
var StagenetBootnodes = []string{}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Electroneum Testnet network.
var TestnetBootnodes = []string{}

var V5Bootnodes = []string{}

//const dnsPrefix = "enrtree://AKA3AM6LPBYEUDMVNU3BSVQJ5AD45Y7YPOHJLEF6W26QOE4VTUDPE@"

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	/*var net string
	switch genesis {
	case MainnetGenesisHash:
		net = "mainnet"
	case StagenetGenesisHash:
		net = "stagenet"
	case TestnetGenesisHash:
		net = "testnet"
	default:
		return ""
	}
	return dnsPrefix + protocol + "." + net + ".ethdisco.net"*/
	return ""
}
