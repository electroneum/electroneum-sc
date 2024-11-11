// Quorum
package consensus

import (
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/types"
)

// Constants to match up protocol versions and messages
// istanbul/99 was added to accommodate new eth/64 handshake status data with fork id
// this is for backward compatibility which allows a mixed old/new istanbul node network
// istanbul/64 will continue using old status data as eth/63
const (
	eth66       = 66
	Istanbul100 = 100
)

var (
	IstanbulProtocol = Protocol{
		Name:     "etn-istanbul",
		Versions: []uint{Istanbul100},
		// istanbul/100 has to have 22 message to be backwards compatible although at the p2p layer it only has
		// 1 message with msg.Code 17
		Lengths: map[uint]uint64{Istanbul100: 22},
	}

	CliqueProtocol = Protocol{
		Name:     "etn",
		Versions: []uint{eth66},
		Lengths:  map[uint]uint64{eth66: 17},
	}

	// Default: Keep up-to-date with eth/protocol.go
	EthProtocol = Protocol{
		Name:     "etn",
		Versions: []uint{eth66},
		Lengths:  map[uint]uint64{eth66: 17},
	}
)

// Protocol defines the protocol of the consensus
type Protocol struct {
	// Official short name of the protocol used during capability negotiation.
	Name string
	// Supported versions of the eth protocol (first is primary).
	Versions []uint
	// Number of implemented message corresponding to different protocol versions.
	Lengths map[uint]uint64
}

// Broadcaster defines the interface to enqueue blocks to fetcher and find peer
type Broadcaster interface {
	// Enqueue add a block into fetcher queue
	Enqueue(id string, block *types.Block)
	// FindPeers retrives peers by addresses
	FindPeers(map[common.Address]bool) map[common.Address]Peer
}

// Peer defines the interface to communicate with peer
type Peer interface {
	// Send sends the message to this peer
	Send(msgcode uint64, data interface{}) error

	// SendConsensus sends the message to this p2p peer using the consensus specific devp2p subprotocol
	SendConsensus(msgcode uint64, data interface{}) error

	// SendIBFTConsensus is used to send consensus subprotocol messages from an "eth" peer without encoding the payload
	SendIBFTConsensus(msgcode uint64, payload []byte) error
}
