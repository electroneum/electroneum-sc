// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package consensus

import (
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/ethdb"
	"github.com/electroneum/electroneum-sc/p2p"
)

// Handler is implemented by a consensus engine that needs to exchange messages
// with peers over its own devp2p subprotocol.
type Handler interface {
	// NewChainHead handles a new head block
	NewChainHead() error

	// HandleMsg handles a message from a peer
	HandleMsg(address common.Address, data p2p.Msg) (bool, error)

	// SetBroadcaster sets the broadcaster used to send messages to peers
	SetBroadcaster(Broadcaster)
}

// Istanbul is a byzantine-fault-tolerant consensus engine.
//
// Start takes currentBlock as a func() *types.Block rather than a header:
// upstream changed BlockChain.CurrentBlock to return *types.Header after this
// interface was written, so eth/backend.go adapts at the call site instead of
// changing the engine's contract.
type Istanbul interface {
	Engine

	// Start starts the engine
	Start(chain ChainHeaderReader, currentBlock func() *types.Block, hasBadBlock func(db ethdb.Reader, hash common.Hash) bool) error

	// Stop stops the engine
	Stop() error
}
