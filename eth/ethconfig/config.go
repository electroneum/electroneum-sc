// Copyright 2021 The go-ethereum Authors
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

// Package ethconfig contains the configuration of the ETH and LES protocols.
package ethconfig

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulBackend "github.com/electroneum/electroneum-sc/consensus/istanbul/backend"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/beacon"
	"github.com/electroneum/electroneum-sc/consensus/clique"
	"github.com/electroneum/electroneum-sc/consensus/ethash"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/txpool/blobpool"
	"github.com/electroneum/electroneum-sc/core/txpool/legacypool"
	"github.com/electroneum/electroneum-sc/eth/downloader"
	"github.com/electroneum/electroneum-sc/eth/gasprice"
	"github.com/electroneum/electroneum-sc/ethdb"
	"github.com/electroneum/electroneum-sc/miner"
	"github.com/electroneum/electroneum-sc/params"
)

// FullNodeGPO contains default gasprice oracle settings for full node.
var FullNodeGPO = gasprice.Config{
	Blocks:           20,
	Percentile:       60,
	MaxHeaderHistory: 1024,
	MaxBlockHistory:  1024,
	MaxPrice:         gasprice.DefaultMaxPrice,
	IgnorePrice:      gasprice.DefaultIgnorePrice,
}

// LightClientGPO contains default gasprice oracle settings for light client.
var LightClientGPO = gasprice.Config{
	Blocks:           2,
	Percentile:       60,
	MaxHeaderHistory: 300,
	MaxBlockHistory:  5,
	MaxPrice:         gasprice.DefaultMaxPrice,
	IgnorePrice:      gasprice.DefaultIgnorePrice,
}

// Defaults contains default settings for use on the Ethereum main net.
var Defaults = Config{
	SyncMode:  downloader.SnapSync,
	NetworkId: 52014, // Electroneum mainnet; the eth handshake rejects a mismatch
	// Electroneum raised this from go-ethereum's 2350000: both mean "about a
	// year", but ETN produces a block every 5 seconds rather than every 12.
	// Inheriting the upstream number silently discards ~4M blocks of
	// transaction index, so eth_getTransactionByHash and
	// eth_getTransactionReceipt start returning null for anything older.
	TxLookupLimit:      6307200,
	TransactionHistory: 6307200,
	StateHistory:       params.FullImmutabilityThreshold,
	StateScheme:        rawdb.HashScheme,
	LightPeers:         100,
	DatabaseCache:      512,
	TrieCleanCache:     154,
	TrieDirtyCache:     256,
	TrieTimeout:        60 * time.Minute,
	SnapshotCache:      102,
	FilterLogCacheSize: 32,
	Miner:              miner.DefaultConfig,
	TxPool:             legacypool.DefaultConfig,
	BlobPool:           blobpool.DefaultConfig,
	RPCGasCap:          50000000,
	RPCEVMTimeout:      5 * time.Second,
	GPO:                FullNodeGPO,
	// Electroneum's cap, not go-ethereum's 1 ether. The unit is ETN, and ETN is
	// worth far less than ether, so upstream's cap would reject transactions
	// the rest of the network accepts -- a 30M-gas call needs only a 33 gwei
	// fee cap to exceed 1 ETN.
	RPCTxFeeCap:      100000,
	RPCLogQueryLimit: 1000,
	RangeLimit:       0, // disabled by default, matching upstream go-ethereum
}

//go:generate go run github.com/fjl/gencodec -type Config -formats toml -out gen_config.go

