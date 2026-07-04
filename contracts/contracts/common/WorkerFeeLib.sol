// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {WorkerErrors} from "./WorkerErrors.sol";
import {WorkerTypes} from "./WorkerTypes.sol";

/// @title WorkerFeeLib
/// @notice Fee quoting helpers for Executor and DVN worker contracts.
library WorkerFeeLib {
    uint16 internal constant BPS_DENOMINATOR = 10_000;

    /// @notice Reverts unless a destination price snapshot and fee model are fresh and internally valid.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot Shared price snapshot to validate.
    /// @param fee Worker role fee model to validate.
    function assertFresh(uint32 dstEid, WorkerTypes.PriceSnapshot memory snapshot, WorkerTypes.FeeModel memory fee)
        internal
        view
    {
        if (fee.marginBps > BPS_DENOMINATOR) revert WorkerErrors.InvalidBps(fee.marginBps);
        if (snapshot.updatedAt == 0 || block.timestamp > uint256(snapshot.updatedAt) + snapshot.staleAfter) {
            revert WorkerErrors.PriceSnapshotStale(dstEid, snapshot.updatedAt, snapshot.staleAfter);
        }
    }

    /// @notice Quotes executor payment from base fee, destination gas price, destination gas overhead, and margin.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot Shared destination price snapshot.
    /// @param fee Executor fee model.
    /// @param lzReceiveGas Gas requested for destination lzReceive.
    /// @return Native-token fee denominated in the source-chain token.
    function quoteExecutor(
        uint32 dstEid,
        WorkerTypes.PriceSnapshot memory snapshot,
        WorkerTypes.FeeModel memory fee,
        uint128 lzReceiveGas
    ) internal view returns (uint256) {
        return quoteWithGas(dstEid, snapshot, fee, uint256(lzReceiveGas));
    }

    /// @notice Quotes DVN payment from base fee, destination verification gas overhead, and margin.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot Shared destination price snapshot.
    /// @param fee DVN fee model.
    /// @return Native-token fee denominated in the source-chain token.
    function quoteDVN(uint32 dstEid, WorkerTypes.PriceSnapshot memory snapshot, WorkerTypes.FeeModel memory fee)
        internal
        view
        returns (uint256)
    {
        return quoteWithGas(dstEid, snapshot, fee, 0);
    }

    /// @notice Quotes worker payment from fixed fee plus configured and request-specific destination gas units.
    /// @param dstEid Destination endpoint ID.
    /// @param snapshot Shared destination price snapshot.
    /// @param fee Worker role fee model.
    /// @param variableDstGas Request-specific destination gas units.
    /// @return Native-token fee denominated in the source-chain token.
    function quoteWithGas(
        uint32 dstEid,
        WorkerTypes.PriceSnapshot memory snapshot,
        WorkerTypes.FeeModel memory fee,
        uint256 variableDstGas
    ) internal view returns (uint256) {
        assertFresh(dstEid, snapshot, fee);
        uint256 gasUnits = uint256(fee.dstGasOverhead) + variableDstGas;
        if (gasUnits > 0 && snapshot.dstGasPriceInSrcToken == 0) revert WorkerErrors.InvalidDstGasPrice(gasUnits);
        uint256 raw = fee.baseFee + gasUnits * snapshot.dstGasPriceInSrcToken;
        return raw + (raw * fee.marginBps) / BPS_DENOMINATOR;
    }
}
