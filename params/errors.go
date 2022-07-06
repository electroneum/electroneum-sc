package params

import (
	"errors"
	"fmt"
)

var (
	ErrBlockNumberMissing = errors.New("block number not given in transitions data")
	ErrBlockOrder         = errors.New("block order should be ascending")
)

func ErrTransitionIncompatible(field string) error {
	return fmt.Errorf("transitions.%s data incompatible. %s historical data does not match", field, field)
}
