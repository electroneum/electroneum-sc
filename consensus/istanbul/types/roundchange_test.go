package qbfttypes

import (
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
)

func testBlock(txs ...*types.Transaction) *types.Block {
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: 30_000_000}
	return types.NewBlock(header, txs, nil, nil, trie.NewStackTrie(nil))
}

// encodeWithBody encodes rc with its prepared block's body replaced, keeping the
// header (and so the hash the signature covers) the same.
func encodeWithBody(t *testing.T, rc *RoundChange, txs []*types.Transaction, uncles []*types.Header) []byte {
	payload, err := rc.encodePayloadInternal()
	if err != nil {
		t.Fatal(err)
	}
	block := []interface{}{rc.PreparedBlock.Header(), txs, uncles}
	enc, err := rlp.EncodeToBytes([]interface{}{
		[]interface{}{rlp.RawValue(payload), rc.signature},
		block, rc.Justification,
	})
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestRoundChangeDecode_PreparedBodyBoundToHeader(t *testing.T) {
	tx := func(nonce uint64) *types.Transaction {
		return types.NewTx(&types.LegacyTx{Nonce: nonce, Gas: 21000, GasPrice: big.NewInt(1)})
	}
	block := testBlock(tx(0), tx(1))
	rc := NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
	rc.SetSignature([]byte{0xAA})

	// The genuine message decodes.
	data, err := rlp.EncodeToBytes(rc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(RoundChangeCode, data); err != nil {
		t.Fatalf("genuine round change rejected: %v", err)
	}
	// So does one with nothing prepared.
	empty := NewRoundChange(big.NewInt(1), big.NewInt(1), nil, nil, false)
	empty.SetSignature([]byte{0xAA})
	if data, err = rlp.EncodeToBytes(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(RoundChangeCode, data); err != nil {
		t.Fatalf("round change without a prepared block rejected: %v", err)
	}

	for name, data := range map[string][]byte{
		"swapped txs":   encodeWithBody(t, rc, []*types.Transaction{tx(5)}, nil),
		"dropped txs":   encodeWithBody(t, rc, nil, nil),
		"added uncle":   encodeWithBody(t, rc, block.Transactions(), []*types.Header{{Number: big.NewInt(0), Difficulty: big.NewInt(1)}}),
		"reordered txs": encodeWithBody(t, rc, []*types.Transaction{tx(1), tx(0)}, nil),
	} {
		if _, err := Decode(RoundChangeCode, data); err == nil {
			t.Errorf("%s: tampered prepared block accepted", name)
		}
	}
}
