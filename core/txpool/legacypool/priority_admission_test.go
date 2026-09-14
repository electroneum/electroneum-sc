// Copyright 2024 The go-ethereum Authors
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

package legacypool

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/core/state"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/event"
	"github.com/electroneum/electroneum-sc/params"
)

// pubkeyOf returns the allowlist key for a priority signing key.
func pubkeyOf(k *ecdsa.PrivateKey) common.PublicKey {
	return common.BytesToPublicKey(crypto.FromECDSAPub(&k.PublicKey))
}

// newPriorityTx builds a doubly-signed type 0x40 transaction.
func newPriorityTx(t *testing.T, nonce uint64, feeCap, tip *big.Int, key, priorityKey *ecdsa.PrivateKey) *types.Transaction {
	t.Helper()
	tx, err := types.SignNewPriorityTx(key, priorityKey, types.LatestSignerForChainID(params.TestChainConfig.ChainID), &types.PriorityTx{
		ChainID:   params.TestChainConfig.ChainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       100000,
		To:        &common.Address{},
		Value:     big.NewInt(100),
	})
	if err != nil {
		t.Fatalf("failed to sign priority tx: %v", err)
	}
	return tx
}

// setupPriorityPool builds a pool whose chain hands back the supplied allowlist.
// Returns the pool, the sending key, and the test chain so a test can mutate the
// allowlist and reset.
func setupPriorityPool(t *testing.T, allowlist common.PriorityTransactorMap) (*LegacyPool, *ecdsa.PrivateKey, *testBlockChain) {
	t.Helper()
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(rawdb.NewMemoryDatabase()), nil)
	blockchain := newTestBlockChain(params.TestChainConfig, 10000000, statedb, new(event.Feed))
	blockchain.priorityTransactors = allowlist

	pool := New(testTxPoolConfig, blockchain)
	if err := pool.Init(new(big.Int).SetUint64(testTxPoolConfig.PriceLimit), blockchain.CurrentBlock(), makeAddressReserver()); err != nil {
		t.Fatalf("pool init: %v", err)
	}
	<-pool.initDoneCh
	t.Cleanup(func() { pool.Close() })

	key, _ := crypto.GenerateKey()
	testAddBalance(pool, crypto.PubkeyToAddress(key.PublicKey), big.NewInt(1_000_000_000_000_000))
	return pool, key, blockchain
}

// TestPriorityAllowlistResolvedForNextBlock pins the fix from electroneum PR #75:
// the pool holds head state but admits transactions for head+1, so the transactor
// contract address must be resolved for head+1. Resolving at head makes the pool
// disagree with the block its transactions actually land in across a transition.
func TestPriorityAllowlistResolvedForNextBlock(t *testing.T) {
	t.Parallel()

	_, _, chain := setupPriorityPool(t, common.PriorityTransactorMap{})

	head := chain.CurrentBlock().Number
	want := new(big.Int).Add(head, big.NewInt(1))
	if chain.priorityAddressBlock == nil {
		t.Fatal("pool never resolved the transactor allowlist")
	}
	if chain.priorityAddressBlock.Cmp(want) != 0 {
		t.Fatalf("allowlist resolved for block %v, want head+1 = %v", chain.priorityAddressBlock, want)
	}
}

// TestPriorityAdmissionRejectsUnauthorisedKey checks that a priority transaction
// signed with a key outside the allowlist is refused at submission rather than
// accepted and then dropped later by the miner.
func TestPriorityAdmissionRejectsUnauthorisedKey(t *testing.T) {
	t.Parallel()

	authorised, _ := crypto.GenerateKey()
	stranger, _ := crypto.GenerateKey()

	pool, key, _ := setupPriorityPool(t, common.PriorityTransactorMap{
		pubkeyOf(authorised): {IsGasPriceWaiver: false, EntityName: "authorised"},
	})

	tx := newPriorityTx(t, 0, big.NewInt(1_000_000_000), big.NewInt(1), key, stranger)
	err := pool.addRemoteSync(tx)
	if !errors.Is(err, errBadPriorityKey) {
		t.Fatalf("expected errBadPriorityKey, got %v", err)
	}
}

// TestPriorityAdmissionWaiverMustHaveZeroFees pins the fix from electroneum
// PR #68. A waiver sender pays nothing, so execution rejects any waiver
// transaction carrying non-zero fee fields. Without the mirrored pool rule the
// transaction is accepted and then sits in the pool forever, because every
// execution attempt fails the same way.
func TestPriorityAdmissionWaiverMustHaveZeroFees(t *testing.T) {
	t.Parallel()

	waiver, _ := crypto.GenerateKey()
	pool, key, _ := setupPriorityPool(t, common.PriorityTransactorMap{
		pubkeyOf(waiver): {IsGasPriceWaiver: true, EntityName: "waiver"},
	})

	bad := newPriorityTx(t, 0, big.NewInt(1_000_000_000), big.NewInt(1), key, waiver)
	if err := pool.addRemoteSync(bad); !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver for a fee-carrying waiver tx, got %v", err)
	}

	good := newPriorityTx(t, 0, big.NewInt(0), big.NewInt(0), key, waiver)
	if err := pool.addRemoteSync(good); err != nil {
		t.Fatalf("zero-fee waiver tx should be accepted, got %v", err)
	}
}

