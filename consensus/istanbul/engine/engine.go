package qbftengine

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/consensus/misc/eip1559"
	"github.com/electroneum/electroneum-sc/core/state"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
	"golang.org/x/crypto/sha3"
)

var (
	nilUncleHash = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
)

type SignerFn func(data []byte) ([]byte, error)

type Engine struct {
	cfg *istanbul.Config

	signer common.Address // Ethereum address of the signing key
	sign   SignerFn       // Signer function to authorize hashes with
}

func NewEngine(cfg *istanbul.Config, signer common.Address, sign SignerFn) *Engine {
	return &Engine{
		cfg:    cfg,
		signer: signer,
		sign:   sign,
	}
}

// Author returns the address of the block's proposer.
//
// Post-FutureFork: recovers the proposer address cryptographically from the
// ProposerSeal embedded in QBFTExtra. This makes Author() trustworthy for all
// callers including snapshot voting, API queries, and accountability logic.
//
// Pre-FutureFork: returns header.Coinbase on trust. Proposer authenticity is
// enforced at the consensus message layer (handlePreprepareMsg binds Coinbase
// to the cryptographically verified PRE-PREPARE source).
func (e *Engine) Author(header *types.Header) (common.Address, error) {
	if extra, err := types.ExtractQBFTExtra(header); err == nil && len(extra.ProposerSeal) == types.IstanbulExtraSeal {
		return istanbul.GetSignatureAddress(sigHash(header).Bytes(), extra.ProposerSeal)
	}
	return header.Coinbase, nil
}

func (e *Engine) CommitHeader(header *types.Header, seals [][]byte, round *big.Int) error {
	return ApplyHeaderQBFTExtra(
		header,
		writeCommittedSeals(seals),
		writeRoundNumber(round),
	)
}

// writeCommittedSeals writes the extra-data field of a block header with given committed seals.
func writeCommittedSeals(committedSeals [][]byte) ApplyQBFTExtra {
	return func(qbftExtra *types.QBFTExtra) error {
		if len(committedSeals) == 0 {
			return istanbulcommon.ErrInvalidCommittedSeals
		}

		for _, seal := range committedSeals {
			if len(seal) != types.IstanbulExtraSeal {
				return istanbulcommon.ErrInvalidCommittedSeals
			}
		}

		qbftExtra.CommittedSeal = make([][]byte, len(committedSeals))
		copy(qbftExtra.CommittedSeal, committedSeals)

		return nil
	}
}

// writeProposerSeal writes the proposer's signature into the extra-data field.
func writeProposerSeal(seal []byte) ApplyQBFTExtra {
	return func(qbftExtra *types.QBFTExtra) error {
		if len(seal) != types.IstanbulExtraSeal {
			return istanbulcommon.ErrInvalidSignature
		}
		qbftExtra.ProposerSeal = make([]byte, len(seal))
		copy(qbftExtra.ProposerSeal, seal)
		return nil
	}
}

// writeRoundNumber writes the extra-data field of a block header with given round.
func writeRoundNumber(round *big.Int) ApplyQBFTExtra {
	return func(qbftExtra *types.QBFTExtra) error {
		qbftExtra.Round = uint32(round.Uint64())
		return nil
	}
}

func (e *Engine) VerifyBlockProposal(chain consensus.ChainHeaderReader, block *types.Block, validators istanbul.ValidatorSet) (time.Duration, error) {
	// check block body
	txnHash := types.DeriveSha(block.Transactions(), trie.NewStackTrie(nil))
	if txnHash != block.Header().TxHash {
		return 0, istanbulcommon.ErrMismatchTxhashes
	}

	uncleHash := types.CalcUncleHash(block.Uncles())
	if uncleHash != nilUncleHash {
		return 0, istanbulcommon.ErrInvalidUncleHash
	}

	// verify the header of proposed block
	err := e.VerifyHeader(chain, block.Header(), nil, validators)
	if err == nil || err == istanbulcommon.ErrEmptyCommittedSeals {
		// ignore errEmptyCommittedSeals error because we don't have the committed seals yet
		return 0, nil
	} else if err == consensus.ErrFutureBlock {
		return time.Until(time.Unix(int64(block.Header().Time), 0)), consensus.ErrFutureBlock
	}

	return 0, err
}

