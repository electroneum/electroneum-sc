// Copyright 2022 Electroneum Ltd
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
	"encoding/json"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/ethdb"
)

const (
	dbKeyEmissionPrefix = "istanbul-emission"
)

// Emission is the state of ETN circulating supply at given point in time
type Emission struct {
	Number            uint64
	Hash              common.Hash
	CirculatingSupply *big.Int
}

func newEmission(blockNumber uint64, hash common.Hash, circulatingSupply *big.Int) *Emission {
	emission := &Emission{
		Number:            blockNumber,
		Hash:              hash,
		CirculatingSupply: circulatingSupply,
	}
	return emission
}

// loadEmission loads an existing emission snapshot from the database.
func loadEmission(hash common.Hash, db ethdb.Database) (*Emission, error) {
	blob, err := db.Get(append([]byte(dbKeyEmissionPrefix), hash[:]...))
	if err != nil {
		return nil, err
	}
	emission := new(Emission)
	if err := json.Unmarshal(blob, emission); err != nil {
		return nil, err
	}

	return emission, nil
}

// store inserts the emission snapshot into the database.
func (e *Emission) store(db ethdb.Database) error {
	blob, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return db.Put(append([]byte(dbKeyEmissionPrefix), e.Hash[:]...), blob)
}

// copy creates a deep copy of the emission snapshot
func (e *Emission) copy() *Emission {
	cpy := &Emission{
		Number:            e.Number,
		Hash:              e.Hash,
		CirculatingSupply: e.CirculatingSupply,
	}

	return cpy
}

type emissionJSON struct {
	Number            uint64      `json:"number"`
	Hash              common.Hash `json:"hash"`
	CirculatingSupply *big.Int    `json:"circulatingsupply"`
}

func (e *Emission) toJSONStruct() *emissionJSON {
	return &emissionJSON{
		Number:            e.Number,
		Hash:              e.Hash,
		CirculatingSupply: e.CirculatingSupply,
	}
}

// Unmarshal from a json byte array
func (e *Emission) UnmarshalJSON(b []byte) error {
	var j emissionJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	e.Number = j.Number
	e.Hash = j.Hash
	e.CirculatingSupply = j.CirculatingSupply

	return nil
}

// Marshal to a json byte array
func (e *Emission) MarshalJSON() ([]byte, error) {
	j := e.toJSONStruct()
	return json.Marshal(j)
}