// TestPriorityAdmissionNonWaiverMustPay is the mirror rule: a sender without a
// waiver may not submit a zero-fee transaction.
func TestPriorityAdmissionNonWaiverMustPay(t *testing.T) {
	t.Parallel()

	payer, _ := crypto.GenerateKey()
	pool, key, _ := setupPriorityPool(t, common.PriorityTransactorMap{
		pubkeyOf(payer): {IsGasPriceWaiver: false, EntityName: "payer"},
	})

	tx := newPriorityTx(t, 0, big.NewInt(0), big.NewInt(0), key, payer)
	if err := pool.addRemoteSync(tx); !errors.Is(err, errNoGasPriceWaiver) {
		t.Fatalf("expected errNoGasPriceWaiver for a zero-fee non-waiver tx, got %v", err)
	}
}

// TestPriorityAdmissionRejectsWhenAllowlistEmpty pins the deny-by-default rule.
// core.GetPriorityTransactorsAt returns an empty map on every failure path - no
// contract configured, contract not deployed, ABI or call failure - specifically
// so that losing the transactor list withdraws privileges rather than granting
// them. Admission has to agree: an empty list authorises nobody.
func TestPriorityAdmissionRejectsWhenAllowlistEmpty(t *testing.T) {
	t.Parallel()

	stranger, _ := crypto.GenerateKey()
	pool, key, _ := setupPriorityPool(t, common.PriorityTransactorMap{})

	tx := newPriorityTx(t, 0, big.NewInt(1_000_000_000), big.NewInt(1), key, stranger)
	if err := pool.addRemoteSync(tx); !errors.Is(err, errBadPriorityKey) {
		t.Fatalf("expected errBadPriorityKey with an empty allowlist, got %v", err)
	}
}

// TestExpiredPriorityTxEvictedFromPending pins the fix from electroneum PR #81.
// When a priority key stops being authorised, a pending transaction signed with
// it must be routed through removeTx during demotion. Removing it only from the
// global lookup strands it in the pending list, where no eviction path can ever
// reclaim it because removeTx no-ops once the lookup entry is gone.
func TestExpiredPriorityTxEvictedFromPending(t *testing.T) {
	t.Parallel()

	priorityKey, _ := crypto.GenerateKey()
	pool, key, chain := setupPriorityPool(t, common.PriorityTransactorMap{
		pubkeyOf(priorityKey): {IsGasPriceWaiver: false, EntityName: "soon-revoked"},
	})

	tx := newPriorityTx(t, 0, big.NewInt(1_000_000_000), big.NewInt(1), key, priorityKey)
	if err := pool.addRemoteSync(tx); err != nil {
		t.Fatalf("failed to add priority tx: %v", err)
	}
	if pending, queued := pool.Stats(); pending != 1 || queued != 0 {
		t.Fatalf("unexpected initial state: pending=%d queued=%d", pending, queued)
	}

	// Revoke the key and let the pool observe a new head.
	chain.priorityTransactors = common.PriorityTransactorMap{}
	<-pool.requestReset(nil, nil)

	if pending, queued := pool.Stats(); pending != 0 || queued != 0 {
		t.Fatalf("revoked priority tx survived demotion: pending=%d queued=%d", pending, queued)
	}
	if pool.Get(tx.Hash()) != nil {
		t.Fatal("revoked priority tx still present in the pool lookup")
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	if list := pool.pending[addr]; list != nil && list.Len() != 0 {
		t.Fatalf("revoked priority tx stranded in the pending list (%d entries)", list.Len())
	}
}

// TestExpiredPriorityTxEvictedFromQueue pins the fix from electroneum PR #74,
// the queued-side counterpart. Removing the transaction only from the global
// lookup leaves it in the queued list, where list.Ready would still promote and
// broadcast it even though pool.Get reports it as unknown.
func TestExpiredPriorityTxEvictedFromQueue(t *testing.T) {
	t.Parallel()

	priorityKey, _ := crypto.GenerateKey()
	pool, key, chain := setupPriorityPool(t, common.PriorityTransactorMap{
		pubkeyOf(priorityKey): {IsGasPriceWaiver: false, EntityName: "soon-revoked"},
	})

	// Nonce 1 against a state nonce of 0 leaves a gap, so the tx queues.
	tx := newPriorityTx(t, 1, big.NewInt(1_000_000_000), big.NewInt(1), key, priorityKey)
	if err := pool.addRemoteSync(tx); err != nil {
		t.Fatalf("failed to queue priority tx: %v", err)
	}
	if pending, queued := pool.Stats(); pending != 0 || queued != 1 {
		t.Fatalf("unexpected initial state: pending=%d queued=%d", pending, queued)
	}

	chain.priorityTransactors = common.PriorityTransactorMap{}
	<-pool.requestReset(nil, nil)

	if pending, queued := pool.Stats(); pending != 0 || queued != 0 {
		t.Fatalf("revoked priority tx survived promotion: pending=%d queued=%d", pending, queued)
	}
	if pool.Get(tx.Hash()) != nil {
		t.Fatal("revoked priority tx still present in the pool lookup")
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	if list := pool.queue[addr]; list != nil && list.Len() != 0 {
		t.Fatalf("revoked priority tx stranded in the queued list (%d entries)", list.Len())
	}
}
