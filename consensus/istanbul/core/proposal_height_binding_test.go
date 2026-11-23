package core

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/prque"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
)

// Nothing used to check that a PRE-PREPARE's block is for the sequence the
// message is for. Verify only checks a block against its own parent, so a
// proposer at sequence N+1 could propose a second block N on top of N-1 and get
// it prepared and committed. It imports at the same total difficulty as the
// real block N and the fork choice coin flip can swap it in, undoing a block
// everyone treated as final. A block that's prepared but can't be re-proposed
// locks the validators on it instead.

// heightBackend counts how often a proposal reaches Verify.
type heightBackend struct {
	viewBindingBackend
	verifies int
}

func (b *heightBackend) Verify(istanbul.Proposal) (time.Duration, error) {
	b.verifies++
	return 0, nil
}

// buildHeightCore returns a core at round 0 of seq that isn't the proposer,
// along with the proposer and everyone's keys.
func buildHeightCore(t *testing.T, seq int64) (*core, *heightBackend, common.Address, map[common.Address]*ecdsa.PrivateKey) {
	t.Helper()
	keys := make(map[common.Address]*ecdsa.PrivateKey, 4)
	var addrs []common.Address
	for i := 0; i < 4; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		addr := crypto.PubkeyToAddress(key.PublicKey)
		keys[addr] = key
		addrs = append(addrs, addr)
	}
	policy := istanbul.NewRoundRobinProposerPolicy()
	policy.Use(istanbul.ValidatorSortByByte())
	valSet := validator.NewSet(addrs, policy)
	valSet.CalcProposer(common.Address{}, 0)
	proposer := valSet.GetProposer().Address()

	backend := new(heightBackend)
	for _, addr := range addrs {
		if addr != proposer {
			backend.addr = addr
			break
		}
	}
	view := &istanbul.View{Sequence: big.NewInt(seq), Round: big.NewInt(0)}
	c := &core{
		address:     backend.addr,
		state:       StateAcceptRequest,
		logger:      log.New(),
		cleanLogger: log.New(),
		backend:     backend,
		valSet:      valSet,
		validateFn: func(data []byte, sig []byte) (common.Address, error) {
			return istanbul.CheckValidatorSignature(valSet, data, sig)
		},
		backlogs:          make(map[common.Address]*prque.Prque),
		backlogsMu:        new(sync.Mutex),
		currentMutex:      new(sync.Mutex),
		current:           newRoundState(view, valSet, nil, nil, nil, nil, func(common.Hash) bool { return false }),
		roundChangeSet:    newRoundChangeSet(valSet),
		pendingRequests:   prque.New(nil),
		pendingRequestsMu: new(sync.Mutex),
	}
	return c, backend, proposer, keys
}

func TestHandleEncodedMsg_RejectsProposalForAnotherHeight(t *testing.T) {
	const seq = 5
	for name, tc := range map[string]struct {
		number int64
		want   error
	}{
		"same height as the last block": {number: seq - 1, want: errInvalidProposalNumber},
		"an older height":               {number: seq - 3, want: errInvalidProposalNumber},
		"a later height":                {number: seq + 1, want: errInvalidProposalNumber},
		"this sequence":                 {number: seq},
	} {
		t.Run(name, func(t *testing.T) {
			c, backend, proposer, keys := buildHeightCore(t, seq)

			// The real proposer, with its own coinbase, so every other check passes.
			block := blockWithCoinbase(tc.number, 2, proposer)
			preprepare := qbfttypes.NewPreprepare(big.NewInt(seq), big.NewInt(0), block)
			signQBFTMessage(t, preprepare, keys[proposer])
			payload, err := encodePreprepare(t, preprepare)
			if err != nil {
				t.Fatal(err)
			}

			err = c.handleEncodedMsg(qbfttypes.PreprepareCode, payload)
			if !errors.Is(err, tc.want) {
				t.Fatalf("have %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				if c.state != StatePreprepared || backend.broadcasts != 1 {
					t.Fatalf("proposal for this sequence not accepted: state %v, %d broadcasts", c.state, backend.broadcasts)
				}
				return
			}
			if c.state != StateAcceptRequest || c.current.Proposal() != nil {
				t.Errorf("accepted a block at height %d for sequence %d", tc.number, seq)
			}
			if backend.broadcasts != 0 {
				t.Errorf("sent a PREPARE for a block at height %d", tc.number)
			}
			// Turned away before Verify, which is what let it through.
			if backend.verifies != 0 {
				t.Errorf("block at height %d reached Verify", tc.number)
			}
		})
	}
}
