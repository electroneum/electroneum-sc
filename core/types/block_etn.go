// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package types

import (
	"fmt"

	"github.com/electroneum/electroneum-sc/common"
)

// String restores the method upstream go-ethereum removed after v1.10.18.
//
// It is not cosmetic here: consensus/istanbul's Proposal interface requires
// String(), and *Block is the only concrete Proposal on this chain. Kept in its
// own file so the ETN delta stays out of upstream's block.go.
func (b *Block) String() string {
	return fmt.Sprintf("{Header: %v}", b.header)
}

// QBFTHashWithRoundNumber gets the hash of the Header with only the commit seal
// set to its null value, at a given round.
func (h *Header) QBFTHashWithRoundNumber(round uint32) common.Hash {
	return rlpHash(QBFTFilteredHeaderWithRound(h, round))
}
