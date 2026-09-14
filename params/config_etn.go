// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package params

// Electroneum-specific chain configuration: the IBFT/QBFT consensus parameters,
// the Transitions mechanism that lets them change at a given height, and the
// priority-transactors contract lookup.
//
// Kept out of config.go so the ETN delta against upstream go-ethereum stays
// isolated and future rebases stay mechanical.

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/math"
)

// IBFTConfig is the consensus engine configs for Istanbul based sealing.
type IBFTConfig struct {
	EpochLength              uint64 `json:"epochlength"`              // Number of blocks that should pass before pending validator votes are reset
	BlockPeriodSeconds       uint64 `json:"blockperiodseconds"`       // Minimum time between two consecutive IBFT or QBFT blocks’ timestamps in seconds
	RequestTimeoutSeconds    uint64 `json:"requesttimeoutseconds"`    // Minimum request timeout for each IBFT or QBFT round in seconds
	MaxRequestTimeoutSeconds uint64 `json:"maxrequesttimeoutseconds"` // Maximum request timeout for each IBFT or QBFT round in seconds
	ProposerPolicy           uint64 `json:"policy"`                   // The policy for proposer selection
	AllowedFutureBlockTime   uint64 `json:"allowedfutureblocktime"`   //Allowed number of seconds a timestamp can be in the future before it's considered a future block'
}

func (c IBFTConfig) String() string {
	return "IBFT"
}

type Transition struct {
	Block                              *big.Int       `json:"block"`
	EpochLength                        uint64         `json:"epochlength,omitempty"`              // Number of blocks that should pass before pending validator votes are reset
	BlockPeriodSeconds                 uint64         `json:"blockperiodseconds,omitempty"`       // Minimum time between two consecutive IBFT or QBFT blocks’ timestamps in seconds
	RequestTimeoutSeconds              uint64         `json:"requesttimeoutseconds,omitempty"`    // Minimum request timeout for each IBFT or QBFT round in seconds
	MaxRequestTimeoutSeconds           uint64         `json:"maxrequesttimeoutseconds,omitempty"` // Maximum request timeout for each IBFT or QBFT round in seconds
	PriorityTransactorsContractAddress common.Address `json:"prioritytransactorscontractaddress"` // Smart contract address for priority transactors
	AllowedFutureBlockTime             uint64         `json:"allowedfutureblocktime,omitempty"`
}

// IsFutureFork returns whether num is either equal to the Future fork block or greater.
func (c *ChainConfig) IsFutureFork(num *big.Int) bool {
	return isBlockForked(c.FutureForkBlock, num)
}

func (c *ChainConfig) GetPriorityTransactorsContractAddress(blockNumber *big.Int) common.Address {
	if c.Transitions != nil {
		for i := len(c.Transitions) - 1; i >= 0; i-- {
			if c.Transitions[i].Block.Cmp(blockNumber) <= 0 && c.Transitions[i].PriorityTransactorsContractAddress != (common.Address{}) {
				return c.Transitions[i].PriorityTransactorsContractAddress
			}
		}
	}
	return c.PriorityTransactorsContractAddress
}

func (c *ChainConfig) CheckTransitionsData() error {
	prevBlock := big.NewInt(0)
	for _, transition := range c.Transitions {
		if transition.Block == nil {
			return ErrBlockNumberMissing
		}
		if transition.Block.Cmp(prevBlock) < 0 {
			return ErrBlockOrder
		}
		prevBlock = transition.Block
	}
	return nil
}

func isTransitionsConfigCompatible(c1, c2 *ChainConfig, head *big.Int) (*big.Int, *big.Int, error) {
	if len(c1.Transitions) == 0 && len(c2.Transitions) == 0 {
		// maxCodeSizeConfig not used. return
		return big.NewInt(0), big.NewInt(0), nil
	}

	// existing config had Transitions and new one does not have the same return error
	if len(c1.Transitions) > 0 && len(c2.Transitions) == 0 {
		return head, head, fmt.Errorf("genesis file missing transitions information")
	}

	if len(c2.Transitions) > 0 && len(c1.Transitions) == 0 {
		return big.NewInt(0), big.NewInt(0), nil
	}

	// check the number of records below current head in both configs
	// if they do not match throw an error
	c1RecsBelowHead := 0
	for _, data := range c1.Transitions {
		if data.Block.Cmp(head) <= 0 {
			c1RecsBelowHead++
		} else {
			break
		}
	}

	c2RecsBelowHead := 0
	for _, data := range c2.Transitions {
		if data.Block.Cmp(head) <= 0 {
			c2RecsBelowHead++
		} else {
			break
		}
	}

	// if the count of past records is not matching return error
	if c1RecsBelowHead != c2RecsBelowHead {
		return head, head, errors.New("transitions data incompatible. updating transitions for past")
	}

	// validate that each past record is matching exactly. if not return error
	for i := 0; i < c1RecsBelowHead; i++ {
		isDifferentBlock := c1.Transitions[i].Block.Cmp(c2.Transitions[i].Block) != 0

		if isDifferentBlock {
			return head, head, fmt.Errorf("Block mismatch for transition %d", i)
		}

		if c1.Transitions[i].BlockPeriodSeconds != c2.Transitions[i].BlockPeriodSeconds {
			return head, head, ErrTransitionIncompatible("BlockPeriodSeconds")
		}
		if c1.Transitions[i].RequestTimeoutSeconds != c2.Transitions[i].RequestTimeoutSeconds {
			return head, head, ErrTransitionIncompatible("RequestTimeoutSeconds")
		}
		if c1.Transitions[i].MaxRequestTimeoutSeconds != c2.Transitions[i].MaxRequestTimeoutSeconds {
			return head, head, ErrTransitionIncompatible("MaxRequestTimeoutSeconds")
		}
		if c1.Transitions[i].AllowedFutureBlockTime != c2.Transitions[i].AllowedFutureBlockTime {
			return head, head, ErrTransitionIncompatible("AllowedFutureBlockTime")
		}
		if c1.Transitions[i].EpochLength != c2.Transitions[i].EpochLength {
			return head, head, ErrTransitionIncompatible("EpochLength")
		}
		if c1.Transitions[i].PriorityTransactorsContractAddress != c2.Transitions[i].PriorityTransactorsContractAddress {
			return head, head, ErrTransitionIncompatible("PriorityTransactorsContractAddress")
		}
	}

	return big.NewInt(0), big.NewInt(0), nil
}

