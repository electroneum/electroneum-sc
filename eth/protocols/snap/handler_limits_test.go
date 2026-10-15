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

package snap

import (
	"bytes"
	"errors"
	"testing"

	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/rlp"
)

// Reported via Bugcrowd: long lists of one-byte items in snap packets decode
// into many times their wire size, so list lengths are capped before decoding.

func encodeMsg(t *testing.T, val interface{}) p2p.Msg {
	enc, err := rlp.EncodeToBytes(val)
	if err != nil {
		t.Fatal(err)
	}
	return p2p.Msg{Size: uint32(len(enc)), Payload: bytes.NewReader(enc)}
}

func oneByteItems(n int) [][]byte {
	items := make([][]byte, n)
	for i := range items {
		items[i] = []byte{0x01}
	}
	return items
}

func TestByteCodesItemLimit(t *testing.T) {
	ok := &ByteCodesPacket{ID: 1, Codes: oneByteItems(maxPacketItems)}
	if err := decodeLimited(encodeMsg(t, ok), new(ByteCodesPacket), 1); err != nil {
		t.Fatalf("packet at the limit rejected: %v", err)
	}
	over := &ByteCodesPacket{ID: 1, Codes: oneByteItems(maxPacketItems + 1)}
	if err := decodeLimited(encodeMsg(t, over), new(ByteCodesPacket), 1); !errors.Is(err, errTooManyItems) {
		t.Fatalf("oversized packet: have %v, want %v", err, errTooManyItems)
	}
}

func TestStorageRangesItemLimit(t *testing.T) {
	over := &StorageRangesPacket{ID: 1, Slots: make([][]*StorageData, maxPacketItems+1)}
	if err := decodeLimited(encodeMsg(t, over), new(StorageRangesPacket), 1, 2); !errors.Is(err, errTooManyItems) {
		t.Fatalf("oversized slots: have %v, want %v", err, errTooManyItems)
	}
	over = &StorageRangesPacket{ID: 1, Proof: oneByteItems(maxPacketItems + 1)}
	if err := decodeLimited(encodeMsg(t, over), new(StorageRangesPacket), 1, 2); !errors.Is(err, errTooManyItems) {
		t.Fatalf("oversized proof: have %v, want %v", err, errTooManyItems)
	}
}

func TestGetTrieNodesItemLimit(t *testing.T) {
	ok := &GetTrieNodesPacket{ID: 1, Paths: []TrieNodePathSet{oneByteItems(maxPacketItems / 2), oneByteItems(maxPacketItems / 2)}}
	if err := decodeLimitedTrieNodesRequest(encodeMsg(t, ok), new(GetTrieNodesPacket)); err != nil {
		t.Fatalf("request at the limit rejected: %v", err)
	}
	// The cap is on the total, not per path set.
	over := &GetTrieNodesPacket{ID: 1, Paths: []TrieNodePathSet{oneByteItems(maxPacketItems / 2), oneByteItems(maxPacketItems/2 + 1)}}
	if err := decodeLimitedTrieNodesRequest(encodeMsg(t, over), new(GetTrieNodesPacket)); !errors.Is(err, errTooManyItems) {
		t.Fatalf("oversized request: have %v, want %v", err, errTooManyItems)
	}
	over = &GetTrieNodesPacket{ID: 1, Paths: make([]TrieNodePathSet, maxPacketItems+1)}
	if err := decodeLimitedTrieNodesRequest(encodeMsg(t, over), new(GetTrieNodesPacket)); !errors.Is(err, errTooManyItems) {
		t.Fatalf("too many path sets: have %v, want %v", err, errTooManyItems)
	}
}
