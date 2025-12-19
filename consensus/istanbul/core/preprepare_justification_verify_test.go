package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/log"
)

func TestVerifySignatures_PreprepareRejectsBadJustificationPrepares(t *testing.T) {
	// Addresses we want validateFn to return on successful "signature validation"
	proposerAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	validatorAddr := common.HexToAddress("0x2000000000000000000000000000000000000002")

	// Stub validateFn:
	// - outer Preprepare signature 0xAA => proposerAddr
	// - good Prepare signature 0xBB     => validatorAddr
	// - anything else                   => error
	validateFn := func(_payload []byte, sig []byte) (common.Address, error) {
		if len(sig) == 0 {
			return common.Address{}, errors.New("empty signature")
		}
		switch sig[0] {
		case 0xAA:
			return proposerAddr, nil
		case 0xBB:
			return validatorAddr, nil
		default:
			return common.Address{}, errors.New("invalid signature")
		}
	}

	c := &core{
		validateFn: validateFn,
		// verifySignatures() calls c.currentLogger(), which calls c.logger.New(...)
		// so logger must be non-nil.
		logger: log.New(),
	}

	// Build a Preprepare (round > 0 so it can carry justification)
	block := makeBlock(1)

	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(1), block)
	pp.SetSignature([]byte{0xAA}) // valid outer preprepare signature per stub validateFn

	// Add a justification PREPARE with a BAD signature (0x00)
	// If verifySignatures() correctly verifies JustificationPrepares, it must fail.
	badPrepare := qbfttypes.NewPrepareWithSigAndSource(
		big.NewInt(1),
		big.NewInt(0),
		block.Hash(),
		[]byte{0x00}, // invalid per validateFn
		common.Address{}, // source is irrelevant; verifySignatures should authenticate it
	)

	pp.JustificationPrepares = []*qbfttypes.Prepare{badPrepare}
	pp.JustificationRoundChanges = nil

	err := c.verifySignatures(pp)
	if err == nil {
		t.Fatalf("expected verifySignatures(preprepare) to fail due to bad JustificationPrepares signature, but got nil")
	}
	if !errors.Is(err, errInvalidSigner) {
		t.Fatalf("expected errInvalidSigner, got: %v", err)
	}
}

// Sanity test: with valid embedded Prepare signatures, verifySignatures should pass and set sources.
func TestVerifySignatures_PreprepareAcceptsGoodJustificationPreparesAndSetsSource(t *testing.T) {
	proposerAddr := common.HexToAddress("0x1000000000000000000000000000000000000001")
	validatorAddr := common.HexToAddress("0x2000000000000000000000000000000000000002")

	validateFn := func(_payload []byte, sig []byte) (common.Address, error) {
		if len(sig) == 0 {
			return common.Address{}, errors.New("empty signature")
		}
		switch sig[0] {
		case 0xAA:
			return proposerAddr, nil
		case 0xBB:
			return validatorAddr, nil
		default:
			return common.Address{}, errors.New("invalid signature")
		}
	}

	c := &core{
		validateFn: validateFn,
		logger:     log.New(),
	}

	block := makeBlock(1)

	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(1), block)
	pp.SetSignature([]byte{0xAA})

	goodPrepare := qbfttypes.NewPrepareWithSigAndSource(
		big.NewInt(1),
		big.NewInt(0),
		block.Hash(),
		[]byte{0xBB},
		common.Address{}, // will be overwritten by verifySignatures via SetSource
	)

	pp.JustificationPrepares = []*qbfttypes.Prepare{goodPrepare}
	pp.JustificationRoundChanges = nil

	if err := c.verifySignatures(pp); err != nil {
		t.Fatalf("expected verifySignatures(preprepare) to succeed, got: %v", err)
	}

	// verifySignatures() must SetSource on the outer message and on embedded prepares
	if pp.Source() != proposerAddr {
		t.Fatalf("expected preprepare source %s, got %s", proposerAddr.Hex(), pp.Source().Hex())
	}
	if pp.JustificationPrepares[0].Source() != validatorAddr {
		t.Fatalf("expected embedded prepare source %s, got %s", validatorAddr.Hex(), pp.JustificationPrepares[0].Source().Hex())
	}
}
