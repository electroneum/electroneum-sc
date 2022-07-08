pragma solidity >=0.5.0;
  
interface ETNGovernanceInterface {

    function candidateRegistration() external payable;
    function withdrawRegistration() external;
    function candidateSupport(address validatorAddress) external payable;
    function withdrawSupport(address validatorAddress) external;
    function newEpoch(uint blockNumber) external;

    function getValidators() external view returns (address[] memory);

}