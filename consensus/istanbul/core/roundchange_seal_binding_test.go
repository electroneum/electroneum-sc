package core

import (
	"bytes"
	"errors"
	"math/big"
	"sync"
	"testing"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/rlp"
)

// A ROUND-CHANGE's prepared block is tied to the signed digest by its hash
// alone, and the hash leaves out the proposer and committed seals. The first
// prepared block stored for a prepared round is the one the next proposer
// re-proposes, so a validator could send its own ROUND-CHANGE (with the real
// PREPARE quorum as justification) carrying a copy with broken seals ahead of
// everyone else, and every validator would reject the re-proposal. Doing it
// every round stalls the chain. Prepared blocks now have their seals checked
// before they're stored.

// sealCheckBackend records PRE-PREPAREs and lets the test decide which
// prepared blocks have valid seals.
type sealCheckBackend struct {
	viewBindingBackend
	verifySeals func(istanbul.Proposal) error
	preprepares []*qbfttypes.Preprepare
}

func (b *sealCheckBackend) VerifyProposalSeals(p istanbul.Proposal) error { return b.verifySeals(p) }

func (b *sealCheckBackend) Broadcast(_ istanbul.ValidatorSet, code uint64, payload []byte) error {
	if code == qbfttypes.PreprepareCode {
		m, err := qbfttypes.Decode(code, payload)
		if err != nil {
			return err
		}
		b.preprepares = append(b.preprepares, m.(*qbfttypes.Preprepare))
	}
	return nil
}

// withSeals re-encodes block's extra-data after mutate, keeping everything else.
func withSeals(t *testing.T, block *types.Block, mutate func(*types.QBFTExtra)) *types.Block {
	t.Helper()
	header := block.Header()
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		t.Fatal(err)
	}
	mutate(extra)
	if header.Extra, err = rlp.EncodeToBytes(extra); err != nil {
		t.Fatal(err)
	}
	return types.NewBlockWithHeader(header).WithBody(block.Transactions(), block.Uncles())
}

func extraOf(p istanbul.Proposal) []byte { return p.(*types.Block).Header().Extra }

func TestHandleRoundChange_RejectsPreparedBlockWithBrokenSeals(t *testing.T) {
	genuine := makeProposalBlock(t, 1)
	for name, mutate := range map[string]func(*types.QBFTExtra){
		"proposer seal": func(e *types.QBFTExtra) { e.ProposerSeal = bytes.Repeat([]byte{0xee}, types.IstanbulExtraSeal) },
		"committed seal": func(e *types.QBFTExtra) {
			e.CommittedSeal = [][]byte{bytes.Repeat([]byte{0xee}, types.IstanbulExtraSeal)}
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := withSeals(t, genuine, mutate)
			if tampered.Hash() != genuine.Hash() {
				t.Fatal("tampering with the seals changed the block hash")
			}
			c := newRoundChangeTestCore(4, 1, 1)
			backend := &sealCheckBackend{verifySeals: func(p istanbul.Proposal) error {
				if bytes.Equal(extraOf(p), extraOf(genuine)) {
					return nil
				}
				return errors.New("broken seals")
			}}
			c.backend = backend
			// We are the proposer; the last of the others is Byzantine.
			c.address = c.valSet.GetProposer().Address()
			backend.addr = c.address
			c.currentMutex = new(sync.Mutex)
			addrs := []common.Address{c.address}
			for _, v := range c.valSet.List() {
				if v.Address() != c.address {
					addrs = append(addrs, v.Address())
				}
			}
			prepares := makeAttackPrepares(addrs, c.QuorumSize(), 1, 0, genuine.Hash())
			roundChange := func(src common.Address, block *types.Block) *qbfttypes.RoundChange {
				rc := qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), block, false)
				rc.SetSource(src)
				rc.SetSignature([]byte{0x01})
				rc.Justification = prepares
				return rc
			}
			// The Byzantine validator gets its copy in before anyone else's.
			if err := c.handleRoundChange(roundChange(addrs[3], tampered)); !errors.Is(err, errInvalidPreparedBlock) {
				t.Errorf("tampered prepared block: have %v, want %v", err, errInvalidPreparedBlock)
			}
			if n := c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(1)); n != 0 {
				t.Errorf("rejected ROUND-CHANGE was counted: %d messages", n)
			}
			// Honest validators reach quorum and we re-propose.
			for _, src := range addrs[:c.QuorumSize()] {
				if err := c.handleRoundChange(roundChange(src, genuine)); err != nil {
					t.Fatalf("honest ROUND-CHANGE from %x: %v", src, err)
				}
			}
			if len(backend.preprepares) != 1 {
				t.Fatalf("expected one re-proposal, got %d", len(backend.preprepares))
			}
			if got := backend.preprepares[0].Proposal; !bytes.Equal(extraOf(got), extraOf(genuine)) {
				t.Fatalf("re-proposed the block with broken seals: extra %x", extraOf(got))
			}
		})
	}
}

// The seal check only runs on a prepared block its PREPAREs back, the only kind
// that gets stored and re-proposed. The sender picks the block, so checking
// anything else would let a validator make us build voting snapshots at any
// height it likes. The ROUND-CHANGE itself is still counted either way.
func TestHandleRoundChange_SealCheckOnlyForBackedPreparedBlocks(t *testing.T) {
	block := makeProposalBlock(t, 1)
	other := makeProposalBlock(t, 2)
	for name, tc := range map[string]struct {
		block   *types.Block
		digest  common.Hash
		short   bool // one PREPARE short of a quorum
		checked bool
	}{
		"no prepared block":          {},
		"prepares for another block": {block: block, digest: other.Hash()},
		"short of a quorum":          {block: block, digest: block.Hash(), short: true},
		"backed by a quorum":         {block: block, digest: block.Hash(), checked: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := newRoundChangeTestCore(4, 1, 1)
			calls := 0
			c.backend = &sealCheckBackend{verifySeals: func(istanbul.Proposal) error { calls++; return nil }}
			var addrs []common.Address
			for _, v := range c.valSet.List() {
				addrs = append(addrs, v.Address())
			}
			rc := makeRoundChange(1, 1, addrs[1])
			if tc.block != nil {
				rc = qbfttypes.NewRoundChange(big.NewInt(1), big.NewInt(1), big.NewInt(0), tc.block, false)
				rc.SetSource(addrs[1])
				rc.SetSignature([]byte{0x01})
				n := c.QuorumSize()
				if tc.short {
					n--
				}
				rc.Justification = makeAttackPrepares(addrs, n, 1, 0, tc.digest)
			}
			if err := c.handleRoundChange(rc); err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{false: 0, true: 1}[tc.checked]; calls != want {
				t.Errorf("seal check ran %d times, want %d", calls, want)
			}
			if n := c.roundChangeSet.getRCMessagesForGivenRound(big.NewInt(1)); n != 1 {
				t.Errorf("ROUND-CHANGE not counted: %d messages", n)
			}
			if _, stored := c.highestPrepared(big.NewInt(1)); (stored != nil) != tc.checked {
				t.Errorf("prepared block stored: %v, but seal check ran: %v", stored != nil, tc.checked)
			}
		})
	}
}
