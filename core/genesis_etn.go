// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package core

// Electroneum genesis blocks. The extra-data field carries the RLP-encoded IBFT
// validator set, which is why these cannot be expressed with upstream's
// Ethereum genesis helpers.

import (
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/hexutil"
	"github.com/electroneum/electroneum-sc/common/math"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rlp"
)

// DefaultGenesisBlock returns the Electroneum-sc mainnet genesis block.
func DefaultGenesisBlock() *Genesis {
	validatorSet := []common.Address{
		common.HexToAddress("0x135ec2bc4c04935ccd53967a072562120e4a3f92"),
		common.HexToAddress("0x57752beb85f0b8811023f048039591ef9eb78929"),
		common.HexToAddress("0xd8a2376cf7afa426414b960cfbfb4bb780e5181a"),
		common.HexToAddress("0x83ff6272d1de08ad8d492167412f1f329bd805f3"),
		common.HexToAddress("0x5584c91681cd7d850750941618dbaea2944e8ed4"),
		common.HexToAddress("0x763976728bc214a5c389d3928e03661bd6e7a649"),
		common.HexToAddress("0x915956a26fd7ee449d37ec93bbcfc5cad5ac8e27"),
		common.HexToAddress("0xd3e10f17e2e34e0a0fea05573992b21e13c224c9"),
		common.HexToAddress("0x7779ab2cb675d7a31714e86d01bd7a56a03f41d8"),
	}

	return &Genesis{
		Config:     params.MainnetChainConfig,
		Number:     0,
		Nonce:      0,
		Timestamp:  1709141867, // 28 Feb 2024
		ExtraData:  GenerateGenesisExtraDataForIBFTValSet(validatorSet),
		GasLimit:   30000000,
		GasUsed:    0, //ok unless we add a smart contract in the genesis state
		Difficulty: big.NewInt(1),
		Mixhash:    common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365"),
		ParentHash: common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		Coinbase:   common.Address{},
		Alloc: GenesisAlloc{
			common.HexToAddress("0x7b56c6e6f53498e3e9332b180fe41f1add202f28"): {Balance: math.MustParseBig256("17964946965760000000000000000")}, // = terminal circulating supply for legacy mainnet [0,1806749}. Legacy emissions are burned from height 1806749 onwards
		},
	}
}

// DefaultTestnetGenesisBlock returns the stage network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	validatorSet := []common.Address{
		common.HexToAddress("0x3254e381fbc4b4cb796cadbaa7f8f1039ce672db"),
		common.HexToAddress("0xad76beb1f31c987ceb8bca6bb1889ea72651ce03"),
		common.HexToAddress("0x3dec15db792252b5541b839b735731adc9e6506d"),
		common.HexToAddress("0x3d950613caddabbe8e2188b61e0a4ab66754dddd"),
	}
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Number:     0,
		Nonce:      0,
		Timestamp:  1707989393, // feb 15 2024
		ExtraData:  GenerateGenesisExtraDataForIBFTValSet(validatorSet),
		GasLimit:   30000000,
		GasUsed:    0, //ok unless we add a smart contract in the genesis state
		Difficulty: big.NewInt(1),
		Mixhash:    common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365"),
		ParentHash: common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		Coinbase:   common.Address{},
		Alloc: GenesisAlloc{ // the address is correct / predetermined
			common.HexToAddress("0x8baf588ed346f0dff956da926d0ab473b4bc9dd9"): {Balance: math.MustParseBig256("17951808565760000000000000000")},
		},
	}
}

// DefaultStagenetGenesisBlock returns the test network genesis block.
func DefaultStagenetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.StagenetChainConfig,
		Nonce:      0,
		Timestamp:  0,
		ExtraData:  hexutil.MustDecode("0xf88fa00000000000000000000000000000000000000000000000000000000000000000f86994c21ee98b5a90a6a45aba37fa5eddf90f5e8e181694ff0d56bd960c455a71f908496c79e8eafec34ccf9407afbe0d7d36b80454be1e185f55e02b9453625a944f9a82d7e094de7fb70d9ce2033ec0d65ac311249497f060952b1008c75cb030e3599725ad5cc306a2c080c0"),
		GasLimit:   16234336,
		Difficulty: big.NewInt(1),
		Mixhash:    common.HexToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365"),
		Coinbase:   common.Address{},
		Alloc: GenesisAlloc{
			common.HexToAddress("0x72f1a0bAA7f1C79129A391C2F32bCD8247A18a63"): {Balance: math.MustParseBig256("1000000000000000000000000000")},
			common.HexToAddress("0xf29A0844926Fe8d63e5B211978B26E3f6d9e9fd5"): {Balance: math.MustParseBig256("1000000000000000000000000000")},
		},
	}
}

func GenerateGenesisExtraDataForIBFTValSet(valset []common.Address) []byte {
	// Initialize a pointer to an instance of types.QBFTExtra
	extra := &types.QBFTExtra{
		VanityData:    make([]byte, 32),
		Validators:    valset,     // Update as necessary
		Vote:          nil,        // Nil at genesis
		Round:         0,          // 0 at genesis
		CommittedSeal: [][]byte{}, // Empty at genesis
	}

	// Encode the instance to bytes
	extraBytes, err := rlp.EncodeToBytes(extra)
	if err != nil {
		panic("RLP Encoding of genesis extra failed. Unable to create genesis block")
	}

	//genesisExtraDataHex := hex.EncodeToString(extraBytes)
	//fmt.Println(genesisExtraDataHex)

	return extraBytes
}
