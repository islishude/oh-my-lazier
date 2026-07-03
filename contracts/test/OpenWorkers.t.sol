// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {OpenDVN} from "../contracts/workers/OpenDVN.sol";
import {OpenExecutor} from "../contracts/workers/OpenExecutor.sol";
import {TestOFT} from "../contracts/oft/TestOFT.sol";
import {WorkerErrors} from "../contracts/common/WorkerErrors.sol";
import {WorkerTypes} from "../contracts/common/WorkerTypes.sol";
import {Origin} from "@layerzerolabs/lz-evm-protocol-v2/contracts/interfaces/ILayerZeroEndpointV2.sol";
import {ILayerZeroDVN} from "@layerzerolabs/lz-evm-messagelib-v2/contracts/uln/interfaces/ILayerZeroDVN.sol";

contract EndpointMock {
    address public delegate;

    function setDelegate(address value) external {
        delegate = value;
    }
}

contract TestOFTHarness is TestOFT {
    constructor(address endpoint, address delegate, address initialRecipient, uint256 initialSupply)
        TestOFT("Test OFT", "TOFT", endpoint, delegate, initialRecipient, initialSupply)
    {}

    function exposedDebit(address from, uint256 amountLD, uint256 minAmountLD, uint32 dstEid)
        external
        returns (uint256 amountSentLD, uint256 amountReceivedLD)
    {
        return _debit(from, amountLD, minAmountLD, dstEid);
    }

    function exposedReceive(Origin calldata origin, bytes32 guid, bytes calldata message) external {
        _lzReceive(origin, guid, message, address(0), message[0:0]);
    }

    function forceRateLimitState(uint32 dstEid, uint256 tokens, uint64 updatedAt) external {
        outboundRateLimitState[dstEid] = WorkerTypes.RateLimitState({tokens: tokens, updatedAt: updatedAt});
    }
}

contract SendLibCaller {
    function executorFee(OpenExecutor executor, uint32 dstEid, address oapp, uint256 size, bytes calldata options)
        external
        view
        returns (uint256)
    {
        return executor.getFee(dstEid, oapp, size, options);
    }

    function assignExecutor(OpenExecutor executor, uint32 dstEid, address oapp, uint256 size, bytes calldata options)
        external
        returns (uint256)
    {
        return executor.assignJob(dstEid, oapp, size, options);
    }

    function dvnFee(OpenDVN dvn, uint32 dstEid, uint64 confirmations, address oapp, bytes calldata options)
        external
        view
        returns (uint256)
    {
        return dvn.getFee(dstEid, confirmations, oapp, options);
    }

    function assignDVN(OpenDVN dvn, ILayerZeroDVN.AssignJobParam calldata param, bytes calldata options)
        external
        payable
        returns (uint256)
    {
        return dvn.assignJob{value: msg.value}(param, options);
    }
}

contract ReceiveUlnMock {
    address public lastDVN;
    bytes public lastPacketHeader;
    bytes32 public lastPayloadHash;
    uint64 public lastConfirmations;

    function verify(bytes calldata packetHeader, bytes32 payloadHash, uint64 confirmations) external {
        lastDVN = msg.sender;
        lastPacketHeader = packetHeader;
        lastPayloadHash = payloadHash;
        lastConfirmations = confirmations;
    }
}

