package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	"github.com/electroneum/electroneum-sc/core/types"
)

// IsProposer and IsCurrentProposal are called by the p2p handler on its own
// goroutine while the consensus event loop records pending proposals,
// replaces the round state and recalculates the proposer. Run under -race:
// the two queries must not touch any of that live state.
func TestProposerQueriesDoNotRaceWithEventLoop(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	self := c.address

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the p2p handler
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			c.IsProposer()
			c.IsCurrentProposal(common.Hash{})
		}
	}()

	// The event loop's writes, in the order a real sequence makes them.
	for seq := int64(10); seq < 60; seq++ {
		block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(seq)})
		c.current.pendingRequest = &Request{Proposal: block} // handleRequest
		c.publishRoundState()

		view := &istanbul.View{Sequence: big.NewInt(seq + 1), Round: big.NewInt(0)}
		c.currentMutex.Lock() // startNewRound: a fresh round state for the new sequence
		c.current = newRoundState(view, c.valSet, nil, nil, nil, nil, noBadProposals)
		c.valSet.CalcProposer(self, 0)
		c.publishRoundStateLocked()
		c.currentMutex.Unlock()
	}
	close(stop)
	wg.Wait()
}

// The queries answer from the published snapshot: the pending proposal is
// recognised once published, and forgotten when a new sequence starts.
func TestIsCurrentProposalFollowsPublishedState(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	block := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(10)})

	if c.IsCurrentProposal(block.Hash()) {
		t.Fatal("IsCurrentProposal true before anything was published")
	}
	c.current.pendingRequest = &Request{Proposal: block}
	c.publishRoundState()
	if !c.IsCurrentProposal(block.Hash()) {
		t.Fatal("IsCurrentProposal false for the published pending proposal")
	}
	if c.IsCurrentProposal(common.Hash{0x01}) {
		t.Fatal("IsCurrentProposal true for a different block")
	}

	c.currentMutex.Lock()
	c.current = newRoundState(&istanbul.View{Sequence: big.NewInt(11), Round: big.NewInt(0)}, c.valSet, nil, nil, nil, nil, noBadProposals)
	c.publishRoundStateLocked()
	c.currentMutex.Unlock()
	if c.IsCurrentProposal(block.Hash()) {
		t.Fatal("IsCurrentProposal still true after a new sequence started")
	}
}

// IsProposer reflects the proposer in the published snapshot.
func TestIsProposerFollowsPublishedState(t *testing.T) {
	c := newRoundStateInfoTestCore(4, 10, 0)
	vals := c.valSet.List()

	if c.IsProposer() {
		t.Fatal("IsProposer true before anything was published")
	}
	// Round robin picks the validator after the last proposer, so making
	// the one before us the last proposer makes us the next.
	selfIdx, _ := c.valSet.GetByAddress(c.address)
	before := vals[(selfIdx+len(vals)-1)%len(vals)].Address()
	c.valSet.CalcProposer(before, 0)
	c.publishRoundState()
	if !c.IsProposer() {
		t.Fatalf("IsProposer false; proposer %s, self %s", c.valSet.GetProposer().Address().Hex(), c.address.Hex())
	}
	c.valSet.CalcProposer(c.address, 0)
	c.publishRoundState()
	if c.IsProposer() {
		t.Fatal("IsProposer true after the proposer moved on")
	}
}

func noBadProposals(common.Hash) bool { return false }
