package eth

import (
	"errors"

	"github.com/ethereum/go-ethereum/p2p"
)

// ibft_protocol enables the eth service to return two different protocols, one for the eth mainnet "eth" service,
// and one for the ibft specific consensus algo, obtained from engine.consensus
// 2021 Jan in the future consensus (istanbul) may run from its own service and use a single subprotocol there,
// instead of overloading the eth service.

var (
	// errEthPeerNil is returned when no eth peer is found to be associated with a p2p peer.
	errEthPeerNil           = errors.New("eth peer was nil")
	errEthPeerNotRegistered = errors.New("eth peer was not registered")
)

// ibft consensus Protocol variables are optionally set in addition to the "eth" protocol variables (eth/protocol.go).
var ibftConsensusProtocolName = ""

// ProtocolVersions are the supported versions of the ibft consensus protocol (first is primary), e.g. []uint{Istanbul64, Istanbul99, Istanbul100}.
var ibftConsensusProtocolVersions []uint

// protocol Length describe the number of messages support by the protocol/version map[uint]uint64{Istanbul64: 18, Istanbul99: 18, Istanbul100: 18}
var ibftConsensusProtocolLengths map[uint]uint64

func (s *Ethereum) ibftConsensusProtocols() []p2p.Protocol {
	protos := make([]p2p.Protocol, len(ibftConsensusProtocolVersions))
	for i, vsn := range ibftConsensusProtocolVersions {
		// if we have a legacy protocol, e.g. istanbul/99, istanbul/64 then the protocol handler is will be the "eth"
		// protocol handler, and the subprotocol "eth" will not be used, but rather the legacy subprotocol will handle
		// both eth messages and consensus messages.
		if isLegacyProtocol(ibftConsensusProtocolName, vsn) {
			length, ok := ibftConsensusProtocolLengths[vsn]
			if !ok {
				panic("makeProtocol for unknown version")
			}
			lp := s.handler.makeLegacyProtocol(ibftConsensusProtocolName, vsn, length)
			protos[i] = lp
		} else {
			length, ok := ibftConsensusProtocolLengths[vsn]
			if !ok {
				panic("makeIbftConsensusProtocol for unknown version")
			}
			protos[i] = s.handler.makeIbftConsensusProtocol(ibftConsensusProtocolName, vsn, length)
		}
	}
	return protos
}

// istanbul/64, istanbul/99, clique/63, clique/64 all override the "eth" subprotocol.
func isLegacyProtocol(name string, version uint) bool {
	// protocols that override "eth" subprotocol and run only the ibft subprotocol.
	ibftLegacyProtocols := map[string][]uint{"istanbul": {64, 99}, "clique": {63, 64}}
	for lpName, lpVersions := range ibftLegacyProtocols {
		if lpName == name {
			for _, v := range lpVersions {
				if v == version {
					return true
				}
			}
		}
	}
	return false
}