contract OpenWorkersTest {
    uint32 internal constant DST_EID = 40449;
    address internal constant OAPP = address(0x2002);

    OpenExecutor internal executor;
    OpenDVN internal dvn;
    SendLibCaller internal sendLib;
    TestOFTHarness internal oft;

    function setUp() public {
        executor = new OpenExecutor(address(this));
        dvn = new OpenDVN(address(this));
        sendLib = new SendLibCaller();
        oft = new TestOFTHarness(address(new EndpointMock()), address(this), address(this), 1_000_000 ether);

        WorkerTypes.PathwayConfig memory pathway = WorkerTypes.PathwayConfig({
            enabled: true, maxMessageSize: 1024, minLzReceiveGas: 50_000, maxLzReceiveGas: 500_000
        });
        WorkerTypes.PriceConfig memory price = WorkerTypes.PriceConfig({
            baseFee: 1 ether,
            dstGasPriceInSrcToken: 10 gwei,
            bufferBps: 3000,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });

        executor.setAllowedSendLib(address(sendLib), true);
        executor.setPathwayConfig(DST_EID, OAPP, pathway);
        executor.setPriceConfig(DST_EID, price);

        dvn.setAllowedSendLib(address(sendLib), true);
        dvn.setPathwayConfig(DST_EID, OAPP, pathway);
        dvn.setPriceConfig(DST_EID, price);
    }

    function test_executorFeeSuccess() public view {
        uint256 fee = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        require(fee == 1.3013 ether, "executor fee mismatch");
    }

    function test_executorRejectsStalePrice() public {
        WorkerTypes.PriceConfig memory stale = WorkerTypes.PriceConfig({
            baseFee: 1 ether, dstGasPriceInSrcToken: 10 gwei, bufferBps: 3000, updatedAt: 0, staleAfter: 30 minutes
        });
        executor.setPriceConfig(DST_EID, stale);

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.PriceConfigStale.selector
        );
    }

    function test_executorRejectsGasBelowMinimum() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, lzReceiveOption(49_999, 0))),
            WorkerErrors.InvalidGas.selector
        );
    }

    function test_executorRejectsGasAboveMaximum() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, lzReceiveOption(500_001, 0))),
            WorkerErrors.InvalidGas.selector
        );
    }

    function test_executorRejectsUnsupportedOptions() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(
                sendLib.executorFee,
                (
                    executor,
                    DST_EID,
                    OAPP,
                    512,
                    executorOption(2, bytes.concat(bytes16(uint128(1)), bytes32(uint256(1))))
                )
            ),
            WorkerErrors.UnsupportedOption.selector
        );
    }

    function test_executorRejectsNonZeroLzReceiveValue() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 1))),
            WorkerErrors.NonZeroLzReceiveValue.selector
        );
    }

    function test_executorRejectsDuplicateLzReceiveOption() public {
        bytes memory payload = bytes.concat(bytes16(uint128(100_000)));
        bytes memory duplicate = bytes.concat(executorOptionEntry(1, payload), executorOptionEntry(1, payload));
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, duplicate)),
            WorkerErrors.DuplicateLzReceiveOption.selector
        );
    }

    function test_executorRejectsNativeDropOption() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(
                sendLib.executorFee,
                (
                    executor,
                    DST_EID,
                    OAPP,
                    512,
                    executorOption(3, bytes.concat(bytes16(uint128(1)), bytes32(uint256(uint160(address(0x1234))))))
                )
            ),
            WorkerErrors.UnsupportedOption.selector
        );
    }

    function test_executorRejectsOrderedExecutionOption() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, executorOption(4, ""))),
            WorkerErrors.UnsupportedOption.selector
        );
    }

    function test_executorRejectsWhenPaused() public {
        executor.setPaused(true);
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.assignExecutor, (executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.Paused.selector
        );
    }

    function test_executorRejectsUnauthorizedSendLib() public {
        expectRevert(
            address(executor),
            abi.encodeCall(executor.getFee, (DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.UnauthorizedSendLib.selector
        );
    }

    function test_executorRejectsUnauthorizedOAppSender() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, address(0x9999), 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.PathwayDisabled.selector
        );
    }

    function test_executorRejectsMessageSize() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 1025, lzReceiveOption(100_000, 0))),
            WorkerErrors.MessageTooLarge.selector
        );
    }

    function test_dvnFeeSuccess() public view {
        uint256 fee = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        require(fee == 1.3 ether, "dvn fee mismatch");
    }

    function test_executorWithdraw() public {
        uint256 beforeBalance = address(this).balance;
        (bool funded,) = payable(address(executor)).call{value: 1 ether}("");
        require(funded, "fund executor failed");
        executor.withdraw(payable(address(this)), 1 ether);
        require(address(this).balance == beforeBalance, "executor withdraw failed");
    }

    function test_dvnRejectsStalePrice() public {
        WorkerTypes.PriceConfig memory stale = WorkerTypes.PriceConfig({
            baseFee: 1 ether, dstGasPriceInSrcToken: 10 gwei, bufferBps: 3000, updatedAt: 0, staleAfter: 30 minutes
        });
        dvn.setPriceConfig(DST_EID, stale);

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.dvnFee, (dvn, DST_EID, 12, OAPP, "")),
            WorkerErrors.PriceConfigStale.selector
        );
    }

    function test_dvnRejectsNonEmptyOptions() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.dvnFee, (dvn, DST_EID, 12, OAPP, hex"01")),
            WorkerErrors.EmptyDVNOptions.selector
        );
    }

    function test_dvnRejectsUnauthorizedSendLib() public {
        expectRevert(
            address(dvn), abi.encodeCall(dvn.getFee, (DST_EID, 12, OAPP, "")), WorkerErrors.UnauthorizedSendLib.selector
        );
    }

    function test_dvnRejectsUnauthorizedOAppSender() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.dvnFee, (dvn, DST_EID, 12, address(0x9999), "")),
            WorkerErrors.PathwayDisabled.selector
        );
    }

    function test_dvnRejectsMessageSize() public {
        bytes memory packetHeader = new bytes(1025);
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: packetHeader,
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        expectRevert(
            address(sendLib), abi.encodeCall(sendLib.assignDVN, (dvn, param, "")), WorkerErrors.MessageTooLarge.selector
        );
    }

    function test_dvnAssignRejectsInsufficientFee() public {
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        (bool ok, bytes memory data) =
            address(sendLib).call{value: 1 ether}(abi.encodeCall(sendLib.assignDVN, (dvn, param, "")));
        require(!ok, "expected revert");
        require(bytes4(data) == WorkerErrors.InsufficientFee.selector, "unexpected revert");
    }

    function test_dvnRejectsWhenPaused() public {
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        dvn.setPaused(true);
        expectRevert(
            address(sendLib), abi.encodeCall(sendLib.assignDVN, (dvn, param, "")), WorkerErrors.Paused.selector
        );
    }

    function test_dvnOwnerSetsVerifierAuthorization() public {
        address verifier = address(0xBEEF);
        dvn.setVerifier(verifier, true);
        require(dvn.verifiers(verifier), "verifier not allowed");
        dvn.setVerifier(verifier, false);
        require(!dvn.verifiers(verifier), "verifier still allowed");
    }

    function test_dvnSubmitVerificationRejectsUnauthorizedVerifier() public {
        ReceiveUlnMock receiveLib = new ReceiveUlnMock();
        expectRevert(
            address(dvn),
            abi.encodeCall(
                dvn.submitVerification, (address(receiveLib), hex"01020304", bytes32(uint256(0x55)), uint64(12))
            ),
            WorkerErrors.UnauthorizedVerifier.selector
        );
    }

    function test_dvnSubmitVerificationRecordsOpenDVNAsSender() public {
        ReceiveUlnMock receiveLib = new ReceiveUlnMock();
        bytes memory packetHeader = hex"01020304";
        bytes32 payloadHash = bytes32(uint256(0x55));

        dvn.setVerifier(address(this), true);
        dvn.submitVerification(address(receiveLib), packetHeader, payloadHash, 12);

        require(receiveLib.lastDVN() == address(dvn), "receive lib sender is not OpenDVN");
        require(keccak256(receiveLib.lastPacketHeader()) == keccak256(packetHeader), "packet header mismatch");
        require(receiveLib.lastPayloadHash() == payloadHash, "payload hash mismatch");
        require(receiveLib.lastConfirmations() == 12, "confirmations mismatch");
    }

    function test_dvnWithdraw() public {
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        uint256 beforeBalance = address(this).balance;
        uint256 fee = sendLib.assignDVN{value: 2 ether}(dvn, param, "");
        require(fee == 1.3 ether, "unexpected dvn fee");
        dvn.withdraw(payable(address(this)), 2 ether);
        require(address(this).balance == beforeBalance, "withdraw failed");
    }

    function test_oftSendPauseRejectsDebit() public {
        oft.pauseSend(DST_EID, true);
        expectRevert(
            address(oft),
            abi.encodeCall(oft.exposedDebit, (address(this), 1 ether, 1 ether, DST_EID)),
            WorkerErrors.SendPaused.selector
        );
    }

    function test_oftReceivePauseRejectsReceive() public {
        uint32 srcEid = 40161;
        oft.pauseReceive(srcEid, true);
        Origin memory origin = Origin({srcEid: srcEid, sender: bytes32(uint256(uint160(address(this)))), nonce: 1});
        expectRevert(
            address(oft),
            abi.encodeCall(oft.exposedReceive, (origin, bytes32(uint256(1)), hex"")),
            WorkerErrors.ReceivePaused.selector
        );
    }

    function test_oftRateLimitRejectsDebitAboveCapacity() public {
        WorkerTypes.RateLimitConfig memory limit = WorkerTypes.RateLimitConfig({capacity: 10 ether, refillPerSecond: 0});
        oft.setOutboundRateLimit(DST_EID, limit);

        expectRevert(
            address(oft),
            abi.encodeCall(oft.exposedDebit, (address(this), 11 ether, 11 ether, DST_EID)),
            WorkerErrors.RateLimitExceeded.selector
        );
    }

    function test_oftZeroRateLimitDrainsPathway() public {
        WorkerTypes.RateLimitConfig memory limit = WorkerTypes.RateLimitConfig({capacity: 0, refillPerSecond: 0});
        oft.setOutboundRateLimit(DST_EID, limit);

        expectRevert(
            address(oft),
            abi.encodeCall(oft.exposedDebit, (address(this), 1, 1, DST_EID)),
            WorkerErrors.RateLimitExceeded.selector
        );
    }

    function test_oftRateLimitAllowsDebitWithinCapacity() public {
        WorkerTypes.RateLimitConfig memory limit = WorkerTypes.RateLimitConfig({capacity: 10 ether, refillPerSecond: 0});
        oft.setOutboundRateLimit(DST_EID, limit);

        uint256 beforeBalance = oft.balanceOf(address(this));
        (uint256 sent, uint256 received) = oft.exposedDebit(address(this), 5 ether, 5 ether, DST_EID);
        require(sent == 5 ether, "unexpected sent amount");
        require(received == 5 ether, "unexpected received amount");
        require(oft.balanceOf(address(this)) == beforeBalance - 5 ether, "debit did not burn");
        (uint256 tokens,) = oft.outboundRateLimitState(DST_EID);
        require(tokens == 5 ether, "rate limit not consumed");
    }

    function test_oftRateLimitRefillsFromElapsedTime() public {
        WorkerTypes.RateLimitConfig memory limit =
            WorkerTypes.RateLimitConfig({capacity: 10 ether, refillPerSecond: 1 ether});
        oft.setOutboundRateLimit(DST_EID, limit);
        uint64 updatedAt = block.timestamp > 5 ? uint64(block.timestamp - 5) : 0;
        oft.forceRateLimitState(DST_EID, 0, updatedAt);

        uint256 expectedRefill = (block.timestamp - updatedAt) * 1 ether;
        require(expectedRefill > 0, "test requires elapsed time");
        uint256 amount = expectedRefill > 5 ether ? 5 ether : expectedRefill;
        (uint256 sent,) = oft.exposedDebit(address(this), amount, amount, DST_EID);
        require(sent == amount, "refilled debit failed");
        (uint256 tokens,) = oft.outboundRateLimitState(DST_EID);
        require(tokens == expectedRefill - amount, "unexpected post-refill tokens");
    }

    receive() external payable {}

    function expectRevert(address target, bytes memory callData, bytes4 selector) internal {
        (bool ok, bytes memory data) = target.call(callData);
        require(!ok, "expected revert");
        require(bytes4(data) == selector, "unexpected revert");
    }

    function lzReceiveOption(uint128 gasLimit, uint128 value) internal pure returns (bytes memory) {
        bytes memory payload =
            value == 0 ? bytes.concat(bytes16(gasLimit)) : bytes.concat(bytes16(gasLimit), bytes16(value));
        return executorOption(1, payload);
    }

    function executorOption(uint8 optionType, bytes memory payload) internal pure returns (bytes memory) {
        return executorOptionEntry(optionType, payload);
    }

    function executorOptionEntry(uint8 optionType, bytes memory payload) internal pure returns (bytes memory) {
        return bytes.concat(bytes1(uint8(1)), bytes2(uint16(payload.length + 1)), bytes1(optionType), payload);
    }
}
