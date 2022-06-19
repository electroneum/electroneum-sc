package testutils

import (
	"bytes"
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	istanbulcommon "github.com/ethereum/go-ethereum/consensus/istanbul/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

func Genesis(validators []common.Address) *core.Genesis {
	// generate genesis block
	genesis := core.DefaultGenesisBlock()
	genesis.Config = params.TestChainConfig
	// force enable Istanbul engine
	genesis.Config.IBFT = &params.IBFTConfig{}
	genesis.Config.Ethash = nil
	genesis.ExtraData = nil
	genesis.Difficulty = istanbulcommon.DefaultDifficulty
	genesis.Nonce = istanbulcommon.EmptyBlockNonce.Uint64()
	genesis.Mixhash = types.IstanbulDigest

	appendValidators(genesis, validators)

	return genesis
}

func GenesisAndKeys(n int) (*core.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	var addrs = make([]common.Address, n)
	for i := 0; i < n; i++ {
		nodeKeys[i], _ = crypto.GenerateKey()
		addrs[i] = crypto.PubkeyToAddress(nodeKeys[i].PublicKey)
	}

	// generate genesis block
	genesis := Genesis(addrs)

	return genesis, nodeKeys
}

func appendValidators(genesis *core.Genesis, addrs []common.Address) {
	vanity := append(genesis.ExtraData, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(genesis.ExtraData))...)
	ist := &types.QBFTExtra{
		VanityData:    vanity,
		Validators:    addrs,
		Vote:          nil,
		CommittedSeal: [][]byte{},
		Round:         0,
	}

	istPayload, err := rlp.EncodeToBytes(&ist)
	if err != nil {
		panic("failed to encode istanbul extra")
	}
	genesis.ExtraData = istPayload
}