func (e *Engine) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators istanbul.ValidatorSet) error {
	return e.verifyHeader(chain, header, parents, validators)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (e *Engine) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators istanbul.ValidatorSet) error {
	if header.Number == nil {
		return istanbulcommon.ErrUnknownBlock
	}

	// Don't waste time checking blocks from the future (adjusting for allowed threshold)
	adjustedTimeNow := time.Now().Add(time.Duration(e.cfg.AllowedFutureBlockTime) * time.Second).Unix()
	if header.Time > uint64(adjustedTimeNow) {
		return consensus.ErrFutureBlock
	}

	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		return istanbulcommon.ErrInvalidExtraDataFormat
	}

	// Reject malformed validator votes here, while validating this block, rather
	// than only later when this header is applied to a voting snapshot. See
	// validateVote for why deferring it halts the chain.
	//
	// Deliberately not fork-gated: this closes a liveness hole that is live on
	// every current network, and it cannot break historical blocks because the
	// only writer of this field, WriteVote, emits nothing but 0x00 and 0xff.
	if err := validateVote(extra.Vote); err != nil {
		return err
	}

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != types.IstanbulDigest {
		return istanbulcommon.ErrInvalidMixDigest
	}

	// Ensure that the block doesn't contain any uncles which are meaningless in Istanbul
	if header.UncleHash != nilUncleHash {
		return istanbulcommon.ErrInvalidUncleHash
	}

	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if header.Difficulty == nil || header.Difficulty.Cmp(istanbulcommon.DefaultDifficulty) != 0 {
		return istanbulcommon.ErrInvalidDifficulty
	}

	return e.verifyCascadingFields(chain, header, validators, parents)
}

func (e *Engine) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool, validators istanbul.ValidatorSet) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		errored := false
		for i, header := range headers {
			var err error
			if errored {
				err = consensus.ErrUnknownAncestor
			} else {
				err = e.verifyHeader(chain, header, headers[:i], validators)
			}

			if err != nil {
				errored = true
			}

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (e *Engine) verifyCascadingFields(chain consensus.ChainHeaderReader, header *types.Header, validators istanbul.ValidatorSet, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}

	// Check parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}

	// Ensure that the block's parent has right number and hash
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}

	// Ensure that the block's timestamp isn't too close to it's parent
	if parent.Time+e.cfg.GetConfig(parent.Number).BlockPeriod > header.Time {
		return istanbulcommon.ErrInvalidTimestamp
	}

	// Verify the absolute gas limit cap, matching the check in
	// beacon/clique/ethash. params.MaxGasLimit is 2^63-1 so this is
	// effectively unreachable in practice, but kept for parity with the
	// other engines and to reject pathological headers up front.
	if header.GasLimit > params.MaxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, params.MaxGasLimit)
	}

	// A block cannot consume more gas than its own limit. core.ApplyTransaction
	// meters execution against a GasPool seeded from header.GasLimit, so any
	// header claiming more than that describes an execution that cannot have
	// happened. Import already rejects it in ValidateState, but only after the
	// block has been executed; QBFT calls this during proposal verification,
	// before honest validators sign PREPARE/COMMIT, so checking here closes the
	// gap between the pre-vote gate and the import gate.
	//
	// Deliberately not fork-gated: no header that ever executed can fail this,
	// so it cannot invalidate historical blocks.
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

	// Verify EIP-1559 header fields (BaseFee correctness, BaseFee presence,
	// and the per-block ±1/1024 GasLimit bound via misc.VerifyGaslimit).
	// Without this, a malicious proposer can ship blocks with an arbitrary
	// BaseFee (skipping the EIP-1559 burn), nil BaseFee (panic during state
	// transition), or an oversized GasLimit (block-size DoS). This matches
	// beacon/clique/ethash, which all call VerifyEip1559Header from their
	// equivalent of verifyCascadingFields.
	//
	// The chain reader is allowed to be nil when callers (typically tests)
	// supply parents directly — mirroring the existing guard above that uses
	// parents to avoid chain.GetHeader. In production every call site
	// (blockchain insert, headerchain, fetchers, backend.Verify) passes a
	// real ChainHeaderReader, so this nil-skip path is unreachable on a real
	// node and does not weaken consensus validation.
	if chain != nil {
		cfg := chain.Config()
		if cfg.IsLondon(header.Number) {
			if err := eip1559.VerifyEIP1559Header(cfg, parent, header); err != nil {
				return err
			}
		}
	}

	// Verify signer
	if err := e.verifySigner(chain, header, parents, validators); err != nil {
		return err
	}

	return e.verifyCommittedSeals(chain, header, parents, validators)
}

