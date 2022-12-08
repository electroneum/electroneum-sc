package qbfttypes

import (
	"bytes"
	"fmt"
	"io"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/rlp"
)

// ROUND-CHANGE
type RoundChange struct {
	SignedRoundChangePayload
	PreparedBlock *types.Block
	Justification []*Prepare
}

func NewRoundChange(sequence *big.Int, round *big.Int, preparedRound *big.Int, preparedBlock istanbul.Proposal, hasBadProposal bool) *RoundChange {
	roundChange := &RoundChange{
		SignedRoundChangePayload: SignedRoundChangePayload{
			CommonPayload: CommonPayload{
				code:     RoundChangeCode,
				Sequence: sequence,
				Round:    round,
			},
			PreparedRound:  preparedRound,
			PreparedDigest: common.Hash{},
		},
	}

	if preparedBlock != nil {
		roundChange.PreparedBlock = preparedBlock.(*types.Block)
		roundChange.PreparedDigest = preparedBlock.Hash()
		roundChange.HasBadProposal = hasBadProposal
	}

	return roundChange
}

type SignedRoundChangePayload struct {
	CommonPayload
	PreparedRound  *big.Int
	PreparedDigest common.Hash
	HasBadProposal bool
}

func (p *SignedRoundChangePayload) String() string {
	return fmt.Sprintf("RoundChange {seq=%v, round=%v, pr=%v, pv=%v, hasBadProposal=%v}",
		p.Sequence, p.Round, p.PreparedRound, p.PreparedDigest.Hex(), p.HasBadProposal)
}

func (p *SignedRoundChangePayload) EncodeRLP(w io.Writer) error {
	var encodedPayload rlp.RawValue
	encodedPayload, err := p.encodePayloadInternal()
	if err != nil {
		return err
	}

	return rlp.Encode(
		w,
		[]interface{}{encodedPayload, p.signature})
}

func (p *SignedRoundChangePayload) DecodeRLP(stream *rlp.Stream) error {
	// Signed Payload
	if _, err := stream.List(); err != nil {
		log.Error("IBFT: Error List() Signed Payload", "err", err)
		return err
	}

	// Payload
	encodedPayload, err := stream.Raw()
	if err != nil {
		log.Error("IBFT: Error Raw()", "err", err)
		return err
	}

	payloadStream := rlp.NewStream(bytes.NewReader(encodedPayload), 0)

	if _, err = payloadStream.List(); err != nil {
		log.Error("IBFT: Error List() Payload", "err", err)
		return err
	}

	if err = payloadStream.Decode(&p.Sequence); err != nil {
		log.Error("IBFT: Error Decode(&m.Sequence)", "err", err)
		return err
	}
	if err = payloadStream.Decode(&p.Round); err != nil {
		log.Error("IBFT: Error Decode(&m.Round)", "err", err)
		return err
	}

	// Prepared
	var size uint64
	if size, err = payloadStream.List(); err != nil {
		log.Error("IBFT: Error List() Prepared", "err", err)
		return err
	}
	if size > 0 {
		if err = payloadStream.Decode(&p.PreparedRound); err != nil {
			log.Error("IBFT: Error Decode(&m.PreparedRound)", "err", err)
			return err
		}
		if err = payloadStream.Decode(&p.PreparedDigest); err != nil {
			log.Error("IBFT: Error Decode(&p.PreparedDigest)", "err", err)
			return err
		}
		if err = payloadStream.Decode(&p.HasBadProposal); err != nil {
			log.Error("IBFT: Error Decode(&p.HasBadProposal)", "err", err)
			return err
		}
	}
	// End Prepared
	if err = payloadStream.ListEnd(); err != nil {
		return err
	}

	// End Payload
	if err = payloadStream.ListEnd(); err != nil {
		return err
	}

	if err = stream.Decode(&p.signature); err != nil {
		return err
	}
	// End SignedPayload
	if err = stream.ListEnd(); err != nil {
		return err
	}

	p.code = RoundChangeCode

	log.Info("IBFT: Correctly decoded SignedRoundChangePayload", "p", p)

	return nil
}

func (p *SignedRoundChangePayload) encodePayloadInternal() ([]byte, error) {
	var prepared = []interface{}{}
	if p.PreparedRound != nil && !common.EmptyHash(p.PreparedDigest) {
		prepared = []interface{}{p.PreparedRound, p.PreparedDigest, p.HasBadProposal}
	}
	return rlp.EncodeToBytes(
		[]interface{}{
			p.Sequence,
			p.Round,
			prepared})
}

