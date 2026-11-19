// Copyright 2026 The electroneum-sc Authors
// This file is part of the electroneum-sc library.
//
// The electroneum-sc library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The electroneum-sc library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the electroneum-sc library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"bytes"
	"math/big"
	"math/rand"
	"runtime"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/rlp"
)

// Reported via Bugcrowd: hashing an Istanbul header decoded its committed
// seals into a [][]byte, twice, so a 10MB extra-data of empty seals allocated
// over 1.6GB per Header.Hash call. Peers could make us hash such a header
// before anything checked its size.

// The filtered-header functions as they were before the hash path stopped
// decoding committed seals. Block hashes are consensus, so the new path has to
// agree with these on every input, valid or not.

func referenceHash(h *Header) common.Hash {
	if h != nil && h.MixDigest == IstanbulDigest {
		if filtered := referenceFilteredHeader(h); filtered != nil {
			return rlpHash(filtered)
		}
	}
	return rlpHash(h)
}

func referenceFilteredHeader(h *Header) *Header {
	if _, err := ExtractIstanbulExtra(h); err != nil {
		return referenceQBFTFilteredHeaderWithRound(h, 0)
	}
	return referenceIstanbulFilteredHeader(h, true)
}

func referenceIstanbulFilteredHeader(h *Header, keepSeal bool) *Header {
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

func referenceQBFTFilteredHeaderWithRound(h *Header, round uint32) *Header {
	newHeader := CopyHeader(h)
	qbftExtra, err := ExtractQBFTExtra(newHeader)
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

func encodedHeader(h *Header) []byte {
	if h == nil {
		return nil
	}
	enc, err := rlp.EncodeToBytes(h)
	if err != nil {
		panic(err)
	}
	return enc
}

// checkHashParity compares every filtered-header entry point against the
// reference implementation for one extra-data.
func checkHashParity(t *testing.T, extra []byte) {
	t.Helper()
	h := &Header{
		ParentHash: common.Hash{1},
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(1),
		GasLimit:   30_000_000,
		Time:       1234,
		MixDigest:  IstanbulDigest,
		Extra:      extra,
	}
	if have, want := h.Hash(), referenceHash(h); have != want {
		t.Fatalf("Hash mismatch for extra %x: have %x, want %x", extra, have, want)
	}
	if have, want := encodedHeader(FilteredHeader(h)), encodedHeader(referenceFilteredHeader(h)); !bytes.Equal(have, want) {
		t.Fatalf("FilteredHeader mismatch for extra %x:\nhave %x\nwant %x", extra, have, want)
	}
	for _, keep := range []bool{true, false} {
		if have, want := encodedHeader(IstanbulFilteredHeader(h, keep)), encodedHeader(referenceIstanbulFilteredHeader(h, keep)); !bytes.Equal(have, want) {
			t.Fatalf("IstanbulFilteredHeader(keepSeal=%v) mismatch for extra %x:\nhave %x\nwant %x", keep, extra, have, want)
		}
	}
	for _, round := range []uint32{0, 1, 7} {
		if have, want := encodedHeader(QBFTFilteredHeaderWithRound(h, round)), encodedHeader(referenceQBFTFilteredHeaderWithRound(h, round)); !bytes.Equal(have, want) {
			t.Fatalf("QBFTFilteredHeaderWithRound(%d) mismatch for extra %x:\nhave %x\nwant %x", round, extra, have, want)
		}
	}
	// The hash decoders must accept exactly what the full ones do, and agree
	// on everything but the committed seals.
	full, fullErr := ExtractIstanbulExtra(h)
	lean, leanErr := extractIstanbulExtraForHash(h)
	if (fullErr == nil) != (leanErr == nil) {
		t.Fatalf("Istanbul decoders disagree on extra %x: full %v, hash %v", extra, fullErr, leanErr)
	}
	if fullErr == nil {
		full.CommittedSeal = nil
		if !bytes.Equal(mustEncode(t, full), mustEncode(t, lean)) {
			t.Fatalf("Istanbul decoders disagree on extra %x: full %+v, hash %+v", extra, full, lean)
		}
	}
	fullQ, fullErr := ExtractQBFTExtra(h)
	leanQ, leanErr := extractQBFTExtraForHash(h)
	if (fullErr == nil) != (leanErr == nil) {
		t.Fatalf("QBFT decoders disagree on extra %x: full %v, hash %v", extra, fullErr, leanErr)
	}
	if fullErr == nil {
		fullQ.CommittedSeal = nil
		if !bytes.Equal(mustEncode(t, fullQ), mustEncode(t, leanQ)) {
			t.Fatalf("QBFT decoders disagree on extra %x: full %+v, hash %+v", extra, fullQ, leanQ)
		}
	}
}

// mustEncode RLP-encodes v or fails the test.
func mustEncode(t testing.TB, v interface{}) []byte {
	enc, err := rlp.EncodeToBytes(v)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func seal(b byte) []byte { return bytes.Repeat([]byte{b}, IstanbulExtraSeal) }

// hashParitySeeds returns well-formed and malformed extra-data in the shapes
// that matter: legacy Istanbul, QBFT before and after the proposer seal, and
// near misses that one decoder or the other must reject.
func hashParitySeeds(t testing.TB) [][]byte {
	vals := []common.Address{{0x01}, {0x02}, {0x03}, {0x04}}
	vanity := make([]byte, IstanbulExtraVanity)
	vote := &ValidatorVote{RecipientAddress: common.Address{0xaa}, VoteType: QBFTAuthVote}

	istanbul := func(e *IstanbulExtra) []byte { return append(append([]byte{}, vanity...), mustEncode(t, e)...) }
	seeds := [][]byte{
		nil,
		{},
		vanity[:31],
		vanity,
		istanbul(&IstanbulExtra{}),
		istanbul(&IstanbulExtra{Validators: vals}),
		istanbul(&IstanbulExtra{Validators: vals, Seal: seal(0x11)}),
		istanbul(&IstanbulExtra{Validators: vals, Seal: seal(0x11), CommittedSeal: [][]byte{seal(0x21), seal(0x22), seal(0x23)}}),
		istanbul(&IstanbulExtra{CommittedSeal: [][]byte{{}, {}, {0x01}, {0x80}}}),
		mustEncode(t, &QBFTExtra{VanityData: vanity, Validators: vals}),
		mustEncode(t, &QBFTExtra{VanityData: vanity, Validators: vals, Round: 3, CommittedSeal: [][]byte{seal(0x31), seal(0x32), seal(0x33)}}),
		mustEncode(t, &QBFTExtra{VanityData: vanity, Validators: vals, Vote: vote, Round: 1, CommittedSeal: [][]byte{seal(0x41)}}),
		mustEncode(t, &QBFTExtra{VanityData: vanity, Validators: vals, Round: 2, CommittedSeal: [][]byte{seal(0x51), seal(0x52), seal(0x53)}, ProposerSeal: seal(0x61)}),
		mustEncode(t, &QBFTExtra{VanityData: vanity, Vote: vote, CommittedSeal: [][]byte{{}, {0x00}, {0x7f}}, ProposerSeal: seal(0x71)}),
	}
	// Committed seals that [][]byte must reject: a nested list, a single byte
	// in long form, a truncated string.
	for _, bad := range [][]byte{
		{0xc1, 0xc0},
		{0xc2, 0x81, 0x05},
		{0xc2, 0x82, 0x01},
		{0x80},
	} {
		qbft := append(append([]byte{}, mustEncode(t, vanity)...), mustEncode(t, vals)...)
		qbft = append(qbft, 0xc0, 0x80) // nil vote, round 0
		qbft = append(qbft, bad...)
		seeds = append(seeds, wrapList(qbft))
		ist := append(append([]byte{}, mustEncode(t, vals)...), 0x80)
		ist = append(ist, bad...)
		seeds = append(seeds, append(append([]byte{}, vanity...), wrapList(ist)...))
	}
	// Other near misses: trailing bytes, an extra field, an oversized round,
	// and an empty vote of the wrong kind.
	good := mustEncode(t, &QBFTExtra{VanityData: vanity, Validators: vals, CommittedSeal: [][]byte{seal(0x81)}})
	seeds = append(seeds,
		append(append([]byte{}, good...), 0x00),
		append(append([]byte{}, vanity...), append(mustEncode(t, &IstanbulExtra{Validators: vals}), 0x00)...),
		wrapList(append(append(append([]byte{}, mustEncode(t, vanity)...), 0xc0, 0x80, 0x80, 0xc0, 0x80), 0x80)),
		wrapList(append(append([]byte{}, mustEncode(t, vanity)...), 0xc0, 0xc0, 0x85, 0x01, 0x00, 0x00, 0x00, 0x00, 0xc0)),
		wrapList(append(append([]byte{}, mustEncode(t, vanity)...), 0xc0, 0x80, 0x80, 0xc0)),
	)
	return seeds
}

// wrapList puts an RLP list header in front of already encoded items.
func wrapList(content []byte) []byte {
	var buf bytes.Buffer
	n := len(content)
	switch {
	case n < 56:
		buf.WriteByte(0xc0 + byte(n))
	case n < 1<<8:
		buf.Write([]byte{0xf8, byte(n)})
	case n < 1<<16:
		buf.Write([]byte{0xf9, byte(n >> 8), byte(n)})
	default:
		buf.Write([]byte{0xfa, byte(n >> 16), byte(n >> 8), byte(n)})
	}
	buf.Write(content)
	return buf.Bytes()
}

func TestIstanbulHashParitySeeds(t *testing.T) {
	for _, extra := range hashParitySeeds(t) {
		checkHashParity(t, extra)
	}
}

// Random mutations of the seeds, so ordinary test runs cover more than the
// hand-picked cases. FuzzIstanbulHashParity goes further when run with -fuzz.
func TestIstanbulHashParityMutations(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seeds := hashParitySeeds(t)
	for i := 0; i < 20000; i++ {
		extra := append([]byte{}, seeds[rng.Intn(len(seeds))]...)
		for n := rng.Intn(4) + 1; n > 0; n-- {
			switch op := rng.Intn(4); {
			case op == 0 && len(extra) > 0:
				extra[rng.Intn(len(extra))] = byte(rng.Intn(256))
			case op == 1:
				pos := rng.Intn(len(extra) + 1)
				extra = append(extra[:pos], append([]byte{byte(rng.Intn(256))}, extra[pos:]...)...)
			case op == 2 && len(extra) > 0:
				pos := rng.Intn(len(extra))
				extra = append(extra[:pos], extra[pos+1:]...)
			case op == 3 && len(extra) > 0:
				extra = extra[:rng.Intn(len(extra))]
			}
		}
		checkHashParity(t, extra)
	}
}

func FuzzIstanbulHashParity(f *testing.F) {
	for _, extra := range hashParitySeeds(f) {
		f.Add(extra)
	}
	f.Fuzz(func(t *testing.T, extra []byte) {
		checkHashParity(t, extra)
	})
}

// stuffedSeals builds extra-data in the given shape whose committed seals are
// n copies of item, the way the reported header was built.
func stuffedSeals(t testing.TB, qbft bool, n int, item byte) []byte {
	seals := wrapList(bytes.Repeat([]byte{item}, n))
	if qbft {
		return wrapList(append(append(mustEncode(t, make([]byte, IstanbulExtraVanity)), 0xc0, 0xc0, 0x80), seals...))
	}
	return append(make([]byte, IstanbulExtraVanity), wrapList(append([]byte{0xc0, 0x80}, seals...))...)
}

// Hashing a header must cost about its own size however its seals are stuffed.
func TestIstanbulHashAllocation(t *testing.T) {
	const n = 10_000_000
	for _, tt := range []struct {
		name string
		qbft bool
		item byte
	}{
		{"istanbul/empty", false, 0x80},
		{"istanbul/byte", false, 0x01},
		{"qbft/empty", true, 0x80},
		{"qbft/byte", true, 0x01},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := &Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), MixDigest: IstanbulDigest, Extra: stuffedSeals(t, tt.qbft, n, tt.item)}
			if err := h.SanityCheck(); err == nil {
				t.Fatal("stuffed header passed SanityCheck")
			}
			// Make sure the hash path decodes the seals rather than falling
			// back to hashing the raw header.
			if FilteredHeader(h) == nil {
				t.Fatal("stuffed extra-data did not decode")
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			h.Hash()
			runtime.ReadMemStats(&after)

			// The header copy, the QBFT decoder's raw copy and two passes over
			// the seals (6 fields, then 5) each cost about the input size;
			// decoding the seals for real cost over 160 times it.
			limit := uint64(5 * len(h.Extra))
			if alloc := after.TotalAlloc - before.TotalAlloc; alloc > limit {
				t.Fatalf("Hash of %d byte extra-data allocated %d bytes, want <= %d", len(h.Extra), alloc, limit)
			}
		})
	}
}

