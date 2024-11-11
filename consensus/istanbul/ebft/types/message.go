package ebfttypes

import (
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
)

// EBFT message codes
const (
	PreprepareCode  = 0x12
	PrepareCode     = 0x13
	CommitCode      = 0x14
	RoundChangeCode = 0x15
)

// A set containing the messages codes for all EBFT messages.
func MessageCodes() map[uint64]struct{} {
	return map[uint64]struct{}{
		PreprepareCode:  {},
		PrepareCode:     {},
		CommitCode:      {},
		RoundChangeCode: {},
	}
}

// Common interface for all EBFT messages
type EBFTMessage interface {
	Code() uint64
	View() istanbul.View
	Source() common.Address
	SetSource(address common.Address)
	EncodePayloadForSigning() ([]byte, error)
	Signature() []byte
	SetSignature(signature []byte)
}
