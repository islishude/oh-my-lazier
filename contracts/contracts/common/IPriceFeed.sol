// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {WorkerTypes} from "./WorkerTypes.sol";

/// @title IPriceFeed
/// @notice Shared destination price snapshots used by worker fee quoting.
interface IPriceFeed {
    /// @notice Reads the price snapshot for a destination endpoint.
    /// @param dstEid Destination endpoint ID.
    /// @return snapshot Shared price inputs for the destination.
    function priceSnapshot(uint32 dstEid) external view returns (WorkerTypes.PriceSnapshot memory snapshot);
}
