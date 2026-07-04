// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {WorkerErrors} from "./WorkerErrors.sol";
import {WorkerTypes} from "./WorkerTypes.sol";

/// @title OpenPriceFeed
/// @notice Owner-managed shared market price snapshots for source-chain workers.
contract OpenPriceFeed is Ownable {
    /// @notice Price snapshot by destination endpoint ID.
    mapping(uint32 dstEid => WorkerTypes.PriceSnapshot snapshot) public priceSnapshot;

    /// @notice Emitted when destination price inputs are updated.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot New price snapshot.
    event PriceSnapshotSet(uint32 indexed dstEid, WorkerTypes.PriceSnapshot snapshot);

    /// @notice Initializes price feed ownership.
    /// @param initialOwner Initial owner address.
    constructor(address initialOwner) Ownable(initialOwner) {}

    /// @notice Stores shared market price inputs for a destination endpoint.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot New price snapshot.
    function setPriceSnapshot(uint32 dstEid, WorkerTypes.PriceSnapshot calldata snapshot) external onlyOwner {
        if (snapshot.dstGasPriceInSrcToken == 0 || snapshot.updatedAt == 0 || snapshot.staleAfter == 0) {
            revert WorkerErrors.InvalidPriceSnapshot(dstEid);
        }
        priceSnapshot[dstEid] = snapshot;
        emit PriceSnapshotSet(dstEid, snapshot);
    }
}
