// Copyright 2026 The electroneum-sc Authors
// This file is part of the electroneum-sc library.
//
// The electroneum-sc library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The electroneum-sc library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the electroneum-sc library. If not, see <http://www.gnu.org/licenses/>.

package filters

import (
	"errors"
	"fmt"

	"github.com/electroneum/electroneum-sc/common"
)

var (
	// ErrExceedLogQueryLimit is returned when a log filter names more addresses,
	// or more topics at a single position, than the node is configured to accept.
	// Each entry costs a bloom-filter clause on every block in the range, so an
	// unbounded list lets one unauthenticated request occupy an RPC worker for an
	// arbitrarily long time.
	ErrExceedLogQueryLimit = errors.New("exceed maximum log query limit")

	// ErrExceedRangeLimit is returned when a log filter spans more blocks than
	// the node is configured to serve in a single query.
	ErrExceedRangeLimit = errors.New("exceed maximum block range")
)

// CheckLogQueryLimit applies this system's configured address/topic cap. Both
// the JSON-RPC filter API and the GraphQL resolver go through here, so the cap
// cannot be sidestepped by picking a different front end.
func (sys *FilterSystem) CheckLogQueryLimit(addresses []common.Address, topics [][]common.Hash) error {
	return CheckLogQueryLimit(sys.cfg.LogQueryLimit, addresses, topics)
}

// CheckLogQueryLimit rejects a filter criteria whose address list, or any single
// topic position, exceeds limit. A limit of 0 disables the cap.
func CheckLogQueryLimit(limit int, addresses []common.Address, topics [][]common.Hash) error {
	if limit <= 0 {
		return nil
	}
	if len(addresses) > limit {
		return fmt.Errorf("%w: %d addresses requested, limit is %d", ErrExceedLogQueryLimit, len(addresses), limit)
	}
	for i, topicList := range topics {
		if len(topicList) > limit {
			return fmt.Errorf("%w: %d topics at position %d, limit is %d", ErrExceedLogQueryLimit, len(topicList), i, limit)
		}
	}
	return nil
}