func (e *Engine) verifySigner(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators istanbul.ValidatorSet) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return istanbulcommon.ErrUnknownBlock
	}

	// After FutureFork: cryptographically verify the proposer seal.
	// Before FutureFork (or when chain is nil in tests): fall back to
	// validator-set membership check only.
	if chain != nil && chain.Config().IsFutureFork(header.Number) {
		extra, err := types.ExtractQBFTExtra(header)
		if err != nil {
			return err
		}
		if len(extra.ProposerSeal) != types.IstanbulExtraSeal {
			return istanbulcommon.ErrInvalidSignature
		}
		signer, err := istanbul.GetSignatureAddress(sigHash(header).Bytes(), extra.ProposerSeal)
		if err != nil {
			return err
		}
		if signer != header.Coinbase {
			return istanbulcommon.ErrInvalidCoinbase
		}
		if _, v := validators.GetByAddress(signer); v == nil {
			return istanbulcommon.ErrUnauthorized
		}
		return nil
	}

	// Pre-fork: Coinbase is not cryptographically bound to the proposer.
	// Proposer authenticity is enforced at the consensus message layer
	// (handlePreprepareMsg). Here we can only verify validator-set membership.
	//
	// Reject any non-empty ProposerSeal pre-fork. Honest nodes never write a
	// ProposerSeal before FutureFork (Seal() gates it on IsFutureFork), so a
	// present seal can only come from a Byzantine proposer. Without this check
	// the field is silently accepted here but later fails in Author(), which
	// unconditionally recovers from any 65-byte ProposerSeal — an inconsistent
	// validation path that lets a poisoned proposal be finalized and then
	// breaks snapshot construction over that header.
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		return err
	}
	if len(extra.ProposerSeal) != 0 {
		return istanbulcommon.ErrInvalidSignature
	}

	if _, v := validators.GetByAddress(header.Coinbase); v == nil {
		return istanbulcommon.ErrUnauthorized
	}

	return nil
}

// verifyCommittedSeals checks whether every committed seal is signed by one of the parent's validators
func (e *Engine) verifyCommittedSeals(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header, validators istanbul.ValidatorSet) error {
	number := header.Number.Uint64()

	if number == 0 {
		// We don't need to verify committed seals in the genesis block
		return nil
	}

	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		return err
	}
	committedSeal := extra.CommittedSeal

	// The length of Committed seals should be larger than 0
	if len(committedSeal) == 0 {
		return istanbulcommon.ErrEmptyCommittedSeals
	}

	// A header can never legitimately carry more committed seals than there are
	// validators: each seal is one validator's COMMIT, RemoveValidator makes each
	// address usable once, and finalization needs only 2F+1 of N. Reject oversized
	// arrays here, before Signers() pays one ecrecover per entry. Without this cap
	// an unauthenticated peer can stuff the array (bounded only by the 100 KB
	// Header.SanityCheck limit, ~1500 seals) with well-formed throwaway signatures
	// and force that many recoveries on every verifying node before the membership
	// loop below rejects the header. This mirrors the justification-length cap
	// already applied to the consensus message path.
	//
	// Deliberately not fork-gated: no honest header ever carried more than N seals,
	// so this cannot invalidate any historical block.
	if len(committedSeal) > validators.Size() {
		return istanbulcommon.ErrInvalidCommittedSeals
	}

	validatorsCpy := validators.Copy()

	// Check whether the committed seals are generated by validators
	validSeal := 0
	committers, err := e.Signers(header)
	if err != nil {
		return err
	}

	for _, addr := range committers {
		if validatorsCpy.RemoveValidator(addr) {
			validSeal++
			continue
		}
		return istanbulcommon.ErrInvalidCommittedSeals
	}

	// IBFT/QBFT finalization requires at least 2F+1 valid committed seals
	requiredSeals := int(math.Ceil(float64(2*validators.Size()) / 3))
	if validSeal < requiredSeals {
		return istanbulcommon.ErrInvalidCommittedSeals
	}
	return nil
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (e *Engine) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return istanbulcommon.ErrInvalidUncleHash
	}
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (e *Engine) VerifySeal(chain consensus.ChainHeaderReader, header *types.Header, validators istanbul.ValidatorSet) error {
	// get parent header and ensure the signer is in parent's validator set
	number := header.Number.Uint64()
	if number == 0 {
		return istanbulcommon.ErrUnknownBlock
	}

	// ensure that the difficulty equals to istanbulcommon.DefaultDifficulty
	if header.Difficulty.Cmp(istanbulcommon.DefaultDifficulty) != 0 {
		return istanbulcommon.ErrInvalidDifficulty
	}

	return e.verifySigner(chain, header, nil, validators)
}

