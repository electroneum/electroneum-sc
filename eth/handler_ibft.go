// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package eth

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/clique"
	"github.com/electroneum/electroneum-sc/consensus/ethash"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/p2p"
	"github.com/electroneum/electroneum-sc/p2p/enode"
	"github.com/electroneum/electroneum-sc/params"
)

// The eth protocol's own limits live in eth/protocols/eth and are unexported,
// so the consensus subprotocol carries its own copies.
const protocolMaxMsgSize = 10 * 1024 * 1024

var errMsgTooLarge = errors.New("message too long")

// ibftNodeInfo is what admin_nodeInfo reports for the consensus subprotocol.
// Upstream's eth.NodeInfo has no Consensus field and moved to
// eth/protocols/eth, so this keeps the ETN-visible shape without touching it.
type ibftNodeInfo struct {
	Network    uint64              `json:"network"`
	Difficulty *big.Int            `json:"difficulty"`
	Genesis    common.Hash         `json:"genesis"`
	Config     *params.ChainConfig `json:"config"`
	Head       common.Hash         `json:"head"`
	Consensus  string              `json:"consensus"`
}

// NodeInfo describes the consensus subprotocol's view of this node.
//
// CurrentBlock returns a *types.Header upstream now, so this reads the head
// number and hash from the header directly rather than from a block.
func (h *handler) NodeInfo() *ibftNodeInfo {
	head := h.chain.CurrentBlock()
	return &ibftNodeInfo{
		Network:    h.networkID,
		Difficulty: h.chain.GetTd(head.Hash(), head.Number.Uint64()),
		Genesis:    h.chain.Genesis().Hash(),
		Config:     h.chain.Config(),
		Head:       head.Hash(),
		Consensus:  h.getConsensusAlgorithm(),
	}
}

// The IBFT consensus subprotocol ("etn-istanbul/100"). Electroneum runs QBFT
// messages over their own p2p protocol alongside the eth protocol, so the
// handler has to serve both.
//
// Kept out of handler.go so the ETN delta against upstream stays isolated.

func (h *handler) Enqueue(id string, block *types.Block) {
	h.blockFetcher.Enqueue(id, block)
}

func (h *handler) getConsensusAlgorithm() string {
	var consensusAlgo string
	switch h.engine.(type) {
	case consensus.Istanbul:
		consensusAlgo = "IBFT"
	case *clique.Clique:
		consensusAlgo = "clique"
	case *ethash.Ethash:
		consensusAlgo = "ethash"
	default:
		consensusAlgo = "unknown"
	}
	return consensusAlgo
}

func (h *handler) FindPeers(targets map[common.Address]bool) map[common.Address]consensus.Peer {
	m := make(map[common.Address]consensus.Peer)
	h.peers.lock.RLock()
	defer h.peers.lock.RUnlock()
	for _, p := range h.peers.peers {
		pubKey := p.Node().Pubkey()
		addr := crypto.PubkeyToAddress(*pubKey)
		if targets[addr] {
			m[addr] = p
		}
	}
	return m
}

