// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package crypto

import (
	"crypto/ecdsa"

	"github.com/electroneum/electroneum-sc/common"
)

// ECDSAPubkeyToPublicKey converts a public key to the 65-byte uncompressed
// form Electroneum uses for priority signatures.
func ECDSAPubkeyToPublicKey(p ecdsa.PublicKey) common.PublicKey {
	return common.BytesToPublicKey(FromECDSAPub(&p))
}
