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
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/p2p/enode"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
)

// Reported via Bugcrowd: response packets were fully decoded before checking
// they answered one of our requests, and nothing capped their item count. A
// 10MB NodeData packet of one-byte items decoded into ~800MB of allocations,
// and since dangling responses were dropped silently the peer could repeat it.

// rlpList wraps already encoded items into an RLP list.
func rlpList(items []byte) []byte {
	n := len(items)
	switch {
	case n < 56:
		return append([]byte{0xc0 + byte(n)}, items...)
	case n < 1<<8:
		return append([]byte{0xf8, byte(n)}, items...)
	case n < 1<<16:
		return append([]byte{0xf9, byte(n >> 8), byte(n)}, items...)
	default:
		return append([]byte{0xfa, byte(n >> 16), byte(n >> 8), byte(n)}, items...)
	}
}

// responseMsg builds an eth/66 response message around an encoded payload list.
func responseMsg(code uint64, id uint64, payload []byte) p2p.Msg {
	idEnc, _ := rlp.EncodeToBytes(id)
	body := rlpList(append(idEnc, payload...))
	return p2p.Msg{Code: code, Size: uint32(len(body)), Payload: bytes.NewReader(body)}
}

func newDispatchTestPeer(t *testing.T) (*Peer, *p2p.MsgPipeRW) {
	backend := newTestBackend(0)
	t.Cleanup(backend.close)

	app, net := p2p.MsgPipe()
	t.Cleanup(func() { app.Close() })

	var id enode.ID
	rand.Read(id[:])
	peer := NewPeer(ETH66, p2p.NewPeer(id, "test", nil), net, backend.TxPool())
	t.Cleanup(peer.Close)
	return peer, app
}

// A one-byte string in long form is non-canonical, so any attempt to decode
// this payload fails with errDecode.
var junkPayload = rlpList([]byte{0xb8, 0x01, 0x05})

// A response nobody asked for must drop the peer without decoding its payload.
func TestDanglingResponseNotDecoded(t *testing.T) {
	handlers := map[string]struct {
		code    uint64
		handler msgHandler
	}{
		"headers":  {BlockHeadersMsg, handleBlockHeaders66},
		"bodies":   {BlockBodiesMsg, handleBlockBodies66},
		"nodedata": {NodeDataMsg, handleNodeData66},
		"receipts": {ReceiptsMsg, handleReceipts66},
	}
	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			peer, _ := newDispatchTestPeer(t)
			err := h.handler(nil, responseMsg(h.code, 0xdeadbeef, junkPayload), peer)
			if !errors.Is(err, errDanglingResponse) {
				t.Fatalf("dangling response: have %v, want %v", err, errDanglingResponse)
			}
		})
	}
}

// A dangling response must cost about its own size, not a multiple of it.
func TestDanglingResponseAllocation(t *testing.T) {
	peer, _ := newDispatchTestPeer(t)

	items := bytes.Repeat([]byte{0x01}, maxMessageSize-64)
	msg := responseMsg(NodeDataMsg, 0xdeadbeef, rlpList(items))

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := handleNodeData66(nil, msg, peer); !errors.Is(err, errDanglingResponse) {
		t.Fatalf("dangling response: have %v, want %v", err, errDanglingResponse)
	}
	runtime.ReadMemStats(&after)

	// Copying the undecoded payload out of the message is the only big cost.
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 3*maxMessageSize {
		t.Fatalf("dangling response allocated %d bytes, want <= %d", alloc, 3*maxMessageSize)
	}
}

// requestNodeData sends a NodeData request for two hashes from peer and returns
// it along with its request id as seen on the wire.
func requestNodeData(t *testing.T, peer *Peer, app *p2p.MsgPipeRW, sink chan *Response) (*Request, uint64) {
	type result struct {
		req *Request
		err error
	}
	resc := make(chan result, 1)
	go func() {
		req, err := peer.RequestNodeData([]common.Hash{{1}, {2}}, sink)
		resc <- result{req, err}
	}()
	msg, err := app.ReadMsg()
	if err != nil {
		t.Fatalf("failed to read request: %v", err)
	}
	var packet GetNodeDataPacket66
	if err := msg.Decode(&packet); err != nil {
		t.Fatalf("failed to decode request: %v", err)
	}
	res := <-resc
	if res.err != nil {
		t.Fatalf("failed to send request: %v", res.err)
	}
	return res.req, packet.RequestId
}