// ETNMaxSupply is the maximum amount of ETN that can ever exist, in wei.
// GetBaseBlockReward clamps against it: once circulating supply reaches this
// value the block reward is zero.
const ETNMaxSupply = "21000000000000000000000000000"

// Electroneum networks. These replace go-ethereum's Ethereum chain configs:
// this node only ever speaks to Electroneum.
//
// Note what is NOT set. Shanghai and Cancun are absent, so PUSH0, transient
// storage and blob transactions never activate -- Electroneum has not
// scheduled them, and enabling one would fork us off the network.
var (
	// MainnetChainConfig is the chain parameters to run a node on the main network.
	MainnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(52014),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    nil,
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		ArrowGlacierBlock:   nil,
		FutureForkBlock:     big.NewInt(math.MaxInt64),
		IBFT: &IBFTConfig{
			BlockPeriodSeconds:       5,
			EpochLength:              17280,
			ProposerPolicy:           0,
			RequestTimeoutSeconds:    10,
			MaxRequestTimeoutSeconds: 60,
			AllowedFutureBlockTime:   5,
		},
		GenesisETN:                         math.MustParseBig256("17964946965760000000000000000"), // = terminal circulating supply for legacy mainnet [0,1806749}. Legacy emissions are burned from height 1806749 onwards
		LegacyV9ForkHeight:                 big.NewInt(862866),
		LegacyToSmartchainMigrationHeight:  big.NewInt(1806749),
		PriorityTransactorsContractAddress: common.HexToAddress("0x92cdf1fc0e54d3150f100265ae2717b0689660ee"),
		Transitions:                        []Transition{},
	}

	// StagenetChainConfig is the chain parameters to run a node on the test network.
	StagenetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(5201419),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    nil,
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		ArrowGlacierBlock:   nil,
		FutureForkBlock:     big.NewInt(math.MaxInt64),
		IBFT: &IBFTConfig{
			BlockPeriodSeconds:       5,
			EpochLength:              17280,
			ProposerPolicy:           0,
			RequestTimeoutSeconds:    10,
			MaxRequestTimeoutSeconds: 60,
			AllowedFutureBlockTime:   5,
		},
		GenesisETN:                         math.MustParseBig256("2000000000000000000000000000"), // 2Bn ETN allocated to developer accounts for testing
		LegacyV9ForkHeight:                 big.NewInt(862866),
		LegacyToSmartchainMigrationHeight:  big.NewInt(0),
		PriorityTransactorsContractAddress: common.HexToAddress("0x92cdf1fc0e54d3150f100265ae2717b0689660ee"),
	}

	// TestnetChainConfig is the chain parameters to run a node on the test network.
	TestnetChainConfig = &ChainConfig{
		ChainID:             big.NewInt(5201420),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(0),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    nil,
		BerlinBlock:         big.NewInt(0),
		LondonBlock:         big.NewInt(0),
		ArrowGlacierBlock:   nil,
		FutureForkBlock:     big.NewInt(math.MaxInt64),
		IBFT: &IBFTConfig{
			BlockPeriodSeconds:       5,
			EpochLength:              17280,
			ProposerPolicy:           0,
			RequestTimeoutSeconds:    10,
			MaxRequestTimeoutSeconds: 60,
			AllowedFutureBlockTime:   5,
		},
		// I observed that the legacy testnet gen block had billions emitted and later the 21B max supply overflowed.
		// Therefore I have entered the circ supply correct to up to and including the **MAINNET*** height 1675364 to
		// help mock the mainnet with the testnet (for now...we may do a reset further down the road)
		GenesisETN:                         math.MustParseBig256("17951808565760000000000000000"),
		LegacyV9ForkHeight:                 big.NewInt(707121),
		LegacyToSmartchainMigrationHeight:  big.NewInt(1455270),
		PriorityTransactorsContractAddress: common.HexToAddress("0x1ef0959497375a7539e487749584aeb4947b7a90"),
		Transitions:                        []Transition{},
	}
)
