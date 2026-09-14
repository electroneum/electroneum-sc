// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package state

import "github.com/electroneum/electroneum-sc/common"

// SetPriorityTransactors caches the priority-transactor allowlist for the block
// being processed. It is read once from the on-chain contract before the first
// transaction, so the per-transaction fee rules do not each pay for an EVM call.
func (s *StateDB) SetPriorityTransactors(transactors common.PriorityTransactorMap) {
	s.priorityTransactorsMu.Lock()
	defer s.priorityTransactorsMu.Unlock()
	s.priorityTransactors = make(common.PriorityTransactorMap, len(transactors))
	for key, transactor := range transactors {
		s.priorityTransactors[key] = transactor
	}
}

// GetPriorityTransactorByKey looks up a priority public key in the cached
// allowlist.
func (s *StateDB) GetPriorityTransactorByKey(pubkey common.PublicKey) (common.PriorityTransactor, bool) {
	s.priorityTransactorsMu.Lock()
	defer s.priorityTransactorsMu.Unlock()
	transactor, found := s.priorityTransactors[pubkey]
	return transactor, found
}
