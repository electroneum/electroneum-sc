// Copyright 2020 The go-ethereum Authors
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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/clique"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/state"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/eth/downloader"
	"github.com/electroneum/electroneum-sc/ethdb/memorydb"
	"github.com/electroneum/electroneum-sc/event"
	"github.com/electroneum/electroneum-sc/trie"
)

type mockBackend struct {
	bc     *core.BlockChain
	txPool *core.TxPool
}

func NewMockBackend(bc *core.BlockChain, txPool *core.TxPool) *mockBackend {
	return &mockBackend{
		bc:     bc,
		txPool: txPool,
	}
}

func (m *mockBackend) BlockChain() *core.BlockChain {
	return m.bc
}

func (m *mockBackend) TxPool() *core.TxPool {
	return m.txPool
}

func (m *mockBackend) StateAtBlock(block *types.Block, reexec uint64, base *state.StateDB, checkLive bool, preferDisk bool) (statedb *state.StateDB, err error) {
	return nil, errors.New("not supported")
}

type testBlockChain struct {
	statedb       *state.StateDB
	gasLimit      uint64
	chainHeadFeed *event.Feed
}

func (bc *testBlockChain) CurrentBlock() *types.Block {
	return types.NewBlock(&types.Header{
		GasLimit: bc.gasLimit,
	}, nil, nil, nil, trie.NewStackTrie(nil))
}

func (bc *testBlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return bc.CurrentBlock()
}

func (bc *testBlockChain) StateAt(common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func (bc *testBlockChain) GetPriorityTransactorsCache() common.PriorityTransactorMap {
	return common.PriorityTransactorMap{}
}

// GetPriorityTransactorsForState receives the priority transactor list appropriate for the current state
func (bc *testBlockChain) GetPriorityTransactorsForState(header *types.Header, state *state.StateDB) common.PriorityTransactorMap {
	return common.PriorityTransactorMap{}
}

func (bc *testBlockChain) GetPriorityTransactorsForStateAt(header *types.Header, state *state.StateDB, addressBlock *big.Int) common.PriorityTransactorMap {
	return bc.GetPriorityTransactorsForState(header, state)
}

func TestMiner(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	// Start the downloader. A bare StartEvent must not pause mining; only a
	// verified ProgressEvent (a peer that actually served a chain head) should.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)
	// Stop the downloader and wait for the update loop to run
	mux.Post(downloader.DoneEvent{})
	waitForMiningState(t, miner, true)

	// Subsequent downloader events after a successful DoneEvent should not cause the
	// miner to start or stop. This prevents a security vulnerability
	// that would allow entities to present fake high blocks that would
	// stop mining operations by causing a downloader sync
	// until it was discovered they were invalid, whereon mining would resume.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, true)

	mux.Post(downloader.FailedEvent{})
	waitForMiningState(t, miner, true)
}

// TestMinerBareStartEventDoesNotStop verifies that an unauthenticated peer
// cannot hold the miner (and with it the IBFT/QBFT consensus engine) out of
// action merely by making the node "start syncing" from a peer that advertises
// a high total difficulty and then never delivers any headers. The miner must
// only pause once a sync reports verifiable ProgressEvent.
func TestMinerBareStartEventDoesNotStop(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)

	// Simulate the attack: each cycle the node "starts syncing" from a
	// non-delivering high-TD peer (StartEvent) and the sync ends only on the
	// request timeout (FailedEvent), with no ProgressEvent in between. The miner
	// must not pause while the sync is in flight (this is where the old code
	// stopped consensus for the whole ~60s timeout), and must still be mining
	// after the timeout fires.
	for i := 0; i < 3; i++ {
		mux.Post(downloader.StartEvent{})
		assertMiningStays(t, miner, true)
		mux.Post(downloader.FailedEvent{})
		assertMiningStays(t, miner, true)
	}

	// A sync that actually makes progress must still pause mining as before...
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)

	// ...and resume once it completes.
	mux.Post(downloader.DoneEvent{})
	waitForMiningState(t, miner, true)
}

// TestMinerDownloaderFirstFails tests that mining is only
// permitted to run indefinitely once the downloader sees a DoneEvent (success).
// An initial FailedEvent should allow mining to stop on a subsequent
// downloader StartEvent.
func TestMinerDownloaderFirstFails(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	// Start the downloader and let it make progress.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)

	// Stop the downloader and wait for the update loop to run
	mux.Post(downloader.FailedEvent{})
	waitForMiningState(t, miner, true)

	// Since the downloader hasn't yet emitted a successful DoneEvent,
	// we expect the miner to stop on the next sync that makes progress.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)

	// Downloader finally succeeds.
	mux.Post(downloader.DoneEvent{})
	waitForMiningState(t, miner, true)

	// Downloader starts again.
	// Since it has achieved a DoneEvent once, we expect miner
	// state to be unchanged.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, true)

	mux.Post(downloader.FailedEvent{})
	waitForMiningState(t, miner, true)
}

