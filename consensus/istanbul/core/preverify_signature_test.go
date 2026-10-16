package core

import (
	"errors"
	"math/big"
	"runtime"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
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
func junkBlock(n int, gasLimit uint64) *types.Block {
	txs := make([]*types.Transaction, n)
	for i := range txs {
		txs[i] = types.NewTx(&types.LegacyTx{})
	}
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: gasLimit}
	return types.NewBlock(header, txs, nil, nil, trie.NewStackTrie(nil))
}

// withBody re-encodes block with other transactions but the same header, as a
// peer tampering with a ROUND-CHANGE's unsigned prepared block would.
func withBody(t *testing.T, block *types.Block, txs []*types.Transaction) rlp.RawValue {
	enc, err := rlp.EncodeToBytes([]interface{}{block.Header(), txs, []*types.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	return enc
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
	// The gas limit allows the transactions, so it's the signature that stops it.
	block := junkBlock(100_000, 100_000*minTxGas)

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

// A block holding more transactions than its gas limit allows is rejected
// before decoding, even under a valid signature.
func TestPreVerifySignature_RejectsOverstuffedBlock(t *testing.T) {
	block := junkBlock(100, 99*minTxGas)

	pp := qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(0), block)
	pp.SetSignature([]byte{0xAA})
	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0xAA})

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
			t.Fatalf("%s: signature checked before the block bound", name)
		}
	}
	// One fewer transaction fits.
	ok := junkBlock(99, 99*minTxGas)
	pp = qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(0), ok)
	pp.SetSignature([]byte{0xAA})
	data, _ := rlp.EncodeToBytes(pp)
	var calls int
	if _, err := countingCore(&calls).preVerifySignature(qbfttypes.PreprepareCode, data); err != nil {
		t.Fatalf("block at the limit rejected: %v", err)
	}
}

// The prepared block in a ROUND-CHANGE isn't signed, only its hash is. Anyone
// holding a validator's ROUND-CHANGE could swap in a huge body under the same
// header. The header's gas limit still bounds it, and a header that doesn't
// match the signed digest is rejected outright.
func TestPreVerifySignature_RoundChangeBodySwap(t *testing.T) {
	block := junkBlock(1, 30_000_000)
	rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0xAA})

	var raw struct {
		Signed        rlp.RawValue
		PreparedBlock rlp.RawValue
		Justification rlp.RawValue
	}
	data, _ := rlp.EncodeToBytes(rc)
	if err := rlp.DecodeBytes(data, &raw); err != nil {
		t.Fatal(err)
	}
	tamper := func(body rlp.RawValue) []byte {
		enc, err := rlp.EncodeToBytes([]interface{}{raw.Signed, body, raw.Justification})
		if err != nil {
			t.Fatal(err)
		}
		return enc
	}
	var calls int
	c := countingCore(&calls)

	// The genuine message passes.
	if _, err := c.preVerifySignature(qbfttypes.RoundChangeCode, data); err != nil {
		t.Fatalf("genuine round change rejected: %v", err)
	}
	// Same header, junk body: bounded by the header's 30M gas limit.
	junk := junkBlock(100_000, 0).Transactions()
	if _, err := c.preVerifySignature(qbfttypes.RoundChangeCode, tamper(withBody(t, block, junk))); !errors.Is(err, errInvalidMessage) {
		t.Fatalf("swapped body: expected errInvalidMessage, got %v", err)
	}
	// Different header with a huge gas limit to dodge the bound: hash mismatch.
	inflated := junkBlock(100_000, 100_000*minTxGas)
	if _, err := c.preVerifySignature(qbfttypes.RoundChangeCode, tamper(withBody(t, inflated, junk))); !errors.Is(err, errInvalidMessage) {
		t.Fatalf("swapped header: expected errInvalidMessage, got %v", err)
	}
}

// End to end with a real key: what an honest validator signs is exactly what
// pre-verification checks, so honest messages keep working.
func TestPreVerifySignature_RealSignature(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	policy := istanbul.NewRoundRobinProposerPolicy()
	policy.Use(istanbul.ValidatorSortByByte())
	c := &core{logger: log.New(), valSet: validator.NewSet([]common.Address{addr}, policy)}
	c.validateFn = c.checkValidatorSignature

	sign := func(m qbfttypes.QBFTMessage) []byte {
		payload, err := m.EncodePayloadForSigning()
		if err != nil {
			t.Fatal(err)
		}
		sig, err := crypto.Sign(crypto.Keccak256(payload), key)
		if err != nil {
			t.Fatal(err)
		}
		m.SetSignature(sig)
		enc, err := rlp.EncodeToBytes(m)
		if err != nil {
			t.Fatal(err)
		}
		return enc
	}
	signer := types.LatestSignerForChainID(big.NewInt(1))
	txs := make([]*types.Transaction, 3)
	for i := range txs {
		txs[i] = types.MustSignNewTx(key, signer, &types.DynamicFeeTx{ChainID: big.NewInt(1), Nonce: uint64(i), Gas: 21000, GasFeeCap: big.NewInt(1), Data: []byte{1, 2, 3}})
	}
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: 30_000_000, BaseFee: big.NewInt(1)}
	block := types.NewBlock(header, txs, nil, nil, trie.NewStackTrie(nil))

	for name, tc := range map[string]struct {
		code uint64
		msg  qbfttypes.QBFTMessage
	}{
		"preprepare":  {qbfttypes.PreprepareCode, qbfttypes.NewPreprepare(big.NewInt(1), big.NewInt(0), block)},
		"roundchange": {qbfttypes.RoundChangeCode, qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)},
		"empty rc":    {qbfttypes.RoundChangeCode, qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), nil, nil, false)},
	} {
		data := sign(tc.msg)
		verified, err := c.preVerifySignature(tc.code, data)
		if err != nil {
			t.Fatalf("%s: pre-verify rejected an honest message: %v", name, err)
		}
		m, err := qbfttypes.Decode(tc.code, data)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		c.validateFn = func(data, sig []byte) (common.Address, error) {
			calls++
			return c.checkValidatorSignature(data, sig)
		}
		if err := c.verifySignaturesWith(m, verified); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c.validateFn = c.checkValidatorSignature
		if calls != 0 || m.Source() != addr {
			t.Fatalf("%s: seeded check not reused (calls %d, source %x)", name, calls, m.Source())
		}
	}
}
