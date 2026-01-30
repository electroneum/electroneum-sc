package core

import (
	"errors"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	qbfttypes "github.com/electroneum/electroneum-sc/consensus/istanbul/types"
	"github.com/electroneum/electroneum-sc/log"
)

// Returns true if the `proposal` is justified by the set `roundChangeMessages` of ROUND-CHANGE messages
// and by the set `prepareMessages` of PREPARE messages.
// For this we must either have:
//   - a quorum of ROUND-CHANGE messages with preparedRound and preparedBlockDigest equal to nil; or
//   - a ROUND-CHANGE message (1) whose preparedRound is not nil and is equal or higher than the
//     preparedRound of `quorumSize` ROUND-CHANGE messages and (2) whose preparedRound and
//     preparedBlockDigest match the round and block of `quorumSize` PREPARE messages.
func isJustified(
	proposal istanbul.Proposal,
	roundChangeMessages []*qbfttypes.SignedRoundChangePayload,
	prepareMessages []*qbfttypes.Prepare,
	quorumSize int,
	validators istanbul.ValidatorSet) error {
	// Check the size of the set of ROUND-CHANGE messages
	if len(roundChangeMessages) < quorumSize {
		return errors.New("number of roundchange messages is less than required quorum of messages")
	}

	// Count round change messages with "bad proposal" reason
	var hasBadProposalCount uint = 0
	for _, rcm := range roundChangeMessages {
		if rcm.HasBadProposal {
			hasBadProposalCount++
		}
	}
	hasBadProposal := hasBadProposalCount >= uint(quorumSize)

	// If there are PREPARE messages, require a quorum of DISTINCT, VALIDATOR-SET signers,
	// and require round/digest consistency.
	var preparedRound *big.Int
	if len(prepareMessages) > 0 {
		seen := make(map[common.Address]struct{}, len(prepareMessages))

		preparedRound = prepareMessages[0].Round
		if preparedRound == nil {
			return errors.New("prepared message has nil round (preparedRound)")
		}

		for _, p := range prepareMessages {
			if p.Round == nil {
				return errors.New("prepared message has nil round")
			}

			// Must be same round and match proposal digest (unless bad-proposal quorum)
			if preparedRound.Cmp(p.Round) != 0 || (proposal.Hash() != p.Digest && !hasBadProposal) {
				return errors.New("prepared messages do not have same round or do not match proposal")
			}

			// Must have a verified source set ( verifysignatures() )
			src := p.Source()
			if src == (common.Address{}) {
				return errors.New("prepared message has empty source (signature not verified)")
			}

			// Must be from a validator
			if _, v := validators.GetByAddress(src); v == nil {
				return errors.New("prepared message signer is not in validator set")
			}

			seen[src] = struct{}{}
		}

		// Must reach quorum of distinct signers
		if len(seen) < quorumSize {
			return errors.New("number of distinct prepared message signers is less than required quorum")
		}
	}

	if preparedRound == nil {
		return hasQuorumOfRoundChangeMessagesForNil(roundChangeMessages, quorumSize)
	} else {
		return hasQuorumOfRoundChangeMessagesForPreparedRoundAndBlock(roundChangeMessages, preparedRound, proposal, quorumSize, hasBadProposal)
	}
}

// Checks whether a set of ROUND-CHANGE messages has `quorumSize` messages with nil prepared round and
// prepared block.
func hasQuorumOfRoundChangeMessagesForNil(roundChangeMessages []*qbfttypes.SignedRoundChangePayload, quorumSize int) error {
	nilCount := 0
	for _, m := range roundChangeMessages {
		log.Trace("IBFT: hasQuorumOfRoundChangeMessagesForNil", "rc", m)
		if (m.PreparedRound == nil || m.PreparedRound.Cmp(common.Big0) == 0) && common.EmptyHash(m.PreparedDigest) {
			nilCount++
			if nilCount == quorumSize {
				return nil
			}
		}
	}
	return errors.New("quorum of roundchange messages with nil prepared round not found")
}

// Checks whether a set of ROUND-CHANGE messages has some message with `preparedRound` and `preparedBlockDigest`,
// and has `quorumSize` messages with prepared round equal to nil or equal or lower than `preparedRound`.
func hasQuorumOfRoundChangeMessagesForPreparedRoundAndBlock(roundChangeMessages []*qbfttypes.SignedRoundChangePayload, preparedRound *big.Int, preparedBlock istanbul.Proposal, quorumSize int, hasQuorumOfBadProposal bool) error {
	lowerOrEqualRoundCount := 0
	hasMatchingMessage := false
	for _, m := range roundChangeMessages {
		log.Trace("IBFT: hasQuorumOfRoundChangeMessagesForPreparedRoundAndBlock", "rc", m)
		if m.PreparedRound == nil || m.PreparedRound.Cmp(preparedRound) <= 0 {
			lowerOrEqualRoundCount++
			if m.PreparedRound != nil && m.PreparedRound.Cmp(preparedRound) == 0 && (m.PreparedDigest == preparedBlock.Hash() || (m.PreparedDigest != preparedBlock.Hash() && hasQuorumOfBadProposal)) {
				hasMatchingMessage = true
			}
			if lowerOrEqualRoundCount >= quorumSize && hasMatchingMessage {
				return nil
			}
		}
	}

	return errors.New("quorum of roundchange messages with prepared round and proposal not found")
}

// Checks whether the round and block of a set of PREPARE messages of at least quorumSize match the
// preparedRound and preparedBlockDigest of a ROUND-CHANGE message, AND ensures quorum of DISTINCT validators.
func hasMatchingRoundChangeAndPrepares(
	roundChange *qbfttypes.RoundChange,
	prepareMessages []*qbfttypes.Prepare,
	quorumSize int,
	hasBadProposal bool,
	validators istanbul.ValidatorSet,
) error {
	if len(prepareMessages) < quorumSize {
		return errors.New("number of prepare messages is less than quorum of messages")
	}

	seen := make(map[common.Address]struct{}, len(prepareMessages))

	for _, p := range prepareMessages {
		if p == nil {
			return errors.New("prepare message is nil")
		}

		// Must match digest (unless bad-proposal quorum)
		if p.Digest != roundChange.PreparedDigest && !hasBadProposal {
			return errors.New("prepared message digest does not match roundchange prepared digest")
		}

		// Must match prepared round
		if p.Round == nil || roundChange.PreparedRound == nil {
			return errors.New("prepare/roundchange has nil round")
		}
		if p.Round.Cmp(roundChange.PreparedRound) != 0 {
			return errors.New("round number in prepared message does not match prepared round in roundchange")
		}

		// Must have a verified source set (i.e., VerifySignatures ran)
		src := p.Source()
		if src == (common.Address{}) {
			return errors.New("prepared message has empty source (signature not verified)")
		}

		// Must be from a validator
		if _, v := validators.GetByAddress(src); v == nil {
			return errors.New("prepared message signer is not in validator set")
		}

		// Must be DISTINCT signers
		if _, ok := seen[src]; ok {
			return errors.New("duplicate prepare signer in justification")
		}
		seen[src] = struct{}{}
	}

	// Must reach quorum of distinct signers
	if len(seen) < quorumSize {
		return errors.New("number of distinct prepared message signers is less than required quorum")
	}

	return nil
}
