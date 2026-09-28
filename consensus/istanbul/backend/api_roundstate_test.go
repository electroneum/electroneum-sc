package backend

import (
	"errors"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	"github.com/electroneum/electroneum-sc/rpc"
)

// istanbul_getRoundState, called over RPC against a running single-validator
// engine, returns the snapshot published by the consensus event loop.
func TestAPIGetRoundState_OverRPC(t *testing.T) {
	chain, engine := newBlockChain(1)
	defer engine.Stop()

	server := rpc.NewServer()
	defer server.Stop()
	for _, api := range engine.APIs(chain) {
		if err := server.RegisterName(api.Namespace, api.Service); err != nil {
			t.Fatalf("registering %s API: %v", api.Namespace, err)
		}
	}
	client := rpc.DialInProc(server)
	defer client.Close()

	// The engine publishes its first snapshot when it starts its first round;
	// allow the event loop a moment.
	var got istanbul.RoundStateInfo
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := client.Call(&got, "istanbul_getRoundState")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("istanbul_getRoundState: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	head := chain.CurrentBlock().NumberU64()
	if got.Sequence != head+1 {
		t.Errorf("sequence = %d, want the next block %d", got.Sequence, head+1)
	}
	if got.Validators != 1 || got.QuorumSize != 1 {
		t.Errorf("validators/quorum = %d/%d, want 1/1", got.Validators, got.QuorumSize)
	}
	if got.Proposer != engine.Address() || !got.IsProposer {
		t.Errorf("proposer = %s (isProposer %v), want this node %s", got.Proposer.Hex(), got.IsProposer, engine.Address().Hex())
	}
	if got.State == "" || got.RoundStartedAt == 0 || got.UpdatedAt == 0 {
		t.Errorf("snapshot not filled in: %+v", got)
	}
	if got.RoundChanges == nil {
		t.Error("roundChanges should be an empty map, not null")
	}
}

// With the consensus engine stopped (e.g. a node that is not running as a
// validator) the call reports that rather than returning stale data.
func TestAPIGetRoundState_EngineStopped(t *testing.T) {
	chain, engine := newBlockChain(1)
	if err := engine.Stop(); err != nil {
		t.Fatalf("stopping engine: %v", err)
	}

	api := &API{chain: chain, backend: engine}
	info, err := api.GetRoundState()
	if info != nil || !errors.Is(err, istanbul.ErrStoppedEngine) {
		t.Fatalf("GetRoundState on a stopped engine = (%+v, %v), want (nil, %v)", info, err, istanbul.ErrStoppedEngine)
	}
}
