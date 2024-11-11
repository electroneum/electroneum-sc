package ebftengine

import "github.com/electroneum/electroneum-sc/core/types"

type ApplyEBFTExtra func(*types.EBFTExtra) error

func Combine(applies ...ApplyEBFTExtra) ApplyEBFTExtra {
	return func(extra *types.EBFTExtra) error {
		for _, apply := range applies {
			err := apply(extra)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func ApplyHeaderEBFTExtra(header *types.Header, applies ...ApplyEBFTExtra) error {
	extra, err := getExtra(header)
	if err != nil {
		return err
	}

	err = Combine(applies...)(extra)
	if err != nil {
		return err
	}

	return setExtra(header, extra)
}
