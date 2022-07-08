// Require:
// 1. solc 0.5.4
// 2. abigen (make all from root)
//go:generate solc --abi --bin -o . --overwrite ./ETNGovernanceInterface.sol
//go:generate abigen -pkg governance -abi ./ETNGovernanceInterface.abi -bin ./ETNGovernanceInterface.bin -type ETNGovernanceInterface -out ./etn_governance.go
//go:generate rm ETNGovernanceInterface.abi ETNGovernanceInterface.bin

package governance