// Config contains configuration options for of the ETH and LES protocols.
type Config struct {
	// The genesis block, which is inserted if the database is empty.
	// If nil, the Ethereum main net block is used.
	Genesis *core.Genesis `toml:",omitempty"`

	// Protocol options
	NetworkId uint64 // Network ID to use for selecting peers to connect to
	SyncMode  downloader.SyncMode

	// This can be set to list of enrtree:// URLs which will be queried for
	// for nodes to connect to.
	EthDiscoveryURLs  []string
	SnapDiscoveryURLs []string

	NoPruning  bool // Whether to disable pruning and flush everything to disk
	NoPrefetch bool // Whether to disable prefetching and only load state on demand

	// Deprecated, use 'TransactionHistory' instead.
	TxLookupLimit      uint64 `toml:",omitempty"` // The maximum number of blocks from head whose tx indices are reserved.
	TransactionHistory uint64 `toml:",omitempty"` // The maximum number of blocks from head whose tx indices are reserved.
	StateHistory       uint64 `toml:",omitempty"` // The maximum number of blocks from head whose state histories are reserved.
	StateScheme        string `toml:",omitempty"` // State scheme used to store ethereum state and merkle trie nodes on top

	// RequiredBlocks is a set of block number -> hash mappings which must be in the
	// canonical chain of all remote peers. Setting the option makes geth verify the
	// presence of these blocks for every new peer connection.
	RequiredBlocks map[uint64]common.Hash `toml:"-"`

	// Light client options
	LightServ        int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress     int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress      int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers       int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune     bool `toml:",omitempty"` // Whether to disable light chain pruning
	LightNoSyncServe bool `toml:",omitempty"` // Whether to serve light clients before syncing

	// Database options
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int
	DatabaseFreezer    string

	TrieCleanCache int
	TrieDirtyCache int
	TrieTimeout    time.Duration
	SnapshotCache  int
	Preimages      bool

	// This is the number of blocks for which logs will be cached in the filter system.
	FilterLogCacheSize int

	// Mining options
	Miner miner.Config

	// Transaction pool options
	TxPool   legacypool.Config
	BlobPool blobpool.Config

	// Gas Price Oracle options
	GPO gasprice.Config

	// Enables tracking of SHA3 preimages in the VM
	EnablePreimageRecording bool

	// Miscellaneous options
	DocRoot string `toml:"-"`

	// RPCGasCap is the global gas cap for eth-call variants.
	RPCGasCap uint64

	// RPCEVMTimeout is the global timeout for eth-call.
	RPCEVMTimeout time.Duration

	// RPCLogQueryLimit caps how many addresses, or topics at a single position,
	// an eth_getLogs / eth_newFilter / logs-subscription filter may name. Each
	// entry becomes a bloom clause evaluated on every block in the range, so an
	// unbounded list lets one unauthenticated request monopolise an RPC worker.
	// 0 disables the cap.
	RPCLogQueryLimit int

	// RangeLimit caps the block span a single range log query may cover.
	// 0 disables the cap, matching upstream go-ethereum.
	RangeLimit uint64

	// RPCTxFeeCap is the global transaction fee(price * gaslimit) cap for
	// send-transaction variants. The unit is ether.
	RPCTxFeeCap float64

	// OverrideCancun (TODO: remove after the fork)
	OverrideCancun *uint64 `toml:",omitempty"`

	// OverrideVerkle (TODO: remove after the fork)
	OverrideVerkle *uint64 `toml:",omitempty"`
}

// CreateConsensusEngine creates a consensus engine for the given chain config.
// Clique is allowed for now to live standalone, but ethash is forbidden and can
// only exist on already merged networks.
// CreateConsensusEngine builds the consensus engine for a chain.
//
// nodeKey is Electroneum's signing identity. Upstream's signature has no place
// for it -- geth stopped handing this function a *node.Node -- so it is threaded
// through explicitly. Callers that can never seal (the chain subcommands, the
// LES client) pass nil and get a clearly-logged read-only engine.
func CreateConsensusEngine(config *params.ChainConfig, db ethdb.Database, nodeKey *ecdsa.PrivateKey) (consensus.Engine, error) {
	// Electroneum runs QBFT, so it takes precedence over every Ethereum engine.
	if config.IBFT != nil {
		// The signing key is this node's consensus identity: QBFT binds a
		// block's coinbase to its proposer, so a wrong key means a wrong
		// identity rather than a harmless one.
		//
		// Backend.New dereferences the key to derive its address, so nil is not
		// an option. Where there genuinely is no node key -- the chain
		// subcommands, which import and export but never seal -- fall back to an
		// ephemeral one and SAY SO, rather than silently running under a random
		// identity.
		key := nodeKey
		if key == nil {
			var err error
			if key, err = crypto.GenerateKey(); err != nil {
				return nil, fmt.Errorf("istanbul: ephemeral key: %w", err)
			}
			log.Warn("IBFT: no node key available, using an ephemeral identity",
				"consequence", "this engine can verify but must not seal")
		}
		return istanbulBackend.New(istanbul.Config{
			BlockPeriod:              config.IBFT.BlockPeriodSeconds,
			Epoch:                    config.IBFT.EpochLength,
			ProposerPolicy:           istanbul.NewProposerPolicy(istanbul.ProposerPolicyId(config.IBFT.ProposerPolicy)),
			RequestTimeoutSeconds:    config.IBFT.RequestTimeoutSeconds,
			MaxRequestTimeoutSeconds: config.IBFT.MaxRequestTimeoutSeconds,
			AllowedFutureBlockTime:   config.IBFT.AllowedFutureBlockTime,
			Transitions:              config.Transitions,
		}, key, db), nil
	}
	// If proof-of-authority is requested, set it up
	if config.Clique != nil {
		return beacon.New(clique.New(config.Clique, db)), nil
	}
	// If defaulting to proof-of-work, enforce an already merged network since
	// we cannot run PoW algorithms and more, so we cannot even follow a chain
	// not coordinated by a beacon node.
	if !config.TerminalTotalDifficultyPassed {
		return nil, errors.New("ethash is only supported as a historical component of already merged networks")
	}
	return beacon.New(ethash.NewFaker()), nil
}
