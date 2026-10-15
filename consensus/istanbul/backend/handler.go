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
	"bytes"
	"errors"
	"io"
	"math/big"
	"reflect"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/rlp"
	lru "github.com/hashicorp/golang-lru"
)

const (
	NewBlockMsg = 0x07
	istanbulMsg = 0x11
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")

	// errPayloadReadFailed is returned when qbft message read fails
	errPayloadReadFailed = errors.New("unable to read payload from message")
)

// Protocol implements consensus.Engine.Protocol
func (sb *Backend) Protocol() consensus.Protocol {
	return consensus.IstanbulProtocol
}

func (sb *Backend) decode(msg p2p.Msg) ([]byte, common.Hash, error) {
	var data []byte = make([]byte, msg.Size)
	if _, err := msg.Payload.Read(data); err != nil {
		return nil, common.Hash{}, errPayloadReadFailed
	}
	return data, istanbul.RLPHash(data), nil
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if _, ok := qbfttypes.MessageCodes()[msg.Code]; ok || msg.Code == istanbulMsg {
		if !sb.coreStarted {
			return true, istanbul.ErrStoppedEngine
		}

		data, hash, err := sb.decode(msg)
		if err != nil {
			return true, errDecodeFailed
		}
		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		// Mark self known message
		if _, ok := sb.knownMessages.Get(hash); ok {
			return true, nil
		}
		sb.knownMessages.Add(hash, true)

		// Deliver synchronously so the p2p read loop cannot outrun the single
		// istanbul event consumer. A detached `go Post` here lets a remote peer
		// enqueue consensus frames faster than core.handleEvents drains them,
		// accumulating unbounded blocked goroutines that each pin a full payload
		// in heap until the node OOMs. Posting inline applies backpressure: the
		// per-peer read loop blocks until the consumer accepts the event, which
		// bounds in-flight pre-validation payloads and propagates flow control
		// down to TCP. The consumer path (handleEvents -> Gossip/Broadcast) never
		// re-acquires coreMu and dispatches peer writes on their own goroutines,
		// so this cannot deadlock or self-block on the mux.
		sb.istanbulEventMux.Post(istanbul.MessageEvent{
			Code:    msg.Code,
			Payload: data,
		})
		return true, nil
	}
	//https://github.com/ConsenSys/quorum/pull/539
	//https://github.com/ConsenSys/quorum/issues/389
	if msg.Code == NewBlockMsg && sb.core != nil && sb.core.IsProposer() { // eth.NewBlockMsg: import cycle
		// this case is to safeguard the race of similar block which gets propagated from other node while this node is proposing
		// as p2p.Msg can only be decoded once (get EOF for any subsequence read), we need to make sure the payload is restored after we decode it
		sb.logger.Debug("IBFT: received NewBlockMsg", "size", msg.Size, "payload.type", reflect.TypeOf(msg.Payload), "sender", addr)
		if reader, ok := msg.Payload.(*bytes.Reader); ok {
			payload, err := io.ReadAll(reader)
			if err != nil {
				return true, err
			}
			reader.Reset(payload)       // ready to be decoded
			defer reader.Reset(payload) // restore so main eth/handler can decode
			// This has to match eth/protocol.go#NewBlockPacket as we are reading
			// NewBlockMsg. Only the header is needed for the hash, so leave the
			// body encoded: the eth handler checks it before decoding.
			var request struct {
				Block struct {
					Header *types.Header
					Body   []rlp.RawValue `rlp:"tail"`
				}
				TD *big.Int
			}
			if err := msg.Decode(&request); err != nil || request.Block.Header == nil {
				sb.logger.Error("IBFT: unable to decode the NewBlockMsg", "error", err)
				return false, nil
			}
			header := request.Block.Header
			if header.MixDigest == types.IstanbulDigest && sb.core.IsCurrentProposal(header.Hash()) {
				sb.logger.Debug("IBFT: block already proposed", "hash", header.Hash(), "sender", addr)
				return true, nil
			}
		}
	}
	return false, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

func (sb *Backend) NewChainHead() error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}
	go sb.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})
	return nil
}