func (e *Engine) Prepare(chain consensus.ChainHeaderReader, header *types.Header, validators istanbul.ValidatorSet) error {
	header.Nonce = istanbulcommon.EmptyBlockNonce
	header.MixDigest = types.IstanbulDigest

	// copy the parent extra data as the header extra data
	number := header.Number.Uint64()

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// use the same difficulty for all blocks
	header.Difficulty = istanbulcommon.DefaultDifficulty

	// set header's timestamp
	header.Time = parent.Time + e.cfg.GetConfig(header.Number).BlockPeriod
	if header.Time < uint64(time.Now().Unix()) {
		header.Time = uint64(time.Now().Unix())
	}

	// add validators in snapshot to extraData's validators section
	return ApplyHeaderQBFTExtra(
		header,
		WriteValidators(validator.SortedAddresses(validators.List())),
	)
}

func WriteValidators(validators []common.Address) ApplyQBFTExtra {
	return func(qbftExtra *types.QBFTExtra) error {
		qbftExtra.Validators = validators
		return nil
	}
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (e *Engine) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, withdrawals []*types.Withdrawal) {
	// No block rewards in Istanbul, so the state remains as is and uncles are dropped
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = nilUncleHash
}

// FinalizeAndAssemble implements consensus.Engine, ensuring no uncles are set,
// nor block rewards given, and returns the final block.
func (e *Engine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, withdrawals []*types.Withdrawal) (*types.Block, error) {
	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts, trie.NewStackTrie(nil)), nil
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (e *Engine) Seal(chain consensus.ChainHeaderReader, block *types.Block, validators istanbul.ValidatorSet) (*types.Block, error) {
	if _, v := validators.GetByAddress(e.signer); v == nil {
		return block, istanbulcommon.ErrUnauthorized
	}

	header := block.Header()
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return block, consensus.ErrUnknownAncestor
	}

	// Set Coinbase
	header.Coinbase = e.signer

	// After FutureFork: sign the header and embed the proposer seal.
	// This provides cryptographic proof of who produced the block.
	if chain.Config().IsFutureFork(header.Number) {
		seal, err := e.sign(sigHash(header).Bytes())
		if err != nil {
			return block, err
		}
		if err := ApplyHeaderQBFTExtra(header, writeProposerSeal(seal)); err != nil {
			return block, err
		}
	}

	return block.WithSeal(header), nil
}

func (e *Engine) SealHash(header *types.Header) common.Hash {
	header.Coinbase = e.signer
	return sigHash(header)
}

func (e *Engine) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return istanbulcommon.DefaultDifficulty
}

func (e *Engine) Validators(header *types.Header) ([]common.Address, error) {
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		return nil, err
	}

	return extra.Validators, nil
}

func (e *Engine) Signers(header *types.Header) ([]common.Address, error) {
	extra, err := types.ExtractQBFTExtra(header)
	if err != nil {
		return []common.Address{}, err
	}
	committedSeal := extra.CommittedSeal
	proposalSeal := PrepareCommittedSeal(header, extra.Round)

	// Defense-in-depth cap against committed-seal amplification. The network
	// verification path is bounded by verifyCommittedSeals against the trusted
	// parent validator set before it reaches here; this bound protects the other
	// callers (the debug_* JSON-RPC signer APIs), which run over already-imported
	// headers whose embedded validator set is trustworthy. A header can never
	// carry more committed seals than its own validator set, so anything beyond
	// that is malformed and must not cost one ecrecover per entry.
	if len(committedSeal) > len(extra.Validators) {
		return nil, istanbulcommon.ErrInvalidCommittedSeals
	}

	var addrs []common.Address
	// 1. Get committed seals from current header
	for _, seal := range committedSeal {
		// 2. Get the original address by seal and parent block hash
		addr, err := istanbul.GetSignatureAddressNoHashing(proposalSeal, seal)
		if err != nil {
			return nil, istanbulcommon.ErrInvalidSignature
		}
		addrs = append(addrs, addr)
	}

	return addrs, nil
}

func (e *Engine) Address() common.Address {
	return e.signer
}

// FIXME: Need to update this for Istanbul
// sigHash returns the hash which is used as input for the Istanbul
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	rlp.Encode(hasher, types.QBFTFilteredHeader(header))
	hasher.Sum(hash[:0])
	return hash
}