// The full decoders still return the committed seals the hash path skips.
func TestExtractExtraKeepsCommittedSeals(t *testing.T) {
	seals := [][]byte{seal(0x01), seal(0x02)}
	h := &Header{MixDigest: IstanbulDigest, Extra: mustEncode(t, &QBFTExtra{VanityData: make([]byte, 32), CommittedSeal: seals, ProposerSeal: seal(0x03)})}
	extra, err := ExtractQBFTExtra(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(extra.CommittedSeal) != 2 || !bytes.Equal(extra.CommittedSeal[1], seals[1]) || !bytes.Equal(extra.ProposerSeal, seal(0x03)) {
		t.Fatalf("ExtractQBFTExtra lost seals: %x / %x", extra.CommittedSeal, extra.ProposerSeal)
	}
	h.Extra = append(make([]byte, IstanbulExtraVanity), mustEncode(t, &IstanbulExtra{Seal: seal(0x04), CommittedSeal: seals})...)
	ist, err := ExtractIstanbulExtra(h)
	if err != nil {
		t.Fatal(err)
	}
	if len(ist.CommittedSeal) != 2 || !bytes.Equal(ist.CommittedSeal[0], seals[0]) {
		t.Fatalf("ExtractIstanbulExtra lost seals: %x", ist.CommittedSeal)
	}
}
