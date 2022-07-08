package params

import (
	"errors"
	"fmt"
)

var (
	ErrBlockNumberMissing            = errors.New("block number not given in transitions data")
	ErrBlockOrder                    = errors.New("block order should be ascending")
	ErrValidatorSelectionMode        = errors.New("validator selection mode is invalid, should be either `contract` or `blockheader`")
	ErrMissingValidatorSelectionMode = errors.New("validator selection mode is missing, should specify `contract` when using validatorcontractaddress")
)

func ErrTransitionIncompatible(field string) error {
	return fmt.Errorf("transitions.%s data incompatible. %s historical data does not match", field, field)
}