// A late reply to a request we cancelled is not the peer's fault, so it is
// discarded without decoding it or dropping the peer. Only once though.
func TestStaleResponseDiscarded(t *testing.T) {
	peer, app := newDispatchTestPeer(t)
	req, id := requestNodeData(t, peer, app, make(chan *Response))
	if err := req.Close(); err != nil {
		t.Fatalf("failed to cancel request: %v", err)
	}
	if err := handleNodeData66(nil, responseMsg(NodeDataMsg, id, junkPayload), peer); err != nil {
		t.Fatalf("stale response: have %v, want nil", err)
	}
	if err := handleNodeData66(nil, responseMsg(NodeDataMsg, id, junkPayload), peer); !errors.Is(err, errDanglingResponse) {
		t.Fatalf("repeated stale response: have %v, want %v", err, errDanglingResponse)
	}
}

// A response of the wrong type to a pending request drops the peer.
func TestMismatchedResponseType(t *testing.T) {
	peer, app := newDispatchTestPeer(t)
	_, id := requestNodeData(t, peer, app, make(chan *Response))
	if err := handleReceipts66(nil, responseMsg(ReceiptsMsg, id, junkPayload), peer); !errors.Is(err, errMismatchingResponseType) {
		t.Fatalf("mismatched response: have %v, want %v", err, errMismatchingResponseType)
	}
}

// Responses are checked against the size of the request they answer.
func TestResponseItemLimit(t *testing.T) {
	request := func(t *testing.T) (*Peer, chan *Response, uint64) {
		peer, app := newDispatchTestPeer(t)
		sink := make(chan *Response, 1)
		_, id := requestNodeData(t, peer, app, sink)
		return peer, sink, id
	}

	t.Run("over", func(t *testing.T) {
		peer, _, id := request(t)
		err := handleNodeData66(nil, responseMsg(NodeDataMsg, id, rlpList([]byte{0x01, 0x02, 0x03})), peer)
		if !errors.Is(err, errDecode) {
			t.Fatalf("oversized response: have %v, want %v", err, errDecode)
		}
	})
	t.Run("within", func(t *testing.T) {
		peer, sink, id := request(t)
		go func() {
			res := <-sink
			if data := *res.Res.(*NodeDataPacket); len(data) != 2 || data[0][0] != 0x01 || data[1][0] != 0x02 {
				res.Done <- errors.New("wrong data delivered")
				return
			}
			res.Done <- nil
		}()
		if err := handleNodeData66(nil, responseMsg(NodeDataMsg, id, rlpList([]byte{0x01, 0x02})), peer); err != nil {
			t.Fatalf("valid response failed: %v", err)
		}
	})
}

type txAcceptingBackend struct {
	*testBackend
	handled bool
}

func (b *txAcceptingBackend) AcceptTxs() bool            { return true }
func (b *txAcceptingBackend) Handle(*Peer, Packet) error { b.handled = true; return nil }

