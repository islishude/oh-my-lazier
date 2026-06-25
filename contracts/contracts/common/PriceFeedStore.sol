// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {WorkerTypes} from "./WorkerTypes.sol";

/// @title PriceFeedStore
/// @notice Shared destination price configuration storage.
abstract contract PriceFeedStore {
    /// @notice Price configuration by destination endpoint ID.
    mapping(uint32 dstEid => WorkerTypes.PriceConfig config) public priceConfig;

    /// @notice Emitted when destination price inputs are updated.
    /// @param dstEid Destination endpoint ID.
    /// @param config New price configuration.
    event PriceConfigSet(uint32 indexed dstEid, WorkerTypes.PriceConfig config);

    /// @notice Stores price configuration for a destination endpoint.
    /// @param dstEid Destination endpoint ID.
    /// @param config New price configuration.
    function _setPriceConfig(uint32 dstEid, WorkerTypes.PriceConfig calldata config) internal {
        priceConfig[dstEid] = config;
        emit PriceConfigSet(dstEid, config);
    }
}
