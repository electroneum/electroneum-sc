// Copyright 2024 The Electroneum Authors
// This file is part of the electroneum-sc library.
//
// The committed-seal validation added to the COMMIT path (core.verifyCommittedSeal)
// recovers seals over core.PrepareCommittedSeal, while the network-wide check
// (Engine.Signers / Engine.verifyCommittedSeals) recovers them over
// qbftengine.PrepareCommittedSeal. The local check is only symmetric with the
// network check if these two functions produce byte-identical payloads. They are
// currently independent copies; this test locks that invariant so a future edit to
// one cannot silently split consensus between block builders and block verifiers.

package qbftengine_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	qbftcore "github.com/electroneum/electroneum-sc/consensus/istanbul/core"
	qbftengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/rlp"
)

func TestPrepareCommittedSeal_CoreEngineParity(t *testing.T) {
	// Use a header carrying a decodable QBFT extra. PrepareCommittedSeal hashes
	// QBFTFilteredHeaderWithRound, which returns a round-independent nil header if
	// the extra cannot be decoded — so a vanity-only extra would make this test
	// pass trivially and never detect a round divergence.
	extra, err := rlp.EncodeToBytes(&types.QBFTExtra{
		VanityData:    make([]byte, types.IstanbulExtraVanity),
		Validators:    []common.Address{},
		Round:         0,
		CommittedSeal: [][]byte{},
	})
	if err != nil {
		t.Fatalf("failed to encode qbft extra: %v", err)
	}
	header := &types.Header{
		Number:     big.NewInt(7),
		GasLimit:   8000000,
		Difficulty: big.NewInt(1),
		MixDigest:  types.IstanbulDigest,
		Extra:      extra,
	}

	// Sanity check that the payload actually varies with the round, so the parity
	// assertion below is meaningful rather than comparing two constant nil-hashes.
	if bytes.Equal(qbftengine.PrepareCommittedSeal(header, 0), qbftengine.PrepareCommittedSeal(header, 1)) {
		t.Fatal("test header does not bind the round; parity check would be vacuous")
	}

	for _, round := range []uint32{0, 1, 2, 7, 255, 1 << 20} {
		core := qbftcore.PrepareCommittedSeal(header, round)
		engine := qbftengine.PrepareCommittedSeal(header, round)
		if !bytes.Equal(core, engine) {
			t.Fatalf("PrepareCommittedSeal diverged at round %d: core=%x engine=%x", round, core, engine)
		}
	}
}
