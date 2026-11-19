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

	QBFTAuthVote = byte(0xFF) // Magic number to vote on adding a new validator
	QBFTDropVote = byte(0x00) // Magic number to vote on removing a validator.

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
// it extracts extradata into QBFTExtra struct
func FilteredHeader(h *Header) *Header {
	// Check if the header extradata can be decoded in IstanbulExtra, if yes, then call IstanbulFilteredHeader()
	// if not then call QBFTFilteredHeader()
	_, err := extractIstanbulExtraForHash(h)
	if err != nil {
		return QBFTFilteredHeader(h)
	}
	return IstanbulFilteredHeader(h, true)
}

// IstanbulFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func IstanbulFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	istanbulExtra, err := extractIstanbulExtraForHash(newHeader)
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

// QBFTExtra represents header extradata for qbft protocol
type QBFTExtra struct {
	VanityData    []byte
	Validators    []common.Address
	Vote          *ValidatorVote
	Round         uint32
	CommittedSeal [][]byte
	ProposerSeal  []byte // Proposer's signature over the header hash (post-FutureFork only)
}

type ValidatorVote struct {
	RecipientAddress common.Address
	VoteType         byte
}

// EncodeRLP serializes qist into the Ethereum RLP format.
// ProposerSeal is only included as a 6th field when it contains a real seal.
// Pre-fork blocks (no seal) encode as 5 fields, preserving identical RLP output
// to the original format and keeping block hashes stable.
func (qst *QBFTExtra) EncodeRLP(w io.Writer) error {
	if len(qst.ProposerSeal) > 0 {
		return rlp.Encode(w, []interface{}{
			qst.VanityData,
			qst.Validators,
			qst.Vote,
			qst.Round,
			qst.CommittedSeal,
			qst.ProposerSeal,
		})
	}
	return rlp.Encode(w, []interface{}{
		qst.VanityData,
		qst.Validators,
		qst.Vote,
		qst.Round,
		qst.CommittedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the QBFTExtra fields from a RLP stream.
// Supports both pre-fork (5-field) and post-fork (6-field) encodings for backward
// compatibility with existing blocks on disk.
func (qst *QBFTExtra) DecodeRLP(s *rlp.Stream) error {
	// Try 6-field decode first (post-fork format with ProposerSeal)
	var qbftExtraV2 struct {
		VanityData    []byte
		Validators    []common.Address
		Vote          *ValidatorVote `rlp:"nil"`
		Round         uint32
		CommittedSeal [][]byte
		ProposerSeal  []byte
	}
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	if err := rlp.DecodeBytes(raw, &qbftExtraV2); err == nil {
		qst.VanityData = qbftExtraV2.VanityData
		qst.Validators = qbftExtraV2.Validators
		qst.Vote = qbftExtraV2.Vote
		qst.Round = qbftExtraV2.Round
		qst.CommittedSeal = qbftExtraV2.CommittedSeal
		qst.ProposerSeal = qbftExtraV2.ProposerSeal
		return nil
	}

	// Fall back to 5-field decode (pre-fork format without ProposerSeal)
	var qbftExtraV1 struct {
		VanityData    []byte
		Validators    []common.Address
		Vote          *ValidatorVote `rlp:"nil"`
		Round         uint32
		CommittedSeal [][]byte
	}
	if err := rlp.DecodeBytes(raw, &qbftExtraV1); err != nil {
		return err
	}
	qst.VanityData = qbftExtraV1.VanityData
	qst.Validators = qbftExtraV1.Validators
	qst.Vote = qbftExtraV1.Vote
	qst.Round = qbftExtraV1.Round
	qst.CommittedSeal = qbftExtraV1.CommittedSeal
	qst.ProposerSeal = nil
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

// ExtractQBFTExtra extracts all values of the QBFTExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractQBFTExtra(h *Header) (*QBFTExtra, error) {
	qbftExtra := new(QBFTExtra)
	err := rlp.DecodeBytes(h.Extra[:], qbftExtra)
	if err != nil {
		return nil, err
	}
	return qbftExtra, nil
}

// QBFTFilteredHeader returns a filtered header which some information (like committed seals, round, validator vote)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func QBFTFilteredHeader(h *Header) *Header {
	return QBFTFilteredHeaderWithRound(h, 0)
}

// QBFTFilteredHeaderWithRound returns the copy of the header with round number set to the given round number
// and commit seal set to its null value
func QBFTFilteredHeaderWithRound(h *Header, round uint32) *Header {
	newHeader := CopyHeader(h)
	qbftExtra, err := extractQBFTExtraForHash(newHeader)
	if err != nil {
		return nil
	}

	qbftExtra.CommittedSeal = [][]byte{}
	qbftExtra.ProposerSeal = []byte{}
	qbftExtra.Round = round

	payload, err := rlp.EncodeToBytes(&qbftExtra)
	if err != nil {
		return nil
	}

	newHeader.Extra = payload

	return newHeader
}

// The filtered headers above are what Header.Hash hashes, and they throw the
// committed seals away. Decoding the seals into a [][]byte first costs a 24
// byte slice header for every one-byte empty seal, so a 10MB extra-data took
// over 1.6GB to hash, and a peer could make us hash one before anything had
// checked its size. The decoders below are the ones Extract*Extra use with the
// seals swapped for skippedSeals: they accept and reject exactly the same
// input and return the same values, but never hold the seals in memory.

// skippedSeals decodes a list of byte strings like [][]byte does, making the
// same Stream calls in the same order (see decodeListSlice and decodeByteSlice
// in package rlp), but keeps none of them.
type skippedSeals struct{}

func (*skippedSeals) DecodeRLP(s *rlp.Stream) error {
	if _, err := s.List(); err != nil {
		return err
	}
	for {
		if _, err := s.Bytes(); err == rlp.EOL {
			break
		} else if err != nil {
			return err
		}
	}
	return s.ListEnd()
}

// extractIstanbulExtraForHash is ExtractIstanbulExtra without the committed
// seals, which are left nil.
func extractIstanbulExtraForHash(h *Header) (*IstanbulExtra, error) {
	if len(h.Extra) < IstanbulExtraVanity {
		return nil, ErrInvalidIstanbulHeaderExtra
	}
	var extra struct {
		Validators    []common.Address
		Seal          []byte
		CommittedSeal skippedSeals
	}
	if err := rlp.DecodeBytes(h.Extra[IstanbulExtraVanity:], &extra); err != nil {
		return nil, err
	}
	return &IstanbulExtra{Validators: extra.Validators, Seal: extra.Seal}, nil
}

// qbftExtraForHash mirrors QBFTExtra.DecodeRLP, minus the committed seals.
type qbftExtraForHash struct {
	QBFTExtra
}

func (qst *qbftExtraForHash) DecodeRLP(s *rlp.Stream) error {
	raw, err := s.Raw()
	if err != nil {
		return err
	}
	var v2 struct {
		VanityData    []byte
		Validators    []common.Address
		Vote          *ValidatorVote `rlp:"nil"`
		Round         uint32
		CommittedSeal skippedSeals
		ProposerSeal  []byte
	}
	if err := rlp.DecodeBytes(raw, &v2); err == nil {
		qst.VanityData = v2.VanityData
		qst.Validators = v2.Validators
		qst.Vote = v2.Vote
		qst.Round = v2.Round
		qst.ProposerSeal = v2.ProposerSeal
		return nil
	}
	var v1 struct {
		VanityData    []byte
		Validators    []common.Address
		Vote          *ValidatorVote `rlp:"nil"`
		Round         uint32
		CommittedSeal skippedSeals
	}
	if err := rlp.DecodeBytes(raw, &v1); err != nil {
		return err
	}
	qst.VanityData = v1.VanityData
	qst.Validators = v1.Validators
	qst.Vote = v1.Vote
	qst.Round = v1.Round
	return nil
}

// extractQBFTExtraForHash is ExtractQBFTExtra without the committed seals,
// which are left nil.
func extractQBFTExtraForHash(h *Header) (*QBFTExtra, error) {
	extra := new(qbftExtraForHash)
	if err := rlp.DecodeBytes(h.Extra[:], extra); err != nil {
		return nil, err
	}
	return &extra.QBFTExtra, nil
}
