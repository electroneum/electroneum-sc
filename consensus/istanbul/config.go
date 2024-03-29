// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package istanbul

import (
	"math/big"
	"sync"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/naoina/toml"
)

type ProposerPolicyId uint64

const (
	RoundRobin ProposerPolicyId = iota
	Sticky
)

// ProposerPolicy represents the Validator Proposer Policy
type ProposerPolicy struct {
	Id         ProposerPolicyId    // Could be RoundRobin or Sticky
	By         ValidatorSortByFunc // func that defines how the ValidatorSet should be sorted
	registry   []ValidatorSet      // Holds the ValidatorSet for a given block height
	registryMU *sync.Mutex         // Mutex to lock access to changes to Registry
}

// NewRoundRobinProposerPolicy returns a RoundRobin ProposerPolicy with ValidatorSortByString as default sort function
func NewRoundRobinProposerPolicy() *ProposerPolicy {
	return NewProposerPolicy(RoundRobin)
}

// NewStickyProposerPolicy return a Sticky ProposerPolicy with ValidatorSortByString as default sort function
func NewStickyProposerPolicy() *ProposerPolicy {
	return NewProposerPolicy(Sticky)
}

func NewProposerPolicy(id ProposerPolicyId) *ProposerPolicy {
	return NewProposerPolicyByIdAndSortFunc(id, ValidatorSortByString())
}

func NewProposerPolicyByIdAndSortFunc(id ProposerPolicyId, by ValidatorSortByFunc) *ProposerPolicy {
	return &ProposerPolicy{Id: id, By: by, registryMU: new(sync.Mutex)}
}

type proposerPolicyToml struct {
	Id ProposerPolicyId
}

func (p *ProposerPolicy) MarshalTOML() (interface{}, error) {
	if p == nil {
		return nil, nil
	}
	pp := &proposerPolicyToml{Id: p.Id}
	data, err := toml.Marshal(pp)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (p *ProposerPolicy) UnmarshalTOML(decode func(interface{}) error) error {
	var innerToml string
	err := decode(&innerToml)
	if err != nil {
		return err
	}
	var pp proposerPolicyToml
	err = toml.Unmarshal([]byte(innerToml), &pp)
	if err != nil {
		return err
	}
	p.Id = pp.Id
	p.By = ValidatorSortByString()
	return nil
}

// Use sets the ValidatorSortByFunc for the given ProposerPolicy and sorts the validatorSets according to it
func (p *ProposerPolicy) Use(v ValidatorSortByFunc) {
	p.By = v

	for _, validatorSet := range p.registry {
		validatorSet.SortValidators()
	}
}

// RegisterValidatorSet stores the given ValidatorSet in the policy registry
func (p *ProposerPolicy) RegisterValidatorSet(valSet ValidatorSet) {
	p.registryMU.Lock()
	defer p.registryMU.Unlock()

	if len(p.registry) == 0 {
		p.registry = []ValidatorSet{valSet}
	} else {
		p.registry = append(p.registry, valSet)
	}
}

// ClearRegistry removes any ValidatorSet from the ProposerPolicy registry
func (p *ProposerPolicy) ClearRegistry() {
	p.registryMU.Lock()
	defer p.registryMU.Unlock()

	p.registry = nil
}

type Config struct {
	RequestTimeoutSeconds              uint64              `toml:",omitempty"` // The timeout for each Istanbul round in seconds.
	MaxRequestTimeoutSeconds           uint64              `toml:",omitempty"` // Max request timeout for each Istanbul round in seconds.
	BlockPeriod                        uint64              `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy                     *ProposerPolicy     `toml:",omitempty"` // The policy for proposer selection
	Epoch                              uint64              `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	AllowedFutureBlockTime             uint64              `toml:",omitempty"` // Max time (in seconds) from current time allowed for blocks, before they're considered future blocks
	Transitions                        []params.Transition // Transition data
	PriorityTransactorsContractAddress common.Address      // PriorityTransactors contract address
}

var DefaultConfig = &Config{
	RequestTimeoutSeconds:    10,
	MaxRequestTimeoutSeconds: 60,
	BlockPeriod:              5,
	ProposerPolicy:           NewRoundRobinProposerPolicy(),
	Epoch:                    30000,
	AllowedFutureBlockTime:   5,
}

func (c Config) GetConfig(blockNumber *big.Int) Config {
	newConfig := c
	for i := 0; c.Transitions != nil && i < len(c.Transitions) && c.Transitions[i].Block.Cmp(blockNumber) <= 0; i++ {
		if c.Transitions[i].RequestTimeoutSeconds != 0 {
			newConfig.RequestTimeoutSeconds = c.Transitions[i].RequestTimeoutSeconds
		}
		if c.Transitions[i].EpochLength != 0 {
			newConfig.Epoch = c.Transitions[i].EpochLength
		}
		if c.Transitions[i].BlockPeriodSeconds != 0 {
			newConfig.BlockPeriod = c.Transitions[i].BlockPeriodSeconds
		}
		if c.Transitions[i].MaxRequestTimeoutSeconds != 0 {
			newConfig.MaxRequestTimeoutSeconds = c.Transitions[i].MaxRequestTimeoutSeconds
		}
		if c.Transitions[i].AllowedFutureBlockTime != 0 {
			newConfig.AllowedFutureBlockTime = c.Transitions[i].AllowedFutureBlockTime
		}
	}
	return newConfig
}
