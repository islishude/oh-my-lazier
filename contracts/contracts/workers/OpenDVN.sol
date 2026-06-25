// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {ILayerZeroDVN} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/uln/interfaces/ILayerZeroDVN.sol";
import {WorkerAccess} from "../common/WorkerAccess.sol";
import {WorkerErrors} from "../common/WorkerErrors.sol";
import {WorkerFeeLib} from "../common/WorkerFeeLib.sol";
import {WorkerTypes} from "../common/WorkerTypes.sol";
import {PriceFeedStore} from "../common/PriceFeedStore.sol";

/// @title OpenDVN
/// @notice First-phase LayerZero DVN worker contract with strict pathway validation.
contract OpenDVN is ILayerZeroDVN, WorkerAccess, PriceFeedStore {
    /// @notice Per-destination and per-OApp pathway configuration.
    mapping(uint32 dstEid => mapping(address sender => WorkerTypes.PathwayConfig config)) public pathwayConfig;

    /// @notice Emitted when a LayerZero send library assigns a DVN job.
    /// @param jobId Deterministic job identifier derived from destination, packet header, payload hash, and sender.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param sendLib Calling LayerZero send library.
    /// @param confirmations Required source-chain confirmations.
    /// @param fee Quoted and paid DVN fee.
    event DVNJobAssigned(
        bytes32 indexed jobId,
        uint32 indexed dstEid,
        address indexed sender,
        address sendLib,
        uint64 confirmations,
        uint256 fee
    );

    /// @notice Emitted when a pathway configuration changes.
    /// @param dstEid Destination endpoint ID.
    /// @param sender Source OApp sender.
    /// @param config New pathway configuration.
    event PathwayConfigSet(uint32 indexed dstEid, address indexed sender, WorkerTypes.PathwayConfig config);

    /// @notice Initializes DVN ownership.
    /// @param initialOwner Initial owner address.
    constructor(address initialOwner) WorkerAccess(initialOwner) {}

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

    /// @notice Sets destination fee inputs.
    /// @param dstEid Destination endpoint ID.
    /// @param config Price configuration.
    function setPriceConfig(uint32 dstEid, WorkerTypes.PriceConfig calldata config) external onlyOwner {
        _setPriceConfig(dstEid, config);
    }

    /// @notice Quotes, charges, and assigns a DVN verification job.
    /// @param param LayerZero DVN assignment parameters.
    /// @param options DVN options, which must be empty in phase 1.
    /// @return fee Quoted DVN fee.
    function assignJob(AssignJobParam calldata param, bytes calldata options)
        external
        payable
        override
        whenNotPaused
        returns (uint256 fee)
    {
        // DVN assignment remains payable in the pinned interface, so underpayment is rejected on-chain.
        fee = _quote(param.dstEid, param.confirmations, param.sender, param.packetHeader.length, options);
        if (msg.value < fee) revert WorkerErrors.InsufficientFee(fee, msg.value);
        bytes32 jobId = keccak256(abi.encode(param.dstEid, param.packetHeader, param.payloadHash, param.sender));
        emit DVNJobAssigned(jobId, param.dstEid, param.sender, msg.sender, param.confirmations, fee);
    }

    /// @notice Quotes DVN fee for a pathway.
    /// @param dstEid Destination endpoint ID.
    /// @param confirmations Required source-chain confirmations.
    /// @param sender Source OApp sender.
    /// @param options DVN options, which must be empty in phase 1.
    /// @return fee Quoted DVN fee.
    function getFee(uint32 dstEid, uint64 confirmations, address sender, bytes calldata options)
        external
        view
        override
        returns (uint256 fee)
    {
        return _quote(dstEid, confirmations, sender, 0, options);
    }

    /// @notice Validates assignment inputs and calculates DVN fee.
    /// @param dstEid Destination endpoint ID.
    /// @param confirmations Required source-chain confirmations, reserved for future per-confirmation pricing.
    /// @param sender Source OApp sender.
    /// @param messageSize Packet header size checked during assignment.
    /// @param options DVN options, which must be empty in phase 1.
    /// @return Quoted DVN fee.
    function _quote(uint32 dstEid, uint64 confirmations, address sender, uint256 messageSize, bytes calldata options)
        internal
        view
        returns (uint256)
    {
        confirmations;
        if (!allowedSendLib[msg.sender]) revert WorkerErrors.UnauthorizedSendLib(msg.sender);
        if (options.length != 0) revert WorkerErrors.EmptyDVNOptions();

        WorkerTypes.PathwayConfig memory pathway = pathwayConfig[dstEid][sender];
        if (!pathway.enabled) revert WorkerErrors.PathwayDisabled(dstEid, sender);
        if (messageSize > pathway.maxMessageSize) {
            revert WorkerErrors.MessageTooLarge(messageSize, pathway.maxMessageSize);
        }

        return WorkerFeeLib.quoteDVN(dstEid, priceConfig[dstEid]);
    }
}
