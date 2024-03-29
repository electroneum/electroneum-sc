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
