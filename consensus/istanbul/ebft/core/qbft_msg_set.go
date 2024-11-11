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

package core

import (
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	ebfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/ebft/types"
	"github.com/electroneum/electroneum-sc/rlp"
)

// Construct a new message set to accumulate messages for given sequence/view number.
func newEBFTMsgSet(valSet istanbul.ValidatorSet) *ebftMsgSet {
	return &ebftMsgSet{
		view: &istanbul.View{
			Round:    new(big.Int),
			Sequence: new(big.Int),
		},
		messagesMu: new(sync.Mutex),
		messages:   make(map[common.Address]ebfttypes.EBFTMessage),
		valSet:     valSet,
	}
}

// ----------------------------------------------------------------------------

type ebftMsgSet struct {
	view       *istanbul.View
	valSet     istanbul.ValidatorSet
	messagesMu *sync.Mutex
	messages   map[common.Address]ebfttypes.EBFTMessage
}

// ebftMsgMapAsStruct is a temporary holder struct to convert messages map to a slice when Encoding and Decoding ebftMsgSet
type ebftMsgMapAsStruct struct {
	Address common.Address
	Msg     ebfttypes.EBFTMessage
}

func (ms *ebftMsgSet) View() *istanbul.View {
	return ms.view
}

func (ms *ebftMsgSet) Add(msg ebfttypes.EBFTMessage) error {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	ms.messages[msg.Source()] = msg
	return nil
}

func (ms *ebftMsgSet) Values() (result []ebfttypes.EBFTMessage) {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	for _, v := range ms.messages {
		result = append(result, v)
	}

	return result
}

func (ms *ebftMsgSet) Size() int {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return len(ms.messages)
}

func (ms *ebftMsgSet) Get(addr common.Address) ebfttypes.EBFTMessage {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return ms.messages[addr]
}

// ----------------------------------------------------------------------------

func (ms *ebftMsgSet) String() string {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	addresses := make([]string, 0, len(ms.messages))
	for _, v := range ms.messages {
		addresses = append(addresses, v.Source().String())
	}
	return fmt.Sprintf("[%v]", strings.Join(addresses, ", "))
}

// EncodeRLP serializes ebftMsgSet into Ethereum RLP format
// valSet is currently not being encoded.
func (ms *ebftMsgSet) EncodeRLP(w io.Writer) error {
	if ms == nil {
		return nil
	}
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	// maps cannot be RLP encoded, convert the map into a slice of struct and then encode it
	var messagesAsSlice []ebftMsgMapAsStruct
	for k, v := range ms.messages {
		msgMapAsStruct := ebftMsgMapAsStruct{
			Address: k,
			Msg:     v,
		}
		messagesAsSlice = append(messagesAsSlice, msgMapAsStruct)
	}

	return rlp.Encode(w, []interface{}{
		ms.view,
		messagesAsSlice,
	})
}

// DecodeRLP deserializes rlp stream into ebftMsgSet
// valSet is currently not being decoded
func (ms *ebftMsgSet) DecodeRLP(stream *rlp.Stream) error {
	// Don't decode ebftMsgSet if the size of the stream is 0
	_, size, _ := stream.Kind()
	if size == 0 {
		return nil
	}
	var msgSet struct {
		MsgView *istanbul.View
		//		valSet        istanbul.ValidatorSet
		MessagesSlice []ebftMsgMapAsStruct
	}
	if err := stream.Decode(&msgSet); err != nil {
		return err
	}

	// convert the messages struct slice back to map
	messages := make(map[common.Address]ebfttypes.EBFTMessage)
	for _, msgStruct := range msgSet.MessagesSlice {
		messages[msgStruct.Address] = msgStruct.Msg
	}

	ms.view = msgSet.MsgView
	//	ms.valSet = msgSet.valSet
	ms.messages = messages
	ms.messagesMu = new(sync.Mutex)

	return nil
}
