// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {WorkerErrors} from "./WorkerErrors.sol";
import {WorkerTypes} from "./WorkerTypes.sol";

/// @title WorkerFeeLib
/// @notice Fee quoting helpers for Executor and DVN worker contracts.
library WorkerFeeLib {
    uint16 internal constant BPS_DENOMINATOR = 10_000;

    /// @notice Reverts unless a destination price config is fresh and internally valid.
    /// @param dstEid Destination endpoint ID.
    /// @param config Price configuration to validate.
    function assertFresh(uint32 dstEid, WorkerTypes.PriceConfig memory config) internal view {
        if (config.bufferBps > BPS_DENOMINATOR) revert WorkerErrors.InvalidBps(config.bufferBps);
        if (config.updatedAt == 0 || block.timestamp > uint256(config.updatedAt) + config.staleAfter) {
            revert WorkerErrors.PriceConfigStale(dstEid, config.updatedAt, config.staleAfter);
        }
    }

    /// @notice Quotes executor payment from base fee, destination gas price, and buffer.
    /// @param dstEid Destination endpoint ID.
    /// @param config Destination price configuration.
    /// @param lzReceiveGas Gas requested for destination lzReceive.
    /// @return Native-token fee denominated in the source-chain token.
    function quoteExecutor(uint32 dstEid, WorkerTypes.PriceConfig memory config, uint128 lzReceiveGas)
        internal
        view
        returns (uint256)
    {
        assertFresh(dstEid, config);
        uint256 raw = config.baseFee + uint256(lzReceiveGas) * config.dstGasPriceInSrcToken;
        return raw + (raw * config.bufferBps) / BPS_DENOMINATOR;
    }

    /// @notice Quotes DVN payment from base fee and buffer.
    /// @param dstEid Destination endpoint ID.
    /// @param config Destination price configuration.
    /// @return Native-token fee denominated in the source-chain token.
    function quoteDVN(uint32 dstEid, WorkerTypes.PriceConfig memory config) internal view returns (uint256) {
        assertFresh(dstEid, config);
        return config.baseFee + (config.baseFee * config.bufferBps) / BPS_DENOMINATOR;
    }
}
