package backend

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	qbftengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/testutils"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
)

// sealWithQuorum builds the next block on parent through the normal Prepare /
// FinalizeAndAssemble / engine Seal path, optionally rewrites its embedded
// validator list, and attaches committed seals from every key in signers.
func sealWithQuorum(t *testing.T, chain *core.BlockChain, backend *Backend, parent *types.Block, signers []*ecdsa.PrivateKey, embedded []common.Address) *types.Block {
	t.Helper()

	block := makeBlockWithoutSeal(chain, backend, parent, true)
	if embedded != nil {
		header := block.Header()
		if err := qbftengine.ApplyHeaderQBFTExtra(header, qbftengine.WriteValidators(embedded)); err != nil {
			t.Fatalf("rewrite embedded validators: %v", err)
		}
		block = block.WithSeal(header)
	}

	snap, err := backend.snapshot(chain, parent.NumberU64(), parent.Hash(), nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	engine := backend.EngineForBlockNumber(block.Number())
	block, err = engine.Seal(chain, block, snap.ValSet)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	header := block.Header()
	payload := qbftengine.PrepareCommittedSeal(header, 0)
	seals := make([][]byte, len(signers))
	for i, key := range signers {
		if seals[i], err = crypto.Sign(payload, key); err != nil {
			t.Fatalf("committed seal: %v", err)
		}
	}
	if err := engine.CommitHeader(header, seals, big.NewInt(0)); err != nil {
		t.Fatalf("commit header: %v", err)
	}
	return block.WithSeal(header)
}

// TestPostFork_EmbeddedValidatorsAcrossValidatorChange runs a real chain with
// FutureFork active from genesis through a validator being voted in. Blocks
// built by Prepare must keep importing on both sides of the change, and a block
// that still carries the old (pre-vote) list once the set has changed must be
// rejected.
func TestPostFork_EmbeddedValidatorsAcrossValidatorChange(t *testing.T) {
	keyA, _ := crypto.GenerateKey()
	keyE, _ := crypto.GenerateKey()
	addrA := crypto.PubkeyToAddress(keyA.PublicKey)
	addrE := crypto.PubkeyToAddress(keyE.PublicKey)

	genesis := testutils.Genesis([]common.Address{addrA})
	cfg := *genesis.Config
	cfg.FutureForkBlock = big.NewInt(0)
	genesis.Config = &cfg

	// Prepare stamps blocks at max(parent+period, now), so keep the period short
	// enough that consecutive blocks don't land in the future-block queue.
	config := copyConfig(istanbul.DefaultConfig)
	config.BlockPeriod = 1

	chain, backend := newBlockchainFromConfig(genesis, []*ecdsa.PrivateKey{keyA}, config)
	defer backend.Stop()

	// Block 1: A is the only validator and votes E in, which takes effect for
	// block 2 onwards.
	backend.candidates[addrE] = true
	block1 := sealWithQuorum(t, chain, backend, chain.Genesis(), []*ecdsa.PrivateKey{keyA}, nil)
	if _, err := chain.InsertChain(types.Blocks{block1}); err != nil {
		t.Fatalf("insert block 1: %v", err)
	}
	delete(backend.candidates, addrE)

	snap, err := backend.snapshot(chain, 1, block1.Hash(), nil)
	if err != nil {
		t.Fatalf("snapshot after block 1: %v", err)
	}
	if snap.ValSet.Size() != 2 {
		t.Fatalf("validator set after vote: have %d, want 2", snap.ValSet.Size())
	}

	// A block 2 that still embeds the old single-validator list is rejected,
	// even with a full quorum of committed seals on it.
	stale := sealWithQuorum(t, chain, backend, block1, []*ecdsa.PrivateKey{keyA, keyE}, []common.Address{addrA})
	if _, err := chain.InsertChain(types.Blocks{stale}); err != istanbulcommon.ErrInconsistentValidatorSet {
		t.Fatalf("stale embedded list: have %v, want %v", err, istanbulcommon.ErrInconsistentValidatorSet)
	}

	// Blocks 2 and 3 built by Prepare carry the new two-validator list and import.
	parent := block1
	for n := 2; n <= 3; n++ {
		block := sealWithQuorum(t, chain, backend, parent, []*ecdsa.PrivateKey{keyA, keyE}, nil)
		if _, err := chain.InsertChain(types.Blocks{block}); err != nil {
			t.Fatalf("insert block %d: %v", n, err)
		}
		parent = block
	}
	if head := chain.CurrentBlock().NumberU64(); head != 3 {
		t.Fatalf("head: have %d, want 3", head)
	}
}
