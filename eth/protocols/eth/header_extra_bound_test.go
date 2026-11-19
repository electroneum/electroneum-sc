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

package eth

import (
	"bytes"
	"errors"
	"math/big"
	"runtime"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/rlp"
)

// Reported via Bugcrowd: hashing an Istanbul header decodes its extra-data,
// and both the header response handler and the NewBlock handler hashed a
// peer's header before checking its size. A 10MB extra-data of empty
// committed seals took over 1.6GB per hash, enough to OOM a validator with a
// handful of connections. Headers are now bounded before they're hashed, and
// a peer sending one that isn't gets dropped.

// stuffedIstanbulExtra is a legacy Istanbul extra-data whose committed seals
// are n empty strings, like the reported header.
func stuffedIstanbulExtra(n int) []byte {
	return append(make([]byte, types.IstanbulExtraVanity), rlpList(append([]byte{0xc0, 0x80}, rlpList(bytes.Repeat([]byte{0x80}, n))...))...)
}

func stuffedIstanbulHeader(n int) *types.Header {
	return &types.Header{
		Number:     big.NewInt(1),
		Difficulty: big.NewInt(1),
		GasLimit:   30_000_000,
		MixDigest:  types.IstanbulDigest,
		Extra:      stuffedIstanbulExtra(n),
	}
}

// requestOneHeader sends a single header request from peer and returns its
// request id as seen on the wire.
func requestOneHeader(t *testing.T, peer *Peer, app *p2p.MsgPipeRW, sink chan *Response) uint64 {
	errc := make(chan error, 1)
	go func() {
		_, err := peer.RequestOneHeader(common.Hash{1}, sink)
		errc <- err
	}()
	msg, err := app.ReadMsg()
	if err != nil {
		t.Fatalf("failed to read request: %v", err)
	}
	var packet GetBlockHeadersPacket66
	if err := msg.Decode(&packet); err != nil {
		t.Fatalf("failed to decode request: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	return packet.RequestId
}

func memAllocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func TestBlockHeadersExtraBound(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		peer, app := newDispatchTestPeer(t)
		sink := make(chan *Response, 1)
		id := requestOneHeader(t, peer, app, sink)

		header := stuffedIstanbulHeader(maxMessageSize - 1024)
		enc, err := rlp.EncodeToBytes(header)
		if err != nil {
			t.Fatal(err)
		}
		msg := responseMsg(BlockHeadersMsg, id, rlpList(enc))

		// Answer a delivery rather than leave the handler blocked on it, so a
		// regression fails instead of hanging.
		delivered := make(chan struct{})
		go func() {
			if res, ok := <-sink; ok {
				close(delivered)
				res.Done <- errors.New("oversized header was delivered")
			}
		}()
		var herr error
		used := memAllocated(func() { herr = handleBlockHeaders66(nil, msg, peer) })
		select {
		case <-delivered:
			t.Fatal("oversized header was delivered")
		default:
		}
		if !errors.Is(herr, errDecode) {
			t.Fatalf("oversized header: have %v, want %v", herr, errDecode)
		}
		if alloc := uint64(3 * maxMessageSize); used > alloc {
			t.Fatalf("oversized header allocated %d bytes, want <= %d", used, alloc)
		}
	})
	t.Run("within", func(t *testing.T) {
		peer, app := newDispatchTestPeer(t)
		sink := make(chan *Response, 1)
		id := requestOneHeader(t, peer, app, sink)

		header := stuffedIstanbulHeader(1000)
		enc, err := rlp.EncodeToBytes(header)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			res := <-sink
			hashes := res.Meta.([]common.Hash)
			if len(hashes) != 1 || hashes[0] != header.Hash() {
				res.Done <- errors.New("wrong header hash delivered")
				return
			}
			res.Done <- nil
		}()
		if err := handleBlockHeaders66(nil, responseMsg(BlockHeadersMsg, id, rlpList(enc)), peer); err != nil {
			t.Fatalf("header within bounds: %v", err)
		}
	})
}

func TestNewBlockExtraBound(t *testing.T) {
	peer, _ := newDispatchTestPeer(t)
	backend := &txAcceptingBackend{testBackend: newTestBackend(1)}
	defer backend.close()

	// The reported message: an oversized gas limit sends it down the path
	// that logs the header's hash and returns without dropping the peer.
	header := stuffedIstanbulHeader(maxMessageSize - 1024)
	header.GasLimit = 1 << 40
	msg := newBlockMsg(t, header, rlpList(nil))

	var herr error
	used := memAllocated(func() { herr = handleNewBlock(backend, msg, peer) })
	if herr == nil {
		t.Fatal("oversized header didn't drop the peer")
	}
	if alloc := uint64(3 * maxMessageSize); used > alloc {
		t.Fatalf("oversized header allocated %d bytes, want <= %d", used, alloc)
	}
	if backend.handled {
		t.Fatal("oversized header reached the backend")
	}
}
