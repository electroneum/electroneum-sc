package backend

import (
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	qbftengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/rlp"
)

// HasBadProposal reads the chain's bad block table, which the round change
// uses to drop a prepared block that can't be imported. The block hash leaves
// out the committed seals and proposer seal, and only covers the body through
// the header's roots. A good block with broken seals or another body used to
// fail import under the good block's hash and get recorded, so any peer that
// served us blocks during sync could mark a good proposal bad. Only a block
// that ran and failed belongs in the table.
func TestInsertChain_OnlyRecordsBlocksThatRan(t *testing.T) {
	chain, backend := newBlockChain(1)
	defer backend.Stop()

	good := commitWithProposerSeal(t, backend, makeBlockWithoutSeal(chain, backend, chain.Genesis(), true))

	for name, tamper := range map[string]func(*types.Block) *types.Block{
		"junk committed seal": func(b *types.Block) *types.Block {
			return withExtra(t, b, func(e *types.QBFTExtra) { e.CommittedSeal = [][]byte{make([]byte, types.IstanbulExtraSeal)} })
		},
		"no committed seals": func(b *types.Block) *types.Block {
			return withExtra(t, b, func(e *types.QBFTExtra) { e.CommittedSeal = [][]byte{} })
		},
		"a proposer seal before the fork": func(b *types.Block) *types.Block {
			return withExtra(t, b, func(e *types.QBFTExtra) { e.ProposerSeal = make([]byte, types.IstanbulExtraSeal) })
		},
		"another body": func(b *types.Block) *types.Block {
			return b.WithBody(types.Transactions{types.NewTx(&types.LegacyTx{Nonce: 1})}, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			copied := tamper(good)
			if copied.Hash() != good.Hash() {
				t.Fatal("tampered copy has a different hash")
			}
			if _, err := chain.InsertChain(types.Blocks{copied}); err == nil {
				t.Fatal("tampered copy imported")
			}
			if backend.HasBadProposal(good.Hash()) {
				t.Fatal("a tampered copy got the good block recorded as bad")
			}
		})
	}
	if _, err := chain.InsertChain(types.Blocks{good}); err != nil {
		t.Fatalf("good block doesn't import: %v", err)
	}

	// A block whose header and seals are fine but that doesn't run is bad
	// under its own hash, and still gets recorded.
	header := makeBlockWithoutSeal(chain, backend, good, true).Header()
	header.Root = common.Hash{0x01}
	broken := commitWithProposerSeal(t, backend, types.NewBlockWithHeader(header))
	if _, err := chain.InsertChain(types.Blocks{broken}); err == nil {
		t.Fatal("block with the wrong state root imported")
	}
	if !backend.HasBadProposal(broken.Hash()) {
		t.Fatal("block that failed to run wasn't recorded as bad")
	}
}

// commitWithProposerSeal adds the single validator's committed seal for round 0.
func commitWithProposerSeal(t *testing.T, backend *Backend, block *types.Block) *types.Block {
	t.Helper()
	header := block.Header()
	seal, err := crypto.Sign(qbftengine.PrepareCommittedSeal(header, 0), backend.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.EngineForBlockNumber(header.Number).CommitHeader(header, [][]byte{seal}, big.NewInt(0)); err != nil {
		t.Fatal(err)
	}
	return block.WithSeal(header)
}

// withExtra returns the block with its QBFT extra-data changed.
func withExtra(t *testing.T, block *types.Block, change func(*types.QBFTExtra)) *types.Block {
	t.Helper()
	header := block.Header()
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		t.Fatal(err)
	}
	change(extra)
	if header.Extra, err = rlp.EncodeToBytes(extra); err != nil {
		t.Fatal(err)
	}
	return block.WithSeal(header)
}