// makeIbftConsensusProtocol is similar to eth/handler.go -> makeProtocol. Called from eth/handler.go -> Protocols.
// returns the supported subprotocol to the p2p server.
// The Run method starts the protocol and is called by the p2p server. The ibft consensus subprotocol,
// leverages the peer created and managed by the "eth" subprotocol.
// The ibft consensus protocol requires that the "eth" protocol is running as well.
func (h *handler) makeIbftConsensusProtocol(ProtoName string, version uint, length uint64) p2p.Protocol {
	return p2p.Protocol{
		Name:    ProtoName,
		Version: version,
		Length:  length,
		// no new peer created, uses the "eth" peer, so no peer management needed.
		Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
			/*
			* 1. wait for the eth protocol to create and register an eth peer.
			* 2. get the associate eth peer that was registered by he "eth" protocol.
			* 3. add the rw protocol for the ibft subprotocol to the eth peer.
			* 4. start listening for incoming messages.
			* 5. the incoming message will be sent on the ibft specific subprotocol, e.g. "istanbul/100".
			* 6. send messages to the consensus engine handler.
			* 7. messages to other to other peers listening to the subprotocol can be sent using the
			*    (eth)peer.ConsensusSend() which will write to the protoRW.
			 */
			// wait for the "eth" protocol to create and register the peer (added to peerset)
			select {
			case <-p.EthPeerRegistered:
				// the ethpeer should be registered, try to retrieve it and start the consensus handler.
				p2pPeerId := fmt.Sprintf("%x", p.ID().Bytes()[:8])
				ethPeer := h.peers.peer(p2pPeerId)
				if ethPeer == nil {
					p2pPeerId = fmt.Sprintf("%x", p.ID().Bytes()) // TODO:BBO
					ethPeer = h.peers.peer(p2pPeerId)
					log.Warn("full p2p peer", "id", p2pPeerId, "etnPeer", ethPeer)
				}
				if ethPeer != nil {
					p.Log().Debug("consensus subprotocol retrieved eth peer from peerset", "etnPeer.id", p2pPeerId, "ProtoName", ProtoName)
					// add the rw protocol for the ibft subprotocol to the eth peer.
					ethPeer.AddConsensusProtoRW(rw)
					return h.handleConsensusLoop(p, rw)
				}
				p.Log().Error("consensus subprotocol retrieved nil eth peer from peerset", "etnPeer.id", p2pPeerId)
				return errEthPeerNil
			case <-p.EthPeerDisconnected:
				return errEthPeerNotRegistered
			}
		},
		NodeInfo: func() interface{} {
			return h.NodeInfo()
		},
		PeerInfo: func(id enode.ID) interface{} {
			if p := h.peers.peer(fmt.Sprintf("%x", id[:8])); p != nil {
				return p.Info()
			}
			if p := h.peers.peer(fmt.Sprintf("%x", id)); p != nil { // TODO:BBO
				return p.Info()
			}
			return nil
		},
	}
}

func (h *handler) handleConsensusLoop(p *p2p.Peer, protoRW p2p.MsgReadWriter) error {
	// Handle incoming messages until the connection is torn down
	for {
		if err := h.handleConsensus(p, protoRW); err != nil {
			if errors.Is(err, istanbul.ErrStoppedEngine) && h.downloader.Synchronising() {
				p.Log().Debug("Ignoring `stopped engine` consensus error due to active sync.")
				continue
			}
			p.Log().Debug("Ethereum ibft message handling failed", "err", err)
			return err
		}
	}
}

// This is a no-op because the eth handleMsg main loop handle ibf message as well.
func (h *handler) handleConsensus(p *p2p.Peer, protoRW p2p.MsgReadWriter) error {
	// Read the next message from the remote peer (in protoRW), and ensure it's fully consumed
	msg, err := protoRW.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > protocolMaxMsgSize {
		return fmt.Errorf("%w: %v > %v", errMsgTooLarge, msg.Size, protocolMaxMsgSize)
	}
	defer msg.Discard()

	// See if the consensus engine protocol can handle this message, e.g. istanbul will check for message is
	// istanbulMsg = 0x11, and NewBlockMsg = 0x07.
	handled, err := h.handleConsensusMsg(p, msg)
	if handled {
		p.Log().Debug("consensus message was handled by consensus engine", "handled", handled,
			"ibftConsensusProtocolName", ibftConsensusProtocolName, "err", err)
		return err
	}

	return nil
}

func (h *handler) handleConsensusMsg(p *p2p.Peer, msg p2p.Msg) (bool, error) {
	if handler, ok := h.engine.(consensus.Handler); ok {
		pubKey := p.Node().Pubkey()
		addr := crypto.PubkeyToAddress(*pubKey)
		handled, err := handler.HandleMsg(addr, msg)
		return handled, err
	}
	return false, nil
}
