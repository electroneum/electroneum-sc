package ebfttypes

import (
	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
)

// EBFT message codes
const (
	PreprepareCode  = 0x16
	PrepareCode     = 0x17
	CommitCode      = 0x18
	RoundChangeCode = 0x19
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
