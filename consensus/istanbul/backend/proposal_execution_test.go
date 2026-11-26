package backend

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/misc"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/core/vm"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/trie"
)

// A validator runs a proposal before voting on it. The header checks don't
// cover what execution produces, so a proposer could otherwise get a quorum to
// certify a block with a made-up state root or receipts, or a transaction its
// sender can't pay for. No node can import that block, but a light client takes
// the certified header, and the receipts it commits to, as final.
func TestVerify_ExecutesProposal(t *testing.T) {
	chain, backend := newBlockChain(1)
	defer backend.Stop()

	// Block 1 pays the validator its block reward, so it can send a
	// transaction in block 2.
	block1 := commitWithProposerSeal(t, backend, makeBlockWithoutSeal(chain, backend, chain.Genesis(), true))
	if _, err := chain.InsertChain(types.Blocks{block1}); err != nil {
		t.Fatal(err)
	}
	broke, _ := crypto.GenerateKey()

	good := buildProposal(t, chain, backend, block1, backend.privateKey)
	if len(good.Transactions()) != 1 {
		t.Fatal("proposal doesn't carry the transaction")
	}
	if _, err := backend.Verify(good); err != nil {
		t.Fatalf("valid proposal refused: %v", err)
	}

	for name, tc := range map[string]struct {
		change func(*types.Header)
		body   types.Transactions
		want   error
	}{
		"a state root that doesn't match": {change: func(h *types.Header) { h.Root = common.Hash{0x01} }},
		"receipts that don't match":       {change: func(h *types.Header) { h.ReceiptHash = common.Hash{0x01} }},
		"a bloom that doesn't match":      {change: func(h *types.Header) { h.Bloom[0] ^= 0x01 }},
		"gas used that doesn't match":     {change: func(h *types.Header) { h.GasUsed++ }},
		// The header keeps the good block's roots, including its receipt for
		// a successful transfer.
		"a transaction its sender can't pay for": {
			body: types.Transactions{transfer(t, chain, broke, 0, good.BaseFee())},
			want: core.ErrInsufficientFunds,
		},
	} {
		t.Run(name, func(t *testing.T) {
			header := good.Header()
			body := good.Transactions()
			if tc.change != nil {
				tc.change(header)
			}
			if tc.body != nil {
				body = tc.body
				header.TxHash = types.DeriveSha(body, new(trie.Trie))
			}
			bad := types.NewBlockWithHeader(header).WithBody(body, nil)

			_, err := backend.Verify(bad)
			if err == nil {
				t.Fatal("proposal that can't be imported was accepted")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("have %v, want %v", err, tc.want)
			}
		})
	}

	// Running a proposal leaves the chain as it was, and the good one imports.
	if head := chain.CurrentBlock(); head.Hash() != block1.Hash() {
		t.Fatalf("head moved to %d while verifying", head.NumberU64())
	}
	if _, err := chain.InsertChain(types.Blocks{commitWithProposerSeal(t, backend, good)}); err != nil {
		t.Fatalf("valid proposal doesn't import: %v", err)
	}
}

// buildProposal builds the next block on parent the way the miner does, with
// one transfer from key.
func buildProposal(t *testing.T, chain *core.BlockChain, backend *Backend, parent *types.Block, key *ecdsa.PrivateKey) *types.Block {
	t.Helper()
	header := makeHeader(parent, backend.config)
	header.Coinbase = backend.Address()
	header.BaseFee = misc.CalcBaseFee(chain.Config(), parent.Header())
	if err := backend.Prepare(chain, header); err != nil {
		t.Fatal(err)
	}
	statedb, err := chain.StateAt(parent.Root())
	if err != nil {
		t.Fatal(err)
	}
	tx := transfer(t, chain, key, statedb.GetNonce(crypto.PubkeyToAddress(key.PublicKey)), header.BaseFee)
	statedb.Prepare(tx.Hash(), 0)
	receipt, err := core.ApplyTransaction(chain.Config(), chain, &header.Coinbase, new(core.GasPool).AddGas(header.GasLimit), statedb, header, tx, &header.GasUsed, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	block, err := backend.FinalizeAndAssemble(chain, header, statedb, types.Transactions{tx}, nil, types.Receipts{receipt})
	if err != nil {
		t.Fatal(err)
	}
	return block
}

func transfer(t *testing.T, chain *core.BlockChain, key *ecdsa.PrivateKey, nonce uint64, baseFee *big.Int) *types.Transaction {
	t.Helper()
	tx, err := types.SignNewTx(key, types.LatestSigner(chain.Config()), &types.LegacyTx{
		Nonce:    nonce,
		GasPrice: baseFee,
		Gas:      params.TxGas,
		To:       &common.Address{0x01},
		Value:    big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}
