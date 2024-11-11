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

package types

import (
	"errors"
	"io"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/rlp"
)

var (
	// IstanbulDigest represents a hash of "Istanbul practical byzantine fault tolerance"
	// to identify whether the block is from Istanbul consensus engine
	IstanbulDigest = common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	IstanbulExtraVanity = 32 // Fixed number of extra-data bytes reserved for validator vanity
	IstanbulExtraSeal   = 65 // Fixed number of extra-data bytes reserved for validator seal

	IBFTAuthVote = byte(0xFF) // Magic number to vote on adding a new validator
	IBFTDropVote = byte(0x00) // Magic number to vote on removing a validator.

	// ErrInvalidIstanbulHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidIstanbulHeaderExtra = errors.New("invalid istanbul header extra-data")
)

// IstanbulExtra represents the legacy IBFT header extradata
type IstanbulExtra struct {
	Validators    []common.Address
	Seal          []byte
	CommittedSeal [][]byte
}

// EncodeRLP serializes ist into the Ethereum RLP format.
func (ist *IstanbulExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		ist.Validators,
		ist.Seal,
		ist.CommittedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the istanbul fields from a RLP stream.
func (ist *IstanbulExtra) DecodeRLP(s *rlp.Stream) error {
	var istanbulExtra struct {
		Validators    []common.Address
		Seal          []byte
		CommittedSeal [][]byte
	}
	if err := s.Decode(&istanbulExtra); err != nil {
		return err
	}
	ist.Validators, ist.Seal, ist.CommittedSeal = istanbulExtra.Validators, istanbulExtra.Seal, istanbulExtra.CommittedSeal
	return nil
}

// ExtractIstanbulExtra extracts all values of the IstanbulExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractIstanbulExtra(h *Header) (*IstanbulExtra, error) {
	if len(h.Extra) < IstanbulExtraVanity {
		return nil, ErrInvalidIstanbulHeaderExtra
	}

	var istanbulExtra *IstanbulExtra
	err := rlp.DecodeBytes(h.Extra[IstanbulExtraVanity:], &istanbulExtra)
	if err != nil {
		return nil, err
	}
	return istanbulExtra, nil
}

// FilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the Istanbul hash rules. It first check if the extradata can be extracted into IstanbulExtra if that fails,
// it extracts extradata into IBFTExtra struct
func FilteredHeader(h *Header) *Header {
	// Check if the header extradata can be decoded in IstanbulExtra, if yes, then call IstanbulFilteredHeader()
	// if not then call IBFTFilteredHeader()
	_, err := ExtractIstanbulExtra(h)
	if err != nil {
		return IBFTFilteredHeader(h)
	}
	return IstanbulFilteredHeader(h, true)
}

// IstanbulFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func IstanbulFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	istanbulExtra, err := ExtractIstanbulExtra(newHeader)
	if err != nil {
		return nil
	}

	if !keepSeal {
		istanbulExtra.Seal = []byte{}
	}
	istanbulExtra.CommittedSeal = [][]byte{}

	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return nil
	}

	newHeader.Extra = append(newHeader.Extra[:IstanbulExtraVanity], payload...)

	return newHeader
}

// IBFTExtra represents header extradata for ibft protocol
type IBFTExtra struct {
	VanityData    []byte
	Validators    []common.Address
	Vote          *ValidatorVote
	Round         uint32
	CommittedSeal [][]byte
}

type ValidatorVote struct {
	RecipientAddress common.Address
	VoteType         byte
}

// EncodeRLP serializes qist into the Ethereum RLP format.
func (qst *IBFTExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		qst.VanityData,
		qst.Validators,
		qst.Vote,
		qst.Round,
		qst.CommittedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the IBFTExtra fields from a RLP stream.
func (qst *IBFTExtra) DecodeRLP(s *rlp.Stream) error {
	var ibftExtra struct {
		VanityData    []byte
		Validators    []common.Address
		Vote          *ValidatorVote `rlp:"nil"`
		Round         uint32
		CommittedSeal [][]byte
	}
	if err := s.Decode(&ibftExtra); err != nil {
		return err
	}
	qst.VanityData, qst.Validators, qst.Vote, qst.Round, qst.CommittedSeal = ibftExtra.VanityData, ibftExtra.Validators, ibftExtra.Vote, ibftExtra.Round, ibftExtra.CommittedSeal

	return nil
}

// EncodeRLP serializes ValidatorVote into the Ethereum RLP format.
func (vv *ValidatorVote) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		vv.RecipientAddress,
		vv.VoteType,
	})
}

// DecodeRLP implements rlp.Decoder, and load the ValidatorVote fields from a RLP stream.
func (vv *ValidatorVote) DecodeRLP(s *rlp.Stream) error {
	var validatorVote struct {
		RecipientAddress common.Address
		VoteType         byte
	}
	if err := s.Decode(&validatorVote); err != nil {
		return err
	}
	vv.RecipientAddress, vv.VoteType = validatorVote.RecipientAddress, validatorVote.VoteType
	return nil
}

// ExtractIBFTExtra extracts all values of the IBFTExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractIBFTExtra(h *Header) (*IBFTExtra, error) {
	ibftExtra := new(IBFTExtra)
	err := rlp.DecodeBytes(h.Extra[:], ibftExtra)
	if err != nil {
		return nil, err
	}
	return ibftExtra, nil
}

// IBFTFilteredHeader returns a filtered header which some information (like committed seals, round, validator vote)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func IBFTFilteredHeader(h *Header) *Header {
	return IBFTFilteredHeaderWithRound(h, 0)
}

// IBFTFilteredHeaderWithRound returns the copy of the header with round number set to the given round number
// and commit seal set to its null value
func IBFTFilteredHeaderWithRound(h *Header, round uint32) *Header {
	newHeader := CopyHeader(h)
	ibftExtra, err := ExtractIBFTExtra(newHeader)
	if err != nil {
		return nil
	}

	ibftExtra.CommittedSeal = [][]byte{}
	ibftExtra.Round = round

	payload, err := rlp.EncodeToBytes(&ibftExtra)
	if err != nil {
		return nil
	}

	newHeader.Extra = payload

	return newHeader
}
