// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package core

// Electroneum priority-transaction fee rules.
//
// A PriorityTx is signed twice: once by the sender, once by a key on the
// on-chain priority-transactor allowlist. If that key carries a gas-price
// waiver, the transaction is exempt from the EIP-1559 base-fee floor and must
// carry all-zero fee fields. That is how Electroneum offers zero-fee
// transactions, and it is roughly 3.5% of live mainnet traffic.

import (
	"errors"
	"fmt"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core/vm"
)

var (
	errBadPriorityKey   = errors.New("priority public key is not an authorised transactor")
	errNoGasPriceWaiver = errors.New("priority transaction fee fields inconsistent with its waiver status")
)

// validatePriorityGasFields validates that a tx's gas fields are consistent with
// the priority-sender rules, and returns whether the sender has a gas-price waiver.
func validatePriorityGasFields(evm *vm.EVM, msg *Message) (bool, error) {
	// Not a priority tx
	ps := msg.PrioritySender
	if ps == (common.PublicKey{}) {
		return false, nil
	}

	// Look up the sender in the priority transactor map already cached in
	// statedb (populated by Process/ApplyTransaction before we get here),
	// rather than making another EVM static call to the contract.
	transactor, found := evm.StateDB.GetPriorityTransactorByKey(ps)
	if !found {
		return false, fmt.Errorf("%w: Bad priority pubkey %v", errBadPriorityKey, ps)
	}

	// Use the raw fee fields, NOT msg.GasPrice() which is the computed
	// effective gas price (min(tipCap+baseFee, feeCap)) and will be non-zero
	// for any valid non-waiver tx.
	feeCap := msg.GasFeeCap
	tipCap := msg.GasTipCap

	if transactor.IsGasPriceWaiver {
		// Waiver priority tx: fee cap and tip cap must both be 0
		if feeCap.Sign() != 0 || tipCap.Sign() != 0 {
			return true, fmt.Errorf("%w: waiver priority tx must have zero fee fields, pubkey %v", errNoGasPriceWaiver, ps)
		}
		return true, nil
	}

	// Non-waiver priority tx: FeeCap must be > 0 (they must pay)
	if feeCap.Sign() <= 0 {
		return false, fmt.Errorf("%w: non-waiver priority tx must have feeCap > 0, pubkey %v", errNoGasPriceWaiver, ps)
	}

	return false, nil
}
