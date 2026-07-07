// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {ISendLib} from "@layerzerolabs/lz-evm-protocol-v2/contracts/interfaces/ISendLib.sol";
import {IPriceFeed} from "./IPriceFeed.sol";
import {WorkerErrors} from "./WorkerErrors.sol";

/// @title WorkerAccess
/// @notice Owner-managed SendLib allowlist, pause switch, and native-token withdrawals.
abstract contract WorkerAccess is Ownable {
    /// @notice Shared source-chain price feed used for destination market price snapshots.
    IPriceFeed public priceFeed;

    /// @notice Whether a LayerZero send library is allowed to assign or quote jobs.
    mapping(address sendLib => bool allowed) public allowedSendLib;

    /// @notice Global pause flag for job assignment.
    bool public paused;

    /// @notice Emitted when a send library allowlist entry changes.
    /// @param sendLib LayerZero send library address.
    /// @param allowed Whether the send library is allowed.
    event AllowedSendLibSet(address indexed sendLib, bool allowed);

    /// @notice Emitted when the global pause flag changes.
    /// @param paused New pause state.
    event PausedSet(bool paused);

    /// @notice Emitted when native tokens are withdrawn by the owner.
    /// @param recipient Withdrawal recipient.
    /// @param amount Amount withdrawn.
    event Withdrawn(address indexed recipient, uint256 amount);

    /// @notice Emitted when worker fees are withdrawn from a LayerZero send library.
    /// @param sendLib LayerZero send library that held the worker fee balance.
    /// @param recipient Withdrawal recipient.
    /// @param amount Amount withdrawn.
    event SendLibFeeWithdrawn(address indexed sendLib, address indexed recipient, uint256 amount);

    /// @notice Emitted when the worker price feed changes.
    /// @param previousPriceFeed Previous price feed address.
    /// @param newPriceFeed New price feed address.
    event PriceFeedSet(address indexed previousPriceFeed, address indexed newPriceFeed);

    /// @notice Initializes worker ownership and price feed.
    /// @param initialOwner Initial owner address.
    /// @param initialPriceFeed Initial shared price feed contract.
    constructor(address initialOwner, address initialPriceFeed) Ownable(initialOwner) {
        _setPriceFeed(initialPriceFeed);
    }

    /// @notice Accepts native-token payments or refunds sent directly to the worker.
    receive() external payable {}

    modifier whenNotPaused() {
        if (paused) revert WorkerErrors.Paused();
        _;
    }

    /// @notice Adds or removes a LayerZero send library from the allowlist.
    /// @param sendLib LayerZero send library address.
    /// @param allowed Whether the send library is allowed.
    function setAllowedSendLib(address sendLib, bool allowed) external onlyOwner {
        allowedSendLib[sendLib] = allowed;
        emit AllowedSendLibSet(sendLib, allowed);
    }

    /// @notice Updates the shared source-chain price feed used for worker fee quotes.
    /// @param newPriceFeed New shared price feed contract.
    function setPriceFeed(address newPriceFeed) external onlyOwner {
        _setPriceFeed(newPriceFeed);
    }

    /// @notice Updates the global assignment pause flag.
    /// @param value New pause state.
    function setPaused(bool value) external onlyOwner {
        paused = value;
        emit PausedSet(value);
    }

    /// @notice Withdraws native tokens from the worker contract.
    /// @param recipient Recipient of withdrawn native tokens.
    /// @param amount Amount to withdraw.
    function withdraw(address payable recipient, uint256 amount) external onlyOwner {
        (bool ok,) = recipient.call{value: amount}("");
        require(ok, "withdraw failed");
        emit Withdrawn(recipient, amount);
    }

    /// @notice Withdraws this worker's accumulated native fees from an allowed LayerZero send library.
    /// @param sendLib LayerZero send library holding the worker fee balance.
    /// @param recipient Recipient of withdrawn fees.
    /// @param amount Amount to withdraw.
    function withdrawFee(address sendLib, address recipient, uint256 amount) external onlyOwner {
        if (!allowedSendLib[sendLib]) revert WorkerErrors.UnauthorizedSendLib(sendLib);
        ISendLib(sendLib).withdrawFee(recipient, amount);
        emit SendLibFeeWithdrawn(sendLib, recipient, amount);
    }

    function _setPriceFeed(address newPriceFeed) internal {
        if (newPriceFeed == address(0)) revert WorkerErrors.InvalidPriceFeed(newPriceFeed);
        address previousPriceFeed = address(priceFeed);
        priceFeed = IPriceFeed(newPriceFeed);
        emit PriceFeedSet(previousPriceFeed, newPriceFeed);
    }
}
