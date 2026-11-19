// Copyright 2015 The go-ethereum Authors
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
	"bytes"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
)

func tryUntilMessageIsHandled(backend *Backend, arbitraryAddress common.Address, arbitraryP2PMessage p2p.Msg) (handled bool, err error) {
	for i := 0; i < 5; i++ { // make 5 tries if a little wait
		handled, err = backend.HandleMsg(arbitraryAddress, arbitraryP2PMessage)
		if handled && err == nil {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	return
}

func TestHandleNewBlockMessage_whenTypical(t *testing.T) {
	_, backend := newBlockChain(1)
	defer backend.Stop()
	arbitraryAddress := common.StringToAddress("arbitrary")
	arbitraryBlock, arbitraryP2PMessage := buildArbitraryP2PNewBlockMessage(t, false)
	postAndWait(backend, arbitraryBlock, t)

	handled, err := tryUntilMessageIsHandled(backend, arbitraryAddress, arbitraryP2PMessage)
	if err != nil {
		t.Errorf("expected message being handled successfully but got %s", err)
	}
	if !handled {
		t.Errorf("expected message being handled but not")
	}
	if _, err := io.ReadAll(arbitraryP2PMessage.Payload); err != nil {
		t.Errorf("expected p2p message payload is restored")
	}
}

func TestHandleNewBlockMessage_whenNotAProposedBlock(t *testing.T) {
	_, backend := newBlockChain(1)
	defer backend.Stop()
	arbitraryAddress := common.StringToAddress("arbitrary")
	_, arbitraryP2PMessage := buildArbitraryP2PNewBlockMessage(t, false)
	postAndWait(backend, types.NewBlock(&types.Header{
		Number:    big.NewInt(1),
		Root:      common.StringToHash("someroot"),
		GasLimit:  1,
		MixDigest: types.IstanbulDigest,
	}, nil, nil, nil, new(trie.Trie)), t)

	handled, err := tryUntilMessageIsHandled(backend, arbitraryAddress, arbitraryP2PMessage)
	if err != nil {
		t.Errorf("expected message being handled successfully but got %s", err)
	}
	if handled {
		t.Errorf("expected message not being handled")
	}
	if _, err := io.ReadAll(arbitraryP2PMessage.Payload); err != nil {
		t.Errorf("expected p2p message payload is restored")
	}
}

func TestHandleNewBlockMessage_whenFailToDecode(t *testing.T) {
	_, backend := newBlockChain(1)
	defer backend.Stop()
	arbitraryAddress := common.StringToAddress("arbitrary")
	_, arbitraryP2PMessage := buildArbitraryP2PNewBlockMessage(t, true)
	postAndWait(backend, types.NewBlock(&types.Header{
		Number:    big.NewInt(1),
		GasLimit:  1,
		MixDigest: types.IstanbulDigest,
	}, nil, nil, nil, new(trie.Trie)), t)

	handled, err := tryUntilMessageIsHandled(backend, arbitraryAddress, arbitraryP2PMessage)
	if err != nil {
		t.Errorf("expected message being handled successfully but got %s", err)
	}
	if handled {
		t.Errorf("expected message not being handled")
	}
	if _, err := io.ReadAll(arbitraryP2PMessage.Payload); err != nil {
		t.Errorf("expected p2p message payload is restored")
	}
}

// The proposer only needs the header to recognise its own block, so the body
// must not be decoded: a peer could otherwise stuff it with millions of tiny
// transactions that decode to many times their size.
func TestHandleNewBlockMessage_bodyNotDecoded(t *testing.T) {
	_, backend := newBlockChain(1)
	defer backend.Stop()
	arbitraryAddress := common.StringToAddress("arbitrary")
	arbitraryBlock, _ := buildArbitraryP2PNewBlockMessage(t, false)
	postAndWait(backend, arbitraryBlock, t)

	// Transactions that fail to decode, so decoding the body would fail too
	junkTxs := append([]rlp.RawValue{}, []byte{0x01}, []byte{0x02})
	request := []interface{}{
		[]interface{}{arbitraryBlock.Header(), junkTxs, []*types.Header{}},
		big.NewInt(1),
	}
	payload, err := rlp.EncodeToBytes(request)
	if err != nil {
		t.Fatalf("can't encode due to %s", err)
	}
	msg := p2p.Msg{Code: 0x07, Size: uint32(len(payload)), Payload: bytes.NewReader(payload)}

	handled, err := tryUntilMessageIsHandled(backend, arbitraryAddress, msg)
	if err != nil {
		t.Errorf("expected message being handled successfully but got %s", err)
	}
	if !handled {
		t.Errorf("expected proposed block to be recognised from its header")
	}
}

func postAndWait(backend *Backend, block *types.Block, t *testing.T) {
	eventSub := backend.EventMux().Subscribe(istanbul.RequestEvent{})
	defer eventSub.Unsubscribe()
	stop := make(chan struct{}, 1)
	eventLoop := func() {
		<-eventSub.Chan()
		stop <- struct{}{}
	}
	go eventLoop()
	if err := backend.EventMux().Post(istanbul.RequestEvent{
		Proposal: block,
	}); err != nil {
		t.Fatalf("%s", err)
	}
	<-stop
}

func buildArbitraryP2PNewBlockMessage(t *testing.T, invalidMsg bool) (*types.Block, p2p.Msg) {
	arbitraryBlock := types.NewBlock(&types.Header{
		Number:    big.NewInt(1),
		GasLimit:  0,
		MixDigest: types.IstanbulDigest,
	}, nil, nil, nil, new(trie.Trie))
	request := []interface{}{&arbitraryBlock, big.NewInt(1)}
	if invalidMsg {
		request = []interface{}{"invalid msg"}
	}
	size, r, err := rlp.EncodeToReader(request)
	if err != nil {
		t.Fatalf("can't encode due to %s", err)
	}
	payload, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("can't read payload due to %s", err)
	}
	arbitraryP2PMessage := p2p.Msg{Code: 0x07, Size: uint32(size), Payload: bytes.NewReader(payload)}
	return arbitraryBlock, arbitraryP2PMessage
}

// paddedQBFTExtra is a QBFT extra-data with n empty committed seals. The seals
// aren't part of the header hash, so every n gives the same hash.
func paddedQBFTExtra(t *testing.T, n int) []byte {
	vanity, err := rlp.EncodeToBytes(make([]byte, types.IstanbulExtraVanity))
	if err != nil {
		t.Fatal(err)
	}
	seals := bytes.Repeat([]byte{0x80}, n)
	content := append(append(vanity, 0xc0, 0xc0, 0x80), listHeader(len(seals))...)
	content = append(content, seals...)
	return append(listHeader(len(content)), content...)
}

func listHeader(n int) []byte {
	switch {
	case n < 56:
		return []byte{0xc0 + byte(n)}
	case n < 1<<8:
		return []byte{0xf8, byte(n)}
	case n < 1<<16:
		return []byte{0xf9, byte(n >> 8), byte(n)}
	default:
		return []byte{0xfa, byte(n >> 16), byte(n >> 8), byte(n)}
	}
}

// Reported via Bugcrowd: hashing an Istanbul header decodes its extra-data,
// and the proposer hashed every NewBlock header it got while holding coreMu.
// A copy of its own proposal with millions of empty committed seals padded in
// still hashes the same. An oversized header is now left to the eth handler,
// which drops the peer, without being hashed here.
func TestHandleNewBlockMessage_oversizedHeaderNotHashed(t *testing.T) {
	_, backend := newBlockChain(1)
	defer backend.Stop()
	arbitraryAddress := common.StringToAddress("arbitrary")

	proposal := types.NewBlock(&types.Header{
		Number:    big.NewInt(1),
		GasLimit:  0,
		MixDigest: types.IstanbulDigest,
		Extra:     paddedQBFTExtra(t, 0),
	}, nil, nil, nil, new(trie.Trie))
	postAndWait(backend, proposal, t)

	padded := types.CopyHeader(proposal.Header())
	padded.Extra = paddedQBFTExtra(t, 1_000_000)
	if padded.Hash() != proposal.Hash() {
		t.Fatal("padding the committed seals changed the header hash")
	}
	payload, err := rlp.EncodeToBytes([]interface{}{
		[]interface{}{padded, []*types.Transaction{}, []*types.Header{}},
		big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	msg := p2p.Msg{Code: 0x07, Size: uint32(len(payload)), Payload: bytes.NewReader(payload)}

	handled, err := backend.HandleMsg(arbitraryAddress, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("oversized header was swallowed as our own proposal instead of reaching the eth handler")
	}
	if rest, err := io.ReadAll(msg.Payload); err != nil || !bytes.Equal(rest, payload) {
		t.Fatal("expected p2p message payload is restored")
	}
	// The same header without the padding is still recognised.
	enc, err := rlp.EncodeToBytes([]interface{}{proposal, big.NewInt(1)})
	if err != nil {
		t.Fatal(err)
	}
	small := p2p.Msg{Code: 0x07, Size: uint32(len(enc)), Payload: bytes.NewReader(enc)}
	if handled, err := tryUntilMessageIsHandled(backend, arbitraryAddress, small); err != nil || !handled {
		t.Fatalf("own proposal not recognised: handled %v, err %v", handled, err)
	}
}
