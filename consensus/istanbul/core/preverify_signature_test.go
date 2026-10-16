package core

import (
	"errors"
	"math/big"
	"runtime"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
)

// countingCore returns a core whose validateFn accepts signatures starting with
// 0xAA and counts how often it is called.
func countingCore(calls *int) *core {
	return &core{
		validateFn: func(_ []byte, sig []byte) (common.Address, error) {
			*calls++
			if len(sig) > 0 && sig[0] == 0xAA {
				return common.HexToAddress("0x01"), nil
			}
			return common.Address{}, errors.New("invalid signature")
		},
		logger: log.New(),
		valSet: testValSet(4),
	}
}

// junkBlock is a block stuffed with empty legacy transactions, which decode to
// many times their encoded size.
func junkBlock(n int) *types.Block {
	txs := make([]*types.Transaction, n)
	for i := range txs {
		txs[i] = types.NewTx(&types.LegacyTx{})
	}
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1)}
	return types.NewBlock(header, txs, nil, nil, trie.NewStackTrie(nil))
}

func allocated(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// A PRE-PREPARE or ROUND-CHANGE with a bad signature must be rejected without
// decoding the block it carries.
func TestPreVerifySignature_RejectsBeforeDecodingBlock(t *testing.T) {
	block := junkBlock(100_000)

	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(0), block)
	pp.SetSignature([]byte{0x00})
	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0x00})

	for name, tc := range map[string]struct {
		code uint64
		msg  interface{}
	}{
		"preprepare":  {qbfttypes.PreprepareCode, pp},
		"roundchange": {qbfttypes.RoundChangeCode, rc},
	} {
		data, err := rlp.EncodeToBytes(tc.msg)
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		c := countingCore(&calls)

		var verr error
		used := allocated(func() { _, verr = c.preVerifySignature(tc.code, data) })
		if !errors.Is(verr, errInvalidSigner) {
			t.Fatalf("%s: expected errInvalidSigner, got %v", name, verr)
		}
		decoded := allocated(func() { qbfttypes.Decode(tc.code, data) })
		t.Logf("%s: %d byte message, pre-verify allocated %d, full decode %d", name, len(data), used, decoded)

		// Pre-verification copies the raw payload a few times; decoding blows it up.
		if used > 4*uint64(len(data)) {
			t.Errorf("%s: pre-verify allocated %d bytes for a %d byte message", name, used, len(data))
		}
		if decoded < 10*used {
			t.Errorf("%s: full decode allocated only %d bytes vs %d, test isn't measuring amplification", name, decoded, used)
		}
	}
}

// An oversized justification is rejected before any signature is checked or
// any entry is decoded.
func TestPreVerifySignature_RejectsOversizedJustification(t *testing.T) {
	block := makeBlock(1)
	prepares := make([]*qbfttypes.Prepare, 5) // valSet has 4
	for i := range prepares {
		prepares[i] = qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(0), block.Hash(), []byte{0xAA}, common.Address{})
	}
	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(1), block)
	pp.SetSignature([]byte{0xAA})
	pp.JustificationPrepares = prepares

	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0xAA})
	rc.Justification = prepares

	for name, tc := range map[string]struct {
		code uint64
		msg  interface{}
	}{
		"preprepare":  {qbfttypes.PreprepareCode, pp},
		"roundchange": {qbfttypes.RoundChangeCode, rc},
	} {
		data, err := rlp.EncodeToBytes(tc.msg)
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		c := countingCore(&calls)
		if _, err := c.preVerifySignature(tc.code, data); !errors.Is(err, errInvalidMessage) {
			t.Fatalf("%s: expected errInvalidMessage, got %v", name, err)
		}
		if calls != 0 {
			t.Fatalf("%s: %d signature checks ran before the length check", name, calls)
		}
	}
}

// A well signed message passes, and the full verification reuses the outer
// signature check rather than recovering it again.
func TestPreVerifySignature_AcceptsAndSeedsVerification(t *testing.T) {
	block := makeBlock(1)
	prepare := qbfttypes.NewPrepareWithSigAndSource(big.NewInt(1), big.NewInt(0), block.Hash(), []byte{0xAB}, common.Address{})

	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(1), block)
	pp.SetSignature([]byte{0xAA})
	pp.JustificationPrepares = []*qbfttypes.Prepare{prepare}

	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0xAA})
	rc.Justification = []*qbfttypes.Prepare{prepare}

	for name, tc := range map[string]struct {
		code uint64
		msg  interface{}
	}{
		"preprepare":  {qbfttypes.PreprepareCode, pp},
		"roundchange": {qbfttypes.RoundChangeCode, rc},
	} {
		data, err := rlp.EncodeToBytes(tc.msg)
		if err != nil {
			t.Fatal(err)
		}
		var calls int
		c := countingCore(&calls)
		verified, err := c.preVerifySignature(tc.code, data)
		if err != nil {
			t.Fatalf("%s: pre-verify failed: %v", name, err)
		}
		m, err := qbfttypes.Decode(tc.code, data)
		if err != nil {
			t.Fatal(err)
		}
		// The justification prepare's 0xAB signature is rejected by the stub,
		// so we expect exactly: one outer check in pre-verify, none for the
		// outer message in full verification, one for the prepare.
		if err := c.verifySignaturesWith(m, verified); !errors.Is(err, errInvalidSigner) {
			t.Fatalf("%s: expected the justification prepare to fail, got %v", name, err)
		}
		if calls != 2 {
			t.Fatalf("%s: expected 2 signature checks, got %d", name, calls)
		}
		if m.Source() != common.HexToAddress("0x01") {
			t.Fatalf("%s: source not set from the seeded check", name)
		}
	}
}

// Prepares and commits don't carry a block, so they skip pre-verification.
func TestPreVerifySignature_SkipsSmallMessages(t *testing.T) {
	var calls int
	c := countingCore(&calls)
	for _, code := range []uint64{qbfttypes.PrepareCode, qbfttypes.CommitCode} {
		if _, err := c.preVerifySignature(code, []byte{0xc0}); err != nil {
			t.Fatalf("code %d: %v", code, err)
		}
	}
	if calls != 0 {
		t.Fatalf("expected no signature checks, got %d", calls)
	}
}