func (p *SignedRoundChangePayload) EncodePayloadForSigning() ([]byte, error) {
	var encodedPayload rlp.RawValue
	encodedPayload, err := p.encodePayloadInternal()
	if err != nil {
		return nil, err
	}

	return rlp.EncodeToBytes(
		[]interface{}{
			p.Code(),
			encodedPayload,
		})
}

func (m *RoundChange) EncodeRLP(w io.Writer) error {
	var encodedPayload rlp.RawValue
	encodedPayload, err := m.encodePayloadInternal()
	if err != nil {
		return err
	}

	return rlp.Encode(
		w,
		[]interface{}{
			[]interface{}{
				encodedPayload,
				m.signature,
			},
			m.PreparedBlock, m.Justification,
		})
}

func (m *RoundChange) DecodeRLP(stream *rlp.Stream) error {
	var err error

	// RoundChange Message
	if _, err = stream.List(); err != nil {
		return err
	}

	// Signed Payload
	if _, err = stream.List(); err != nil {
		log.Error("IBFT: Error List() Signed Payload", "err", err)
		return err
	}

	// Payload
	encodedPayload, err := stream.Raw()
	if err != nil {
		log.Error("IBFT: Error Raw()", "err", err)
		return err
	}

	payloadStream := rlp.NewStream(bytes.NewReader(encodedPayload), 0)

	if _, err = payloadStream.List(); err != nil {
		log.Error("IBFT: Error List() Payload", "err", err)
		return err
	}

	if err = payloadStream.Decode(&m.Sequence); err != nil {
		log.Error("IBFT: Error Decode(&m.Sequence)", "err", err)
		return err
	}
	if err = payloadStream.Decode(&m.Round); err != nil {
		log.Error("IBFT: Error Decode(&m.Round)", "err", err)
		return err
	}

	// Prepared
	var size uint64
	if size, err = payloadStream.List(); err != nil {
		log.Error("IBFT: Error List() Prepared", "err", err)
		return err
	}
	if size > 0 {
		if err = payloadStream.Decode(&m.PreparedRound); err != nil {
			log.Error("IBFT: Error Decode(&m.PreparedRound)", "err", err)
			return err
		}
		if err = payloadStream.Decode(&m.PreparedDigest); err != nil {
			log.Error("IBFT: Error Decode(&m.PreparedDigest)", "err", err)
			return err
		}
		if err = payloadStream.Decode(&m.HasBadProposal); err != nil {
			log.Error("IBFT: Error Decode(&m.HasBadProposal)", "err", err)
			return err
		}
	}
	// End Prepared
	if err = payloadStream.ListEnd(); err != nil {
		return err
	}

	// End Payload
	if err = payloadStream.ListEnd(); err != nil {
		return err
	}

	if err = stream.Decode(&m.signature); err != nil {
		return err
	}
	// End SignedPayload
	if err = stream.ListEnd(); err != nil {
		return err
	}

	if _, size, err = stream.Kind(); err != nil {
		log.Error("IBFT: Error Kind()", "err", err)
		return err
	}
	if size == 0 {
		if _, err = stream.Raw(); err != nil {
			log.Error("IBFT: Error Raw()", "err", err)
			return err
		}
	} else {
		if err = stream.Decode(&m.PreparedBlock); err != nil {
			log.Error("IBFT: Error Decode(&m.PreparedDigest)", "err", err)
			return err
		}
		if m.PreparedBlock.Hash() != m.PreparedDigest {
			log.Error("IBFT: Error m.PreparedDigest.Hash() != digest")
			return istanbulcommon.ErrFailedDecodePreprepare
		}
	}

	if _, size, err = stream.Kind(); err != nil {
		log.Error("IBFT: Error Kind()", "err", err)
		return err
	}
	if size == 0 {
		if _, err = stream.Raw(); err != nil {
			log.Error("IBFT: Error Raw()", "err", err)
			return err
		}
	} else {
		if err = stream.Decode(&m.Justification); err != nil {
			log.Error("IBFT: Error Decode(&m.Justification)", "err", err)
			return err
		}
	}

	// End RoundChange Message
	if err = stream.ListEnd(); err != nil {
		return err
	}

	m.code = RoundChangeCode

	return nil
}
