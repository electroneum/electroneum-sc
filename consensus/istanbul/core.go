package istanbul

import "github.com/electroneum/electroneum-sc/common"

type Core interface {
	Start() error
	Stop() error
	IsProposer() bool

	// RoundState returns the latest snapshot of this node's consensus state,
	// or nil if the consensus event loop has not published one yet.
	RoundState() *RoundStateInfo

	// verify if a hash is the same as the proposed block in the current pending request
	//
	// this is useful when the engine is currently the proposer
	//
	// pending request is populated right at the preprepare stage so this would give us the earliest verification
	// to avoid any race condition of coming propagated blocks
	IsCurrentProposal(blockHash common.Hash) bool
}

// RoundStateInfo is a read-only snapshot of a validator's QBFT consensus
// state. The consensus event loop publishes a fresh one after every event it
// handles, so reading it never touches live consensus state.
//
// It exists for operations: when the chain stalls, comparing snapshots across
// validators shows which block and round each one is on, how far each got in
// the current round, and which rounds the others are asking to move to -
// enough to tell a network partition from a round-change desync.
type RoundStateInfo struct {
	// Sequence is the block number this node is trying to agree on.
	Sequence uint64 `json:"sequence"`
	// Round within that sequence; it increases on every round change.
	Round uint64 `json:"round"`
	// State is the step reached in this round: "Accept request",
	// "Preprepared", "Prepared" or "Committed".
	State string `json:"state"`
	// Proposer expected to propose in this round, and whether that is us.
	Proposer   common.Address `json:"proposer"`
	IsProposer bool           `json:"isProposer"`
	// HasProposal is true once this round's PRE-PREPARE has been accepted.
	HasProposal bool `json:"hasProposal"`
	// Validators is the size of the current validator set; QuorumSize is how
	// many matching messages a step needs.
	Validators int `json:"validators"`
	QuorumSize int `json:"quorumSize"`
	// Prepares and Commits received for the current round.
	Prepares int `json:"prepares"`
	Commits  int `json:"commits"`
	// RoundChanges counts the ROUND-CHANGE messages held for each target
	// round at or above the current one (rounds with none are left out).
	// Messages for a higher round than ours mean other validators have moved
	// on without us.
	RoundChanges map[uint64]int `json:"roundChanges"`
	// PreparedRound is the round in which this node became prepared on a
	// block it must re-propose (a lock), or nil if it holds none.
	PreparedRound *uint64 `json:"preparedRound"`
	// RoundStartedAt is when the current sequence and round began and
	// UpdatedAt when this snapshot was taken, both in unix seconds.
	RoundStartedAt uint64 `json:"roundStartedAt"`
	UpdatedAt      uint64 `json:"updatedAt"`
}
