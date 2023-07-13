// contracts/ETNGovernanceInterface.sol
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.16;
  
interface ETNPriorityTransactorsInterface {
    struct TransactorMeta {
        uint64 startHeight;
        uint64 endHeight;

        bool isGasPriceWaiver;

        string publicKey;
        string name;
    }

    function getTransactors() external view returns (TransactorMeta[] memory);
}