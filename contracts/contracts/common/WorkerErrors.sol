// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

/// @title WorkerErrors
/// @notice Custom errors shared by worker contracts.
library WorkerErrors {
    error UnauthorizedSendLib(address sendLib);
    error UnauthorizedVerifier(address verifier);
    error PathwayDisabled(uint32 dstEid, address sender);
    error MessageTooLarge(uint256 size, uint256 maxSize);
    error InvalidPriceSnapshot(uint32 dstEid);
    error PriceSnapshotStale(uint32 dstEid, uint256 updatedAt, uint256 staleAfter);
    error InsufficientFee(uint256 required, uint256 supplied);
    error InvalidBps(uint16 bps);
    error InvalidDstGasPrice(uint256 gasUnits);
    error InvalidGas(uint256 gasLimit, uint256 minGas, uint256 maxGas);
    error InvalidOptions();
    error MissingLzReceiveOption();
    error DuplicateLzReceiveOption();
    error UnsupportedOption(uint8 workerId, uint8 optionType);
    error NonZeroLzReceiveValue(uint128 value);
    error EmptyDVNOptions();
    error Paused();
    error SendPaused(uint32 dstEid);
    error ReceivePaused(uint32 srcEid);
    error RateLimitExceeded(uint32 dstEid, uint256 requested, uint256 available);
}
