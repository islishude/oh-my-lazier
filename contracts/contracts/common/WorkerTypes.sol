// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

/// @title WorkerTypes
/// @notice Shared structs used by the self-hosted Executor, DVN, and OFT controls.
library WorkerTypes {
    /// @notice Per-OApp pathway limits and enablement.
    struct PathwayConfig {
        bool enabled;
        uint256 maxMessageSize;
        uint128 minLzReceiveGas;
        uint128 maxLzReceiveGas;
    }

    /// @notice Native-token fee inputs for one destination endpoint.
    struct PriceConfig {
        uint256 baseFee;
        uint256 dstGasPriceInSrcToken;
        uint16 bufferBps;
        uint64 updatedAt;
        uint64 staleAfter;
    }

    /// @notice Decoded executor lzReceive option accepted in phase 1.
    struct ExecutorOption {
        uint128 lzReceiveGas;
        uint128 lzReceiveValue;
    }

    /// @notice Token bucket settings for outbound OFT sends.
    struct RateLimitConfig {
        uint256 capacity;
        uint256 refillPerSecond;
    }

    /// @notice Current token bucket state for outbound OFT sends.
    struct RateLimitState {
        uint256 tokens;
        uint64 updatedAt;
    }
}
