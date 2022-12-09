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

package backend

import (
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/rawdb"
)

func TestEmissionSaveAndLoad(t *testing.T) {
	emission := &Emission{
		Number:            10,
		Hash:              common.HexToHash("1234567890"),
		CirculatingSupply: big.NewInt(2e+18),
	}
	db := rawdb.NewMemoryDatabase()
	err := emission.store(db)
	if err != nil {
		t.Errorf("store emission snapshot failed: %v", err)
	}

	emission1, err := loadEmission(emission.Hash, db)
	if err != nil {
		t.Errorf("load emission snapshot failed: %v", err)
	}
	if emission.Number != emission1.Number {
		t.Errorf("block number mismatch: have %v, want %v", emission1.Number, emission.Number)
	}
	if emission.Hash != emission1.Hash {
		t.Errorf("hash mismatch: have %v, want %v", emission1.Hash, emission.Hash)
	}
}
