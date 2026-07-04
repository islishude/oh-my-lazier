// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {ILayerZeroExecutor} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/interfaces/ILayerZeroExecutor.sol";
import {WorkerAccess} from "../common/WorkerAccess.sol";
import {WorkerErrors} from "../common/WorkerErrors.sol";
import {WorkerFeeLib} from "../common/WorkerFeeLib.sol";
import {WorkerOptions} from "../common/WorkerOptions.sol";
import {WorkerTypes} from "../common/WorkerTypes.sol";
import {IPriceFeed} from "../common/IPriceFeed.sol";

/// @title OpenExecutor
/// @notice First-phase LayerZero Executor worker contract with strict option and pathway validation.
contract OpenExecutor is ILayerZeroExecutor, WorkerAccess {
    /// @notice Shared source-chain price feed used for destination market price snapshots.
    IPriceFeed public immutable priceFeed;

    /// @notice Per-destination and per-OApp pathway configuration.
    mapping(uint32 dstEid => mapping(address sender => WorkerTypes.PathwayConfig config)) public pathwayConfig;

    /// @notice Per-destination executor fee model.
    mapping(uint32 dstEid => WorkerTypes.FeeModel model) public feeModel;

    /// @notice Emitted when a LayerZero send library assigns an executor job.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param sendLib Calling LayerZero send library.
    /// @param calldataSize Dynamic calldata size quoted by the send library.
    /// @param price Quoted executor price.
    /// @param options LayerZero options bytes.
    event ExecutorJobAssigned(
        uint32 indexed dstEid,
        address indexed sender,
        address indexed sendLib,
        uint256 calldataSize,
        uint256 price,
        bytes options
    );

    /// @notice Emitted when a pathway configuration changes.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param config New pathway configuration.
    event PathwayConfigSet(uint32 indexed dstEid, address indexed sender, WorkerTypes.PathwayConfig config);

    /// @notice Emitted when an executor fee model changes.
    /// @param dstEid Destination endpoint ID.
    /// @param model New executor fee model.
    event FeeModelSet(uint32 indexed dstEid, WorkerTypes.FeeModel model);

    /// @notice Initializes executor ownership.
    /// @param initialOwner Initial owner address.
    /// @param sharedPriceFeed Shared price feed contract.
    constructor(address initialOwner, address sharedPriceFeed) WorkerAccess(initialOwner) {
        require(sharedPriceFeed != address(0), "price feed required");
        priceFeed = IPriceFeed(sharedPriceFeed);
    }

    /// @notice Sets pathway controls for a destination and source OApp.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param config Pathway limits and enablement.
    function setPathwayConfig(uint32 dstEid, address sender, WorkerTypes.PathwayConfig calldata config)
        external
        onlyOwner
    {
        pathwayConfig[dstEid][sender] = config;
        emit PathwayConfigSet(dstEid, sender, config);
    }

    /// @notice Sets executor fee model inputs.
    /// @param dstEid Destination endpoint ID.
    /// @param model Fee model.
    function setFeeModel(uint32 dstEid, WorkerTypes.FeeModel calldata model) external onlyOwner {
        feeModel[dstEid] = model;
        emit FeeModelSet(dstEid, model);
    }

    /// @notice Quotes and assigns an executor job for a LayerZero send library.
    /// @dev The pinned LayerZero interface is nonpayable; this function emits the quoted price for off-chain accounting.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param calldataSize Dynamic calldata size quoted by the send library.
    /// @param options LayerZero type-3 options bytes.
    /// @return price Quoted executor price.
    function assignJob(uint32 dstEid, address sender, uint256 calldataSize, bytes calldata options)
        external
        override
        whenNotPaused
        returns (uint256 price)
    {
        // The pinned LayerZero ILayerZeroExecutor exposes nonpayable assignJob, so this contract
        // stays interface-compatible and emits the quoted price for off-chain accounting.
        price = _quote(dstEid, sender, calldataSize, options);
        emit ExecutorJobAssigned(dstEid, sender, msg.sender, calldataSize, price, options);
    }

    /// @notice Quotes executor fee for a pathway and options payload.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param calldataSize Dynamic calldata size quoted by the send library.
    /// @param options LayerZero type-3 options bytes.
    /// @return price Quoted executor price.
    function getFee(uint32 dstEid, address sender, uint256 calldataSize, bytes calldata options)
        external
        view
        override
        returns (uint256 price)
    {
        return _quote(dstEid, sender, calldataSize, options);
    }

    /// @notice Validates assignment inputs and calculates executor fee.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param calldataSize Dynamic calldata size quoted by the send library.
    /// @param options LayerZero type-3 options bytes.
    /// @return Quoted executor price.
    function _quote(uint32 dstEid, address sender, uint256 calldataSize, bytes calldata options)
        internal
        view
        returns (uint256)
    {
        if (!allowedSendLib[msg.sender]) revert WorkerErrors.UnauthorizedSendLib(msg.sender);
        WorkerTypes.PathwayConfig memory pathway = pathwayConfig[dstEid][sender];
        if (!pathway.enabled) revert WorkerErrors.PathwayDisabled(dstEid, sender);
        if (calldataSize > pathway.maxMessageSize) {
            revert WorkerErrors.MessageTooLarge(calldataSize, pathway.maxMessageSize);
        }

        WorkerTypes.ExecutorOption memory parsed = WorkerOptions.decodeExecutorOptions(options);
        if (parsed.lzReceiveGas < pathway.minLzReceiveGas || parsed.lzReceiveGas > pathway.maxLzReceiveGas) {
            revert WorkerErrors.InvalidGas(parsed.lzReceiveGas, pathway.minLzReceiveGas, pathway.maxLzReceiveGas);
        }

        return
            WorkerFeeLib.quoteExecutor(dstEid, priceFeed.priceSnapshot(dstEid), feeModel[dstEid], parsed.lzReceiveGas);
    }
}
