// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {WorkerErrors} from "./WorkerErrors.sol";

/// @title WorkerAccess
/// @notice Owner-managed SendLib allowlist, pause switch, and native-token withdrawals.
abstract contract WorkerAccess is Ownable {
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

    /// @notice Initializes worker ownership.
    /// @param initialOwner Initial owner address.
    constructor(address initialOwner) Ownable(initialOwner) {}

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
}
