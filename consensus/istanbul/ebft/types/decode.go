package ebfttypes

import (
	istanbulcommon "github.com/electroneum/electroneum-sc/consensus/istanbul/common"
	"github.com/electroneum/electroneum-sc/rlp"
)

func Decode(code uint64, data []byte) (EBFTMessage, error) {
	switch code {
	case PreprepareCode:
		var preprepare Preprepare
		if err := rlp.DecodeBytes(data, &preprepare); err != nil {
			return nil, istanbulcommon.ErrFailedDecodePreprepare
		}
		preprepare.code = PreprepareCode
		return &preprepare, nil
	case PrepareCode:
		var prepare Prepare
		if err := rlp.DecodeBytes(data, &prepare); err != nil {
			return nil, istanbulcommon.ErrFailedDecodeCommit
		}
		prepare.code = PrepareCode
		return &prepare, nil
	case CommitCode:
		var commit Commit
		if err := rlp.DecodeBytes(data, &commit); err != nil {
			return nil, istanbulcommon.ErrFailedDecodeCommit
		}
		commit.code = CommitCode
		return &commit, nil
	case RoundChangeCode:
		var roundChange RoundChange
		if err := rlp.DecodeBytes(data, &roundChange); err != nil {
			return nil, istanbulcommon.ErrFailedDecodeRoundChange
		}
		roundChange.code = RoundChangeCode
		return &roundChange, nil
	}

	return nil, istanbulcommon.ErrInvalidMessage
}
