// Copyright 2026 The electroneum-sc Authors
// This file is part of the electroneum-sc library.
//
// The electroneum-sc library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The electroneum-sc library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the electroneum-sc library. If not, see <http://www.gnu.org/licenses/>.

package filters

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/ethash"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/rawdb"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/trie"
)

// makeAddresses returns n distinct synthetic addresses.
func makeAddresses(n int) []common.Address {
	out := make([]common.Address, n)
	for i := 0; i < n; i++ {
		out[i] = common.BytesToAddress(big.NewInt(int64(i + 1)).Bytes())
	}
	return out
}

// makeTopics returns a single-position topic list with n entries.
func makeTopics(n int) [][]common.Hash {
	row := make([]common.Hash, n)
	for i := 0; i < n; i++ {
		row[i] = common.BytesToHash(big.NewInt(int64(i + 1)).Bytes())
	}
	return [][]common.Hash{row}
}

// newAPIWithLimit builds a FilterAPI backed by an empty in-memory database with
// the supplied log-query cap.
//
// go-ethereum 1.13 renamed PublicFilterAPI to FilterAPI and moved the per-node
// settings out of the constructor into the FilterSystem's Config, so the caps
// are installed there rather than passed positionally.
func newAPIWithLimit(limit int) *FilterAPI {
	backend := &testBackend{db: rawdb.NewMemoryDatabase()}
	sys := NewFilterSystem(backend, Config{LogQueryLimit: limit})
	return NewFilterAPI(sys, false)
}

// newAPIWithRangeLimit builds a FilterAPI whose block-range cap is set.
func newAPIWithRangeLimit(limit uint64) *FilterAPI {
	backend := &testBackend{db: rawdb.NewMemoryDatabase()}
	sys := NewFilterSystem(backend, Config{RangeLimit: limit})
	return NewFilterAPI(sys, false)
}

func TestCheckLogQueryLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		limit     int
		addresses int
		topics    int
		wantErr   bool
	}{
		{"disabled cap accepts huge address list", 0, 100_000, 0, false},
		{"under cap is accepted", 10, 10, 10, false},
		{"address overflow is rejected", 10, 11, 0, true},
		{"topic overflow is rejected", 10, 0, 11, true},
		{"both within cap is accepted", 1000, 1000, 1000, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckLogQueryLimit(tc.limit, makeAddresses(tc.addresses), makeTopics(tc.topics))
			if tc.wantErr && !errors.Is(err, ErrExceedLogQueryLimit) {
				t.Fatalf("expected ErrExceedLogQueryLimit, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}

func TestGetLogsRejectsOversizedAddresses(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(10)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Addresses: makeAddresses(11),
	}
	if _, err := api.GetLogs(context.Background(), crit); !errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("GetLogs: expected ErrExceedLogQueryLimit, got %v", err)
	}
}

func TestGetLogsRejectsOversizedTopics(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(10)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Topics:    makeTopics(11),
	}
	if _, err := api.GetLogs(context.Background(), crit); !errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("GetLogs: expected ErrExceedLogQueryLimit for oversized topics, got %v", err)
	}
}

func TestNewFilterRejectsOversizedAddresses(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(10)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Addresses: makeAddresses(11),
	}
	if _, err := api.NewFilter(crit); !errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("NewFilter: expected ErrExceedLogQueryLimit, got %v", err)
	}
}

func TestLogsSubscribeRejectsOversizedAddresses(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(10)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Addresses: makeAddresses(11),
	}
	// No notifier in context: the limit check runs first and must reject
	// before reaching the unsupported-notifier path.
	if _, err := api.Logs(context.Background(), crit); !errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("Logs: expected ErrExceedLogQueryLimit, got %v", err)
	}
}

func TestZeroLimitDisablesCap(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(0)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Addresses: makeAddresses(50_000),
	}
	// We don't care whether the query succeeds — only that the limit check
	// does not reject it.
	if _, err := api.GetLogs(context.Background(), crit); errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("GetLogs: unexpected ErrExceedLogQueryLimit with zero (disabled) cap")
	}
}

func TestUnderLimitIsAccepted(t *testing.T) {
	t.Parallel()

	api := newAPIWithLimit(1000)
	crit := FilterCriteria{
		FromBlock: big.NewInt(0),
		ToBlock:   big.NewInt(1),
		Addresses: makeAddresses(1000),
		Topics:    makeTopics(1000),
	}
	if _, err := api.GetLogs(context.Background(), crit); errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("GetLogs: unexpected ErrExceedLogQueryLimit at exact cap")
	}
	if _, err := api.NewFilter(crit); errors.Is(err, ErrExceedLogQueryLimit) {
		t.Fatalf("NewFilter: unexpected ErrExceedLogQueryLimit at exact cap")
	}
}

// newBackendWithChain returns a testBackend whose canonical chain has the given
// number of blocks above genesis, so that "latest" resolves to height n.
//
// go-ethereum 1.13 removed core.GenesisBlockForTesting, so the genesis is
// committed through a core.Genesis spec instead.
func newBackendWithChain(t *testing.T, n int) *testBackend {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	addr := common.BytesToAddress([]byte("tester"))
	gspec := &core.Genesis{
		Config:  params.TestChainConfig,
		Alloc:   core.GenesisAlloc{addr: {Balance: big.NewInt(1000000)}},
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	if _, err := gspec.Commit(db, trie.NewDatabase(db, nil)); err != nil {
		t.Fatalf("commit genesis: %v", err)
	}
	chain, receipts := core.GenerateChain(gspec.Config, gspec.ToBlock(), ethash.NewFaker(), db, n, func(i int, gen *core.BlockGen) {})
	for i, block := range chain {
		rawdb.WriteBlock(db, block)
		rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		rawdb.WriteHeadBlockHash(db, block.Hash())
		rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), receipts[i])
	}
	return &testBackend{db: db}
}

// TestRangeLimit verifies that a non-zero rangeLimit rejects range queries whose
// resolved span (end - begin) exceeds the cap, that the "latest" sentinel is
// resolved before the comparison, and that a zero cap disables the check. This
// mirrors upstream go-ethereum's --rpc.rangelimit behaviour.
func TestRangeLimit(t *testing.T) {
	t.Parallel()

	const head = 20
	backend := newBackendWithChain(t, head)

	assertExceeds := func(t *testing.T, begin, end int64, limit uint64, want bool) {
		t.Helper()
		// 1.13 moved NewRangeFilter onto FilterSystem and takes the cap from its
		// Config rather than as an argument.
		sys := NewFilterSystem(backend, Config{RangeLimit: limit})
		filter := sys.NewRangeFilter(begin, end, nil, nil)
		_, err := filter.Logs(context.Background())
		got := errors.Is(err, ErrExceedRangeLimit)
		if got != want {
			t.Fatalf("begin=%d end=%d limit=%d: range-limit error = %v (err=%v), want %v", begin, end, limit, got, err, want)
		}
	}

	// Span 0..20 (= 20) with cap 5 must be rejected.
	assertExceeds(t, 0, head, 5, true)
	// The "latest" sentinel (-1) must resolve to head before the comparison,
	// so 0..latest with cap 5 is rejected just the same.
	assertExceeds(t, 0, -1, 5, true)
	// A span within the cap is accepted.
	assertExceeds(t, 0, 5, 5, false)
	// A zero cap disables the check entirely, even for the full range.
	assertExceeds(t, 0, -1, 0, false)
}
