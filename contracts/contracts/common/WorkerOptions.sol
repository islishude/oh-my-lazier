// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {CalldataBytesLib} from "@layerzerolabs/lz-evm-protocol-v2/contracts/libs/CalldataBytesLib.sol";
import {ExecutorOptions} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/libs/ExecutorOptions.sol";
import {WorkerErrors} from "./WorkerErrors.sol";
import {WorkerTypes} from "./WorkerTypes.sol";

/// @title WorkerOptions
/// @notice Strict LayerZero option parser for first-phase Executor support.
library WorkerOptions {
    using CalldataBytesLib for bytes;

    uint8 internal constant EXECUTOR_WORKER_ID = 1;
    uint16 internal constant TYPE_3 = 3;

    /// @notice Decodes the single supported executor option.
    /// @dev Parses LayerZero type-3 options and intentionally accepts only one executor lzReceive option.
    /// First-phase worker contracts reject every other worker/option type so unsupported execution modes
    /// cannot be silently priced or assigned.
    /// @param options LayerZero type-3 options bytes.
    /// @return parsed Decoded lzReceive gas and value.
    function decodeExecutorOptions(bytes calldata options)
        internal
        pure
        returns (WorkerTypes.ExecutorOption memory parsed)
    {
        if (options.length < 2 || options.toU16(0) != TYPE_3) revert WorkerErrors.InvalidOptions();

        bool hasLzReceive;
        uint256 cursor = 2;
        while (cursor < options.length) {
            if (options.length < cursor + 4) revert WorkerErrors.InvalidOptions();
            uint8 workerId = options.toU8(cursor);
            uint16 size = options.toU16(cursor + 1);
            if (size == 0 || options.length < cursor + 3 + size) revert WorkerErrors.InvalidOptions();

            uint8 optionType = options.toU8(cursor + 3);
            bytes calldata option = options[cursor + 4:cursor + 3 + size];
            cursor += 3 + size;

            if (workerId != EXECUTOR_WORKER_ID) revert WorkerErrors.UnsupportedOption(workerId, optionType);
            if (optionType != ExecutorOptions.OPTION_TYPE_LZRECEIVE) {
                revert WorkerErrors.UnsupportedOption(workerId, optionType);
            }
            if (hasLzReceive) revert WorkerErrors.DuplicateLzReceiveOption();

            (parsed.lzReceiveGas, parsed.lzReceiveValue) = ExecutorOptions.decodeLzReceiveOption(option);
            if (parsed.lzReceiveValue != 0) revert WorkerErrors.NonZeroLzReceiveValue(parsed.lzReceiveValue);
            hasLzReceive = true;
        }

        if (!hasLzReceive) revert WorkerErrors.MissingLzReceiveOption();
    }
}
