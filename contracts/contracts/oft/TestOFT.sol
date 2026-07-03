// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {OFTPauseAndRateLimit} from "./OFTPauseAndRateLimit.sol";

/// @title TestOFT
/// @notice Mint/burn OFT used for the initial Sepolia/Hoodi pathway.
contract TestOFT is OFTPauseAndRateLimit {
    /// @notice Deploys the test OFT and optionally mints initial supply.
    /// @param name_ ERC20 token name.
    /// @param symbol_ ERC20 token symbol.
    /// @param endpoint_ Local LayerZero endpoint.
    /// @param delegate_ Owner and LayerZero OApp delegate.
    /// @param initialRecipient_ Recipient for initial minted supply.
    /// @param initialSupply_ Initial supply minted in local decimals.
    constructor(
        string memory name_,
        string memory symbol_,
        address endpoint_,
        address delegate_,
        address initialRecipient_,
        uint256 initialSupply_
    ) OFTPauseAndRateLimit(name_, symbol_, endpoint_, delegate_) {
        if (initialSupply_ != 0) {
            _mint(initialRecipient_, initialSupply_);
        }
    }

    // @notice Mints new tokens to the specified address.
    /// @param to The address to mint tokens to.
    /// @param amount The amount of tokens to mint in local decimals.
    function mint(address to, uint256 amount) external onlyOwner {
        _mint(to, amount);
    }
}
