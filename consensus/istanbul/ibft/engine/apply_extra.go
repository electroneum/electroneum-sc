package ibftengine

import "github.com/electroneum/electroneum-sc/core/types"

type ApplyIBFTExtra func(*types.IBFTExtra) error

func Combine(applies ...ApplyIBFTExtra) ApplyIBFTExtra {
	return func(extra *types.IBFTExtra) error {
		for _, apply := range applies {
			err := apply(extra)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func ApplyHeaderIBFTExtra(header *types.Header, applies ...ApplyIBFTExtra) error {
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