func TestMinerStartStopAfterDownloaderEvents(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	// Start the downloader and let it make progress.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)

	// Downloader finally succeeds.
	mux.Post(downloader.DoneEvent{})
	waitForMiningState(t, miner, true)

	miner.Stop()
	waitForMiningState(t, miner, false)

	miner.Start(common.HexToAddress("0x678910"))
	waitForMiningState(t, miner, true)

	miner.Stop()
	waitForMiningState(t, miner, false)
}

func TestStartWhileDownload(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	waitForMiningState(t, miner, false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	// Stop the downloader and wait for the update loop to run
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)
	// Starting the miner after the downloader should not work
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, false)
}

func TestStartStopMiner(t *testing.T) {
	miner, _, cleanup := createMiner(t)
	defer cleanup(false)
	waitForMiningState(t, miner, false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	miner.Stop()
	waitForMiningState(t, miner, false)
}

func TestCloseMiner(t *testing.T) {
	miner, _, cleanup := createMiner(t)
	defer cleanup(true)
	waitForMiningState(t, miner, false)
	miner.Start(common.HexToAddress("0x12345"))
	waitForMiningState(t, miner, true)
	// Terminate the miner and wait for the update loop to run
	miner.Close()
	waitForMiningState(t, miner, false)
}

// TestMinerSetEtherbase checks that etherbase becomes set even if mining isn't
// possible at the moment
func TestMinerSetEtherbase(t *testing.T) {
	miner, mux, cleanup := createMiner(t)
	defer cleanup(false)
	// Start with a 'bad' mining address
	miner.Start(common.HexToAddress("0xdead"))
	waitForMiningState(t, miner, true)
	// Start the downloader and let it make progress.
	mux.Post(downloader.StartEvent{})
	mux.Post(downloader.ProgressEvent{})
	waitForMiningState(t, miner, false)
	// Now user tries to configure proper mining address
	miner.Start(common.HexToAddress("0x1337"))
	// Stop the downloader and wait for the update loop to run
	mux.Post(downloader.DoneEvent{})

	waitForMiningState(t, miner, true)
	// The miner should now be using the good address
	if got, exp := miner.coinbase, common.HexToAddress("0x1337"); got != exp {
		t.Fatalf("Wrong coinbase, got %x expected %x", got, exp)
	}
}

// waitForMiningState waits until either
// * the desired mining state was reached
// * a timeout was reached which fails the test
func waitForMiningState(t *testing.T, m *Miner, mining bool) {
	t.Helper()

	var state bool
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		if state = m.Mining(); state == mining {
			return
		}
	}
	t.Fatalf("Mining() == %t, want %t", state, mining)
}

// assertMiningStays fails the test if the miner leaves the given state at any
// point within a short window. It is used to assert that an event does NOT
// change the mining state, which waitForMiningState cannot verify since it
// returns as soon as the desired state is observed.
func assertMiningStays(t *testing.T, m *Miner, mining bool) {
	t.Helper()

	for i := 0; i < 30; i++ {
		time.Sleep(10 * time.Millisecond)
		if state := m.Mining(); state != mining {
			t.Fatalf("Mining() == %t, want it to stay %t", state, mining)
		}
	}
}

func createMiner(t *testing.T) (*Miner, *event.TypeMux, func(skipMiner bool)) {
	// Create Ethash config
	config := Config{
		Etherbase: common.HexToAddress("123456789"),
	}
	// Create chainConfig
	memdb := memorydb.New()
	chainDB := rawdb.NewDatabase(memdb)
	genesis := core.DeveloperGenesisBlock(15, 11_500_000, common.HexToAddress("12345"))
	chainConfig, _, err := core.SetupGenesisBlock(chainDB, genesis)
	if err != nil {
		t.Fatalf("can't create new chain config: %v", err)
	}
	// Create consensus engine
	engine := clique.New(chainConfig.Clique, chainDB)
	// Create Ethereum backend
	bc, err := core.NewBlockChain(chainDB, nil, chainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("can't create new chain %v", err)
	}
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(chainDB), nil)
	blockchain := &testBlockChain{statedb, 10000000, new(event.Feed)}

	pool := core.NewTxPool(testTxPoolConfig, chainConfig, blockchain)
	backend := NewMockBackend(bc, pool)
	// Create event Mux
	mux := new(event.TypeMux)
	// Create Miner
	miner := New(backend, &config, chainConfig, mux, engine, nil)
	cleanup := func(skipMiner bool) {
		bc.Stop()
		engine.Close()
		pool.Stop()
		if !skipMiner {
			miner.Close()
		}
	}
	return miner, mux, cleanup
}
