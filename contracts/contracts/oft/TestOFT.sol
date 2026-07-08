// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {
    MessagingFee,
    MessagingReceipt
} from "@layerzerolabs/lz-evm-protocol-v2/contracts/interfaces/ILayerZeroEndpointV2.sol";
import {OFTReceipt, SendParam} from "@layerzerolabs/lz-evm-oapp-v2/contracts/oft/interfaces/IOFT.sol";
import {OFTPauseAndRateLimit} from "./OFTPauseAndRateLimit.sol";

/// @title TestOFT
/// @notice Mint/burn OFT used for the initial Sepolia/Hoodi pathway.
contract TestOFT is OFTPauseAndRateLimit {
    /// @notice Raised when a multi-send batch has no entries.
    error EmptyMultiSend();

    /// @notice Raised when a multi-send call supplies the wrong native fee.
    /// @param expected Expected native fee amount.
    /// @param actual Supplied native fee amount.
    error MultiSendNativeFeeMismatch(uint256 expected, uint256 actual);

    /// @notice Raised when a multi-send is entered while another multi-send is active.
    error ReentrantMultiSend();

    uint256 private multiSendNativeFeeRemaining;
    bool private multiSendActive;

    /// @notice Deploys the test OFT and optionally mints initial supply.
    /// @param name_ ERC20 token name.
    /// @param symbol_ ERC20 token symbol.
    /// @param endpoint_ Local LayerZero endpoint.
    /// @param delegate_ Owner and LayerZero OApp delegate.
    /// @param initialRecipient_ Recipient for initial minted supply.
    /// @param initialSupply_ Initial supply minted in local decimals.
    constructor(
        string memory name_,
        string memory symbol_,
        address endpoint_,
        address delegate_,
        address initialRecipient_,
        uint256 initialSupply_
    ) OFTPauseAndRateLimit(name_, symbol_, endpoint_, delegate_) {
        if (initialSupply_ != 0) {
            _mint(initialRecipient_, initialSupply_);
        }
    }

    // @notice Mints new tokens to the specified address.
    /// @param to The address to mint tokens to.
    /// @param amount The amount of tokens to mint in local decimals.
    function mint(address to, uint256 amount) external onlyOwner {
        _mint(to, amount);
    }

    /// @notice Quotes several OFT sends as one batch.
    /// @param sendParams OFT send parameters to quote in order.
    /// @param payInLzToken Whether LayerZero fees are paid in LZ token.
    /// @return totalFee Sum of native and LZ token fees for the batch.
    /// @return fees Per-send LayerZero messaging fees.
    function quoteMultiSend(SendParam[] calldata sendParams, bool payInLzToken)
        external
        view
        returns (MessagingFee memory totalFee, MessagingFee[] memory fees)
    {
        return _quoteMultiSend(sendParams, payInLzToken);
    }

    /// @notice Sends several OFT messages in one source-chain transaction.
    /// @param sendParams OFT send parameters to execute in order.
    /// @param payInLzToken Whether LayerZero fees are paid in LZ token.
    /// @param refundAddress Address receiving any LayerZero fee refunds.
    /// @return receipts Per-send LayerZero messaging receipts.
    /// @return oftReceipts Per-send OFT debit receipts.
    function multiSend(SendParam[] calldata sendParams, bool payInLzToken, address refundAddress)
        external
        payable
        returns (MessagingReceipt[] memory receipts, OFTReceipt[] memory oftReceipts)
    {
        if (multiSendActive) revert ReentrantMultiSend();

        (MessagingFee memory totalFee, MessagingFee[] memory fees) = _quoteMultiSend(sendParams, payInLzToken);
        if (msg.value != totalFee.nativeFee) {
            revert MultiSendNativeFeeMismatch(totalFee.nativeFee, msg.value);
        }

        receipts = new MessagingReceipt[](sendParams.length);
        oftReceipts = new OFTReceipt[](sendParams.length);
        multiSendActive = true;
        multiSendNativeFeeRemaining = totalFee.nativeFee;
        for (uint256 index = 0; index < sendParams.length; index++) {
            SendParam calldata sendParam = sendParams[index];
            (uint256 amountSentLD, uint256 amountReceivedLD) =
                _debit(msg.sender, sendParam.amountLD, sendParam.minAmountLD, sendParam.dstEid);
            (bytes memory message, bytes memory options) = _buildMsgAndOptions(sendParam, amountReceivedLD);

            receipts[index] = _lzSend(sendParam.dstEid, message, options, fees[index], refundAddress);
            oftReceipts[index] = OFTReceipt(amountSentLD, amountReceivedLD);

            emit OFTSent(receipts[index].guid, sendParam.dstEid, msg.sender, amountSentLD, amountReceivedLD);
        }
        if (multiSendNativeFeeRemaining != 0) {
            revert MultiSendNativeFeeMismatch(totalFee.nativeFee, totalFee.nativeFee - multiSendNativeFeeRemaining);
        }
        multiSendActive = false;
    }

    function _quoteMultiSend(SendParam[] calldata sendParams, bool payInLzToken)
        private
        view
        returns (MessagingFee memory totalFee, MessagingFee[] memory fees)
    {
        if (sendParams.length == 0) revert EmptyMultiSend();

        fees = new MessagingFee[](sendParams.length);
        for (uint256 index = 0; index < sendParams.length; index++) {
            (, uint256 amountReceivedLD) =
                _debitView(sendParams[index].amountLD, sendParams[index].minAmountLD, sendParams[index].dstEid);
            (bytes memory message, bytes memory options) = _buildMsgAndOptions(sendParams[index], amountReceivedLD);
            fees[index] = _quote(sendParams[index].dstEid, message, options, payInLzToken);
            totalFee.nativeFee += fees[index].nativeFee;
            totalFee.lzTokenFee += fees[index].lzTokenFee;
        }
    }

    function _payNative(uint256 nativeFee) internal override returns (uint256) {
        if (!multiSendActive) {
            return super._payNative(nativeFee);
        }
        if (multiSendNativeFeeRemaining < nativeFee) {
            revert MultiSendNativeFeeMismatch(nativeFee, multiSendNativeFeeRemaining);
        }
        multiSendNativeFeeRemaining -= nativeFee;
        return nativeFee;
    }
}