// PrepareCommittedSeal returns a committed seal for the given hash
func PrepareCommittedSeal(header *types.Header, round uint32) []byte {
	h := types.CopyHeader(header)
	return h.QBFTHashWithRoundNumber(round).Bytes()
}

func (e *Engine) WriteVote(header *types.Header, candidate common.Address, authorize bool) error {
	return ApplyHeaderQBFTExtra(
		header,
		WriteVote(candidate, authorize),
	)
}

func WriteVote(candidate common.Address, authorize bool) ApplyQBFTExtra {
	return func(qbftExtra *types.QBFTExtra) error {
		voteType := types.QBFTDropVote
		if authorize {
			voteType = types.QBFTAuthVote
		}

		vote := &types.ValidatorVote{RecipientAddress: candidate, VoteType: voteType}
		qbftExtra.Vote = vote
		return nil
	}
}

// validateVote checks that a validator vote carries one of the two permitted
// vote types, QBFTAuthVote (0xff) or QBFTDropVote (0x00). A nil vote is valid
// and means "no vote in this block": Prepare only writes a vote when the node
// has a pending candidate, so honest headers routinely carry none.
//
// This is the single source of truth for vote-type validity, shared by the
// header verification path (verifyHeader, run while validating the block
// itself) and the snapshot path (ReadVote, run when applying an already
// accepted header). Both must agree. Before this was shared, only ReadVote
// checked the vote, and it runs over headers up to the parent -- never over the
// block being verified. A malformed vote therefore passed proposal
// verification, committed-seal finalization and full block import, became the
// canonical head, and only failed afterwards, when every node needed the voting
// snapshot over that header to produce or verify the next block. That is a
// chain halt: nodes could neither build on the head nor verify a successor, and
// a fresh sync stalled at the same height.
//
// The vote is covered by the proposer seal and the committed seals
// (QBFTFilteredHeaderWithRound strips only CommittedSeal, ProposerSeal and
// Round from the hash), so validators genuinely sign over this field and
// rejecting a bad value cannot be used to invalidate an otherwise honest block.
func validateVote(vote *types.ValidatorVote) error {
	if vote == nil {
		return nil
	}
	if vote.VoteType != types.QBFTAuthVote && vote.VoteType != types.QBFTDropVote {
		return istanbulcommon.ErrInvalidVote
	}
	return nil
}

func (e *Engine) ReadVote(header *types.Header) (candidate common.Address, authorize bool, err error) {
	qbftExtra, err := getExtra(header)
	if err != nil {
		return common.Address{}, false, err
	}

	var vote *types.ValidatorVote
	if qbftExtra.Vote == nil {
		vote = &types.ValidatorVote{RecipientAddress: common.Address{}, VoteType: types.QBFTDropVote}
	} else {
		vote = qbftExtra.Vote
	}

	if err := validateVote(vote); err != nil {
		return common.Address{}, false, err
	}

	return vote.RecipientAddress, vote.VoteType == types.QBFTAuthVote, nil
}

func getExtra(header *types.Header) (*types.QBFTExtra, error) {
	// A sub-32-byte (IstanbulExtraVanity) Extra means the QBFT extra-data has not
	// been written yet. This is the normal state of a freshly-built header in
	// Prepare/Seal/CommitHeader, whose Extra is empty until we add the validators,
	// seals, etc., so bootstrap an empty QBFTExtra with padded vanity for the
	// write path to populate.
	//
	// Keep this lenient: turning it into an error would make Prepare fail and halt
	// block production. It is not a validation gap on the read side either -- a
	// header reaching ReadVote/snapApplyHeader is an already-finalized block that
	// always carries a full Extra (verifyHeader rejects undecodable extra-data and
	// verifyCommittedSeals requires a 2F+1 committed-seal quorum), so this branch
	// is never taken for such headers.
	if len(header.Extra) < types.IstanbulExtraVanity {
		vanity := append(header.Extra, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(header.Extra))...)
		return &types.QBFTExtra{
			VanityData:    vanity,
			Validators:    []common.Address{},
			CommittedSeal: [][]byte{},
			Round:         0,
			Vote:          nil,
		}, nil
	}

	// This is the case when Extra has already been set
	return types.ExtractQBFTExtra(header)
}

func setExtra(h *types.Header, qbftExtra *types.QBFTExtra) error {
	payload, err := rlp.EncodeToBytes(qbftExtra)
	if err != nil {
		return err
	}

	h.Extra = payload
	return nil
}
