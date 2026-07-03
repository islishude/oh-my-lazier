// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {ILayerZeroDVN} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/uln/interfaces/ILayerZeroDVN.sol";
import {IReceiveUlnE2} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/uln/interfaces/IReceiveUlnE2.sol";
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

    /// @notice Addresses allowed to submit ReceiveUln302 verification through this DVN contract.
    mapping(address verifier => bool allowed) public verifiers;

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

    /// @notice Emitted when verifier authorization changes.
    /// @param verifier Verifier address whose authorization changed.
    /// @param allowed Whether the verifier is allowed to submit verification.
    event VerifierSet(address indexed verifier, bool allowed);

    /// @notice Emitted after an authorized verifier submits a ReceiveUln302 verification through this DVN.
    /// @param verifier Authorized verifier caller.
    /// @param receiveLib Destination ReceiveUln302 library called by this DVN.
    /// @param payloadHash Packet payload hash submitted to ReceiveUln302.
    /// @param packetHeaderHash Hash of the packet header submitted to ReceiveUln302.
    /// @param confirmations Source confirmations submitted to ReceiveUln302.
    event DVNVerificationSubmitted(
        address indexed verifier,
        address indexed receiveLib,
        bytes32 indexed payloadHash,
        bytes32 packetHeaderHash,
        uint64 confirmations
    );

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

    /// @notice Sets whether an address may submit destination verification through this DVN.
    /// @param verifier Verifier address to update.
    /// @param allowed Whether the verifier is allowed to submit verification.
    function setVerifier(address verifier, bool allowed) external onlyOwner {
        verifiers[verifier] = allowed;
        emit VerifierSet(verifier, allowed);
    }

    /// @notice Submits destination ReceiveUln302 verification with this DVN as the recorded verifier.
    /// @param receiveLib Destination ReceiveUln302 library.
    /// @param packetHeader LayerZero packet header.
    /// @param payloadHash LayerZero packet payload hash.
    /// @param confirmations Source confirmations observed by the verifier.
    function submitVerification(
        address receiveLib,
        bytes calldata packetHeader,
        bytes32 payloadHash,
        uint64 confirmations
    ) external {
        if (!verifiers[msg.sender]) revert WorkerErrors.UnauthorizedVerifier(msg.sender);
        IReceiveUlnE2(receiveLib).verify(packetHeader, payloadHash, confirmations);
        emit DVNVerificationSubmitted(msg.sender, receiveLib, payloadHash, keccak256(packetHeader), confirmations);
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