// Transaction broadcasts are capped in item count before being decoded.
func TestTransactionsItemLimit(t *testing.T) {
	peer, _ := newDispatchTestPeer(t)
	backend := &txAcceptingBackend{testBackend: newTestBackend(0)}
	defer backend.close()

	// Smallest legacy transaction that decodes: nine empty fields.
	tx := []byte{0xc9, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	body := rlpList(bytes.Repeat(tx, maxTransactionsPerPacket+1))
	msg := p2p.Msg{Code: TransactionsMsg, Size: uint32(len(body)), Payload: bytes.NewReader(body)}

	err := handleTransactions(backend, msg, peer)
	if !errors.Is(err, errDecode) || !strings.Contains(err.Error(), errTooManyItems.Error()) {
		t.Fatalf("oversized broadcast: have %v, want %v", err, errTooManyItems)
	}
	if backend.handled {
		t.Fatal("oversized broadcast reached the backend")
	}
}

// Block bodies are hashed straight from their encoding, which must agree with
// hashing the decoded transactions and uncles exactly.
func TestRawBlockBodyHashes(t *testing.T) {
	key, _ := crypto.GenerateKey()
	signer := types.LatestSignerForChainID(big.NewInt(1))

	// Enough of a mix of transaction types to cross the 0x80 index boundary
	// where DeriveSha changes insertion order.
	var txs []*types.Transaction
	for i := 0; i < 300; i++ {
		var inner types.TxData
		switch i % 3 {
		case 0:
			inner = &types.LegacyTx{Nonce: uint64(i), GasPrice: big.NewInt(1), Gas: 21000, To: &common.Address{1}, Value: big.NewInt(int64(i))}
		case 1:
			inner = &types.AccessListTx{ChainID: big.NewInt(1), Nonce: uint64(i), GasPrice: big.NewInt(1), Gas: 21000, To: &common.Address{2}}
		case 2:
			inner = &types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: uint64(i), GasTipCap: big.NewInt(1), GasFeeCap: big.NewInt(2), Gas: 21000, Data: []byte{byte(i)}}
		}
		tx, err := types.SignNewTx(key, signer, inner)
		if err != nil {
			t.Fatal(err)
		}
		txs = append(txs, tx)
	}
	uncles := []*types.Header{{Number: big.NewInt(1), Difficulty: big.NewInt(1)}, {Number: big.NewInt(2), Difficulty: big.NewInt(2)}}

	for _, n := range []int{0, 1, 127, 128, 129, len(txs)} {
		for _, u := range [][]*types.Header{nil, uncles} {
			enc, err := rlp.EncodeToBytes([]*BlockBody{{Transactions: txs[:n], Uncles: u}})
			if err != nil {
				t.Fatal(err)
			}
			peer, app := newDispatchTestPeer(t)
			sink := make(chan *Response, 1)
			go func() {
				peer.RequestBodies([]common.Hash{{1}}, sink)
			}()
			msg, err := app.ReadMsg()
			if err != nil {
				t.Fatal(err)
			}
			var req GetBlockBodiesPacket66
			msg.Decode(&req)

			done := make(chan error, 1)
			go func() {
				res := <-sink
				hashes := res.Meta.([][]common.Hash)
				if want := types.DeriveSha(types.Transactions(txs[:n]), trie.NewStackTrie(nil)); hashes[0][0] != want {
					done <- fmt.Errorf("%d txs, %d uncles: tx root %x, want %x", n, len(u), hashes[0][0], want)
				} else if want := types.CalcUncleHash(u); hashes[1][0] != want {
					done <- fmt.Errorf("%d txs, %d uncles: uncle hash %x, want %x", n, len(u), hashes[1][0], want)
				} else if dtxs, duncles, err := (*res.Res.(*RawBlockBodiesPacket))[0].Decode(); err != nil || len(dtxs) != n || len(duncles) != len(u) {
					done <- fmt.Errorf("%d txs, %d uncles: decoded %d txs, %d uncles, err %v", n, len(u), len(dtxs), len(duncles), err)
				} else {
					done <- nil
				}
				res.Done <- nil
			}()
			if err := handleBlockBodies66(nil, responseMsg(BlockBodiesMsg, req.RequestId, enc), peer); err != nil {
				t.Fatalf("%d txs, %d uncles: response failed: %v", n, len(u), err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
	}
}

// A requested body stuffed with tiny junk transactions is delivered still
// encoded and isn't even hashed, so it only costs about its own size.
func TestRawBlockBodyNotDecoded(t *testing.T) {
	peer, app := newDispatchTestPeer(t)
	sink := make(chan *Response, 1)
	go func() {
		peer.RequestBodies([]common.Hash{{1}}, sink)
	}()
	msg, err := app.ReadMsg()
	if err != nil {
		t.Fatal(err)
	}
	var req GetBlockBodiesPacket66
	msg.Decode(&req)

	// One body of ~10M one-byte "transactions" and no uncles.
	txs := rlpList(bytes.Repeat([]byte{0x01}, maxMessageSize-64))
	body := rlpList(append(txs, 0xc0))

	hashes := make(chan [][]common.Hash, 1)
	go func() {
		res := <-sink
		hashes <- res.Meta.([][]common.Hash)
		res.Done <- nil
	}()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	if err := handleBlockBodies66(nil, responseMsg(BlockBodiesMsg, req.RequestId, rlpList(body)), peer); err != nil {
		t.Fatalf("response failed: %v", err)
	}
	runtime.ReadMemStats(&after)

	// The envelope and body split each copy the payload, but nothing scales
	// with the number of items. Decoding these would take ~800MB.
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 6*maxMessageSize {
		t.Fatalf("junk body allocated %d bytes, want <= %d", alloc, 6*maxMessageSize)
	}
	if root := (<-hashes)[0][0]; root != (common.Hash{}) {
		t.Fatalf("junk body hashed to %x, want empty hash", root)
	}
}
