// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {OFT} from "@layerzerolabs/lz-evm-oapp-v2/contracts/oft/OFT.sol";
import {Origin} from "@layerzerolabs/lz-evm-oapp-v2/contracts/oapp/OApp.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {WorkerErrors} from "../common/WorkerErrors.sol";
import {WorkerTypes} from "../common/WorkerTypes.sol";

/// @title OFTPauseAndRateLimit
/// @notice OFT extension adding pathway pause controls and outbound token-bucket limits.
abstract contract OFTPauseAndRateLimit is OFT {
    /// @notice Whether outbound sends are paused for a destination endpoint.
    mapping(uint32 dstEid => bool paused) public sendPaused;

    /// @notice Whether inbound receives are paused for a source endpoint.
    mapping(uint32 srcEid => bool paused) public receivePaused;

    /// @notice Whether outbound rate limiting has been explicitly configured for a destination endpoint.
    mapping(uint32 dstEid => bool configured) public outboundRateLimitConfigured;

    /// @notice Outbound token bucket configuration by destination endpoint.
    mapping(uint32 dstEid => WorkerTypes.RateLimitConfig config) public outboundRateLimitConfig;

    /// @notice Outbound token bucket state by destination endpoint.
    mapping(uint32 dstEid => WorkerTypes.RateLimitState state) public outboundRateLimitState;

    /// @notice Emitted when outbound sends are paused or unpaused.
    /// @param dstEid Destination endpoint ID.
    /// @param paused New pause state.
    event SendPausedSet(uint32 indexed dstEid, bool paused);

    /// @notice Emitted when inbound receives are paused or unpaused.
    /// @param srcEid Source endpoint ID.
    /// @param paused New pause state.
    event ReceivePausedSet(uint32 indexed srcEid, bool paused);

    /// @notice Emitted when an outbound rate limit is configured.
    /// @param dstEid Destination endpoint ID.
    /// @param config New token bucket configuration.
    event OutboundRateLimitSet(uint32 indexed dstEid, WorkerTypes.RateLimitConfig config);

    /// @notice Emitted when an outbound rate limit is removed.
    /// @param dstEid Destination endpoint ID.
    event OutboundRateLimitCleared(uint32 indexed dstEid);

    /// @notice Initializes the OFT with LayerZero endpoint and delegate ownership.
    /// @param name_ ERC20 token name.
    /// @param symbol_ ERC20 token symbol.
    /// @param endpoint_ Local LayerZero endpoint.
    /// @param delegate_ Owner and LayerZero OApp delegate.
    constructor(string memory name_, string memory symbol_, address endpoint_, address delegate_)
        OFT(name_, symbol_, endpoint_, delegate_)
        Ownable(delegate_)
    {}

    /// @notice Pauses or unpauses outbound sends to a destination endpoint.
    /// @param dstEid Destination endpoint ID.
    /// @param paused New pause state.
    function pauseSend(uint32 dstEid, bool paused) external onlyOwner {
        sendPaused[dstEid] = paused;
        emit SendPausedSet(dstEid, paused);
    }

    /// @notice Pauses or unpauses inbound receives from a source endpoint.
    /// @param srcEid Source endpoint ID.
    /// @param paused New pause state.
    function pauseReceive(uint32 srcEid, bool paused) external onlyOwner {
        receivePaused[srcEid] = paused;
        emit ReceivePausedSet(srcEid, paused);
    }

    /// @notice Configures outbound token-bucket limits for one destination endpoint.
    /// @dev Setting both capacity and refillPerSecond to zero is an explicit drain mode.
    /// @param dstEid Destination endpoint ID.
    /// @param config Token bucket configuration.
    function setOutboundRateLimit(uint32 dstEid, WorkerTypes.RateLimitConfig calldata config) external onlyOwner {
        // Explicit configuration is tracked separately so an unset pathway remains unrestricted while
        // capacity=0/refill=0 is a deliberate drain setting that rejects new sends.
        outboundRateLimitConfigured[dstEid] = true;
        outboundRateLimitConfig[dstEid] = config;
        WorkerTypes.RateLimitState storage state = outboundRateLimitState[dstEid];
        state.tokens = config.capacity;
        state.updatedAt = uint64(block.timestamp);
        emit OutboundRateLimitSet(dstEid, config);
    }

    /// @notice Removes the outbound token-bucket limit for one destination endpoint.
    /// @param dstEid Destination endpoint ID.
    function clearOutboundRateLimit(uint32 dstEid) external onlyOwner {
        outboundRateLimitConfigured[dstEid] = false;
        delete outboundRateLimitConfig[dstEid];
        delete outboundRateLimitState[dstEid];
        emit OutboundRateLimitCleared(dstEid);
    }

    /// @notice Applies send pause and rate-limit checks before burning outbound OFT tokens.
    /// @param from Token owner debited by the OFT send.
    /// @param amountLD Amount requested in local decimals.
    /// @param minAmountLD Minimum amount accepted in local decimals.
    /// @param dstEid Destination endpoint ID.
    /// @return amountSentLD Amount burned on this chain.
    /// @return amountReceivedLD Amount expected on the destination chain.
    function _debit(address from, uint256 amountLD, uint256 minAmountLD, uint32 dstEid)
        internal
        virtual
        override
        returns (uint256 amountSentLD, uint256 amountReceivedLD)
    {
        if (sendPaused[dstEid]) revert WorkerErrors.SendPaused(dstEid);
        _consumeOutboundRateLimit(dstEid, amountLD);
        return super._debit(from, amountLD, minAmountLD, dstEid);
    }

    /// @notice Applies receive pause checks before minting inbound OFT tokens.
    /// @param origin LayerZero message origin.
    /// @param guid LayerZero message GUID.
    /// @param message Encoded OFT message.
    /// @param executor Executor address supplied by LayerZero.
    /// @param extraData Extra data supplied by LayerZero.
    function _lzReceive(
        Origin calldata origin,
        bytes32 guid,
        bytes calldata message,
        address executor,
        bytes calldata extraData
    ) internal virtual override {
        if (receivePaused[origin.srcEid]) revert WorkerErrors.ReceivePaused(origin.srcEid);
        super._lzReceive(origin, guid, message, executor, extraData);
    }

    /// @notice Consumes outbound rate-limit capacity for one destination endpoint.
    /// @param dstEid Destination endpoint ID.
    /// @param amount Token amount in local decimals.
    function _consumeOutboundRateLimit(uint32 dstEid, uint256 amount) internal {
        if (!outboundRateLimitConfigured[dstEid]) return;

        WorkerTypes.RateLimitConfig memory config = outboundRateLimitConfig[dstEid];

        WorkerTypes.RateLimitState storage state = outboundRateLimitState[dstEid];
        uint256 tokens = state.tokens;
        uint64 updatedAt = state.updatedAt;

        if (tokens > config.capacity) {
            tokens = config.capacity;
        }

        // Refill is capped at capacity, and the multiplication is bounded before it is evaluated.
        if (block.timestamp > updatedAt && config.refillPerSecond > 0 && tokens < config.capacity) {
            uint256 elapsed = block.timestamp - updatedAt;
            uint256 missing = config.capacity - tokens;
            uint256 refill = config.refillPerSecond > missing / elapsed ? missing : elapsed * config.refillPerSecond;
            tokens += refill;
        }

        if (amount > tokens) revert WorkerErrors.RateLimitExceeded(dstEid, amount, tokens);
        state.tokens = tokens - amount;
        state.updatedAt = uint64(block.timestamp);
    }
}
