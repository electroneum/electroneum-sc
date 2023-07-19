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
var MainnetBootnodes = []string{
	"enode://7f854ed504a3a27f32106205b7aceaea9d96f19932ec3933062490a64ca20727ae55f2cfe3734f63595a82a409f0c77bde063b807b374e936e045660beec748d@54.144.63.205:30303",
	"enode://8ac5f6a567cf74ed6208c276d39ff7c09cd472413394dfaf4a48451213e392c2620f6f3a363334627d3cdbfe13593f7a8d0bbcbe3628bffe561682eaf3f26ec4@18.138.69.206:30303",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Electroneum Testnet network.
var TestnetBootnodes = []string{
	"enode://98156c2649f9a240c3f8b9f9311ce08c9069da0049e1f7afbfd5106f9aabf9be79e09215df0353c1e8682736a4948ac1718e97bcc70ad65977b89682bce13c47@52.87.138.69:30303",
	"enode://405be926d0f1dc48d34fec934494d8e0dfae5b77f6085e1f65f37097c8ac9efcde7093b06c6644fefe0b05ba60690275152325f8476cb2b75ec09c01a57ecc08@18.136.19.203:30303",
}

// StagenetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the stage Electroneum Testnet network.
var StagenetBootnodes = []string{}

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
