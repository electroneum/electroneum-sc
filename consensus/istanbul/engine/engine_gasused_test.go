// Copyright 2026 The Electroneum Authors
//
// Regression tests for the GasUsed <= GasLimit invariant that
// verifyCascadingFields now enforces, matching beacon/clique/ethash.
//
// Reported via Bugcrowd: the QBFT proposal gate accepted a header claiming more
// gas consumed than the block's own limit. Such a header cannot describe a real
// execution — core.ApplyTransaction meters against a GasPool seeded from
// header.GasLimit — but it was only rejected later, during canonical import in
// ValidateState, after the block had been executed. VerifyBlockProposal runs
// before honest validators sign PREPARE/COMMIT, so the pre-vote gate was weaker
// than the import gate.

package qbftengine

import (
	"strings"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/misc/eip1559"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/trie"
)

// TestVerifyCascadingFields_GasUsedExceedsGasLimit_Rejected is the reported
// PoC: GasUsed = GasLimit + 1 must now be rejected during header verification.
func TestVerifyCascadingFields_GasUsedExceedsGasLimit_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	expected := eip1559.CalcBaseFee(f.chain.Config(), f.parent)
	h := f.childHeader(t, expected, f.parent.GasLimit)
	h.GasUsed = h.GasLimit + 1

	err := f.verify(h)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("GasUsed > GasLimit must be rejected; got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid gasUsed") {
		t.Fatalf("expected an invalid gasUsed error, got %v", err)
	}
}

// TestVerifyCascadingFields_GasUsedEqualsGasLimit_Accepted pins the boundary:
// a fully-saturated block is legitimate and must still pass. Guards against a
// fix that mistakenly uses >= instead of >.
func TestVerifyCascadingFields_GasUsedEqualsGasLimit_Accepted(t *testing.T) {
	f := newEIP1559Fixture(t)
	h := f.childHeader(t, nil, f.parent.GasLimit)
	h.GasUsed = h.GasLimit
	// A saturated parent shifts the expected child BaseFee, so derive it after
	// setting GasUsed rather than reusing the empty-block value.
	h.BaseFee = eip1559.CalcBaseFee(f.chain.Config(), f.parent)

	err := f.verify(h)
	if err != nil && err != istanbulcommon.ErrEmptyCommittedSeals {
		t.Fatalf("GasUsed == GasLimit is legitimate and must pass; got %v", err)
	}
}

// TestVerifyBlockProposal_GasUsedExceedsGasLimit_Rejected exercises the entry
// point the report actually called, confirming the fix closes the pre-vote gate
// and not merely the internal helper.
func TestVerifyBlockProposal_GasUsedExceedsGasLimit_Rejected(t *testing.T) {
	f := newEIP1559Fixture(t)
	expected := eip1559.CalcBaseFee(f.chain.Config(), f.parent)
	h := f.childHeader(t, expected, f.parent.GasLimit)
	h.GasUsed = h.GasLimit + 1

	// VerifyBlockProposal resolves the parent through the chain reader rather
	// than an explicit parents slice, so the reader must serve the fixture's
	// parent header.
	chain := &gasUsedChainReader{mockChainHeaderReader: *f.chain, parent: f.parent}
	block := types.NewBlock(h, nil, nil, nil, trie.NewStackTrie(nil))

	if _, err := f.engine.VerifyBlockProposal(chain, block, f.valSet); err == nil {
		t.Fatal("VerifyBlockProposal accepted a header with GasUsed > GasLimit")
	} else if !strings.Contains(err.Error(), "invalid gasUsed") {
		t.Fatalf("expected an invalid gasUsed error, got %v", err)
	}
}

// gasUsedChainReader extends the shared mock with real parent lookup, which
// VerifyBlockProposal needs because it passes no parents slice.
type gasUsedChainReader struct {
	mockChainHeaderReader
	parent *types.Header
}

func (c *gasUsedChainReader) GetHeader(hash common.Hash, number uint64) *types.Header {
	if c.parent != nil && c.parent.Number.Uint64() == number && c.parent.Hash() == hash {
		return c.parent
	}
	return nil
}
