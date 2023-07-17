// Copyright 2023 Electroneum Ltd
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
	"encoding/json"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/ethdb"
)

const (
	dbKeyBlockSnapshotPrefix = "istanbul-block-snapshot"
)

type BlockSnapshotData struct {
	PriorityTransactors common.PriorityTransactorMap
}

// Snapshot is the state of the authorization voting at a given point in time.
type BlockSnapshot struct {
	Number *big.Int // Block number where the snapshot was created
	Data   BlockSnapshotData
}

// newSnapshot create a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent validators, so only ever use if for
// the genesis block.
func newBlockSnapshot(number *big.Int, data BlockSnapshotData) *BlockSnapshot {
	blockSnap := &BlockSnapshot{
		Number: number,
		Data:   data,
	}
	return blockSnap
}

// loadSnapshot loads an existing snapshot from the database.
func loadBlockSnapshot(db ethdb.Database, blockNumber *big.Int) (*BlockSnapshot, error) {
	blob, err := db.Get(append([]byte(dbKeyBlockSnapshotPrefix), blockNumber.Bytes()[:]...))
	if err != nil {
		return nil, err
	}
	blockSnap := new(BlockSnapshot)
	if err := json.Unmarshal(blob, blockSnap); err != nil {
		return nil, err
	}
	return blockSnap, nil
}

// store inserts the snapshot into the database.
func (s *BlockSnapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte(dbKeyBlockSnapshotPrefix), s.Number.Bytes()[:]...), blob)
}

type blockSnapshotJSON struct {
	Number *big.Int `json:"number"`

	// Data
	PriorityTransactors common.PriorityTransactorMap `json:"prioritytransactors"`
}

func (s *BlockSnapshot) toJSONStruct() *blockSnapshotJSON {
	return &blockSnapshotJSON{
		Number:              s.Number,
		PriorityTransactors: s.Data.PriorityTransactors,
	}
}

// Unmarshal from a json byte array
func (s *BlockSnapshot) UnmarshalJSON(b []byte) error {
	var j blockSnapshotJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	s.Number = j.Number
	s.Data = BlockSnapshotData{PriorityTransactors: j.PriorityTransactors}
	return nil
}

// Marshal to a json byte array
func (s *BlockSnapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}
