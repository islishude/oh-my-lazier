// SPDX-License-Identifier: MIT
pragma solidity ^0.8.35;

import {OpenDVN} from "../contracts/workers/OpenDVN.sol";
import {OpenExecutor} from "../contracts/workers/OpenExecutor.sol";
import {OpenPriceFeed} from "../contracts/workers/OpenPriceFeed.sol";
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
    mapping(address worker => uint256 fee) public fees;

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
        uint256 fee = executor.assignJob(dstEid, oapp, size, options);
        fees[address(executor)] += fee;
        return fee;
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
        uint256 fee = dvn.assignJob(param, options);
        fees[address(dvn)] += fee;
        return fee;
    }

    function setPriceSnapshot(OpenPriceFeed feed, WorkerTypes.PriceSnapshotUpdate[] calldata updates) external {
        feed.setPriceSnapshot(updates);
    }

    function callExecutorSetPriceFeed(OpenExecutor executor, address newPriceFeed) external {
        executor.setPriceFeed(newPriceFeed);
    }

    function callDVNSetPriceFeed(OpenDVN dvn, address newPriceFeed) external {
        dvn.setPriceFeed(newPriceFeed);
    }

    function withdrawFee(address to, uint256 amount) external {
        require(fees[msg.sender] >= amount, "insufficient worker fee");
        fees[msg.sender] -= amount;
        (bool ok,) = to.call{value: amount}("");
        require(ok, "send lib withdraw failed");
    }

    receive() external payable {}
}

contract PriceFeedMock {
    mapping(uint32 dstEid => WorkerTypes.PriceSnapshot snapshot) public priceSnapshot;

    function setPriceSnapshot(uint32 dstEid, WorkerTypes.PriceSnapshot calldata snapshot) external {
        priceSnapshot[dstEid] = snapshot;
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
    uint32 internal constant ALT_DST_EID = 40161;
    address internal constant OAPP = address(0x2002);

    OpenExecutor internal executor;
    OpenDVN internal dvn;
    OpenPriceFeed internal priceFeed;
    SendLibCaller internal sendLib;
    TestOFTHarness internal oft;

    function setUp() public {
        priceFeed = new OpenPriceFeed(address(this), singleAddress(address(this)));
        executor = new OpenExecutor(address(this), address(priceFeed));
        dvn = new OpenDVN(address(this), address(priceFeed));
        sendLib = new SendLibCaller();
        oft = new TestOFTHarness(address(new EndpointMock()), address(this), address(this), 1_000_000 ether);

        WorkerTypes.PathwayConfig memory pathway = WorkerTypes.PathwayConfig({
            enabled: true, maxMessageSize: 1024, minLzReceiveGas: 50_000, maxLzReceiveGas: 500_000
        });
        WorkerTypes.PriceSnapshot memory snapshot = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 10 gwei,
            dstDataFeePerByteInSrcToken: 0,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });
        WorkerTypes.FeeModel memory fee =
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 3000});
        setPriceSnapshot(priceFeed, DST_EID, snapshot);

        executor.setAllowedSendLib(address(sendLib), true);
        executor.setPathwayConfig(DST_EID, OAPP, pathway);
        executor.setFeeModel(DST_EID, fee);

        dvn.setAllowedSendLib(address(sendLib), true);
        dvn.setPathwayConfig(DST_EID, OAPP, pathway);
        dvn.setFeeModel(DST_EID, fee);
    }

    function test_executorFeeSuccess() public view {
        uint256 fee = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        require(fee == 1.301313 ether, "executor fee mismatch");
    }

    function test_priceFeedRejectsUnauthorizedUpdate() public {
        WorkerTypes.PriceSnapshot memory snapshot = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 10 gwei,
            dstDataFeePerByteInSrcToken: 0,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });
        WorkerTypes.PriceSnapshotUpdate[] memory updates = singleUpdate(DST_EID, snapshot);
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.setPriceSnapshot, (priceFeed, updates)),
            WorkerErrors.UnauthorizedPriceSubmitter.selector
        );
    }

    function test_priceFeedOwnerCanManageSubmitterWithoutImplicitSubmitAccess() public {
        OpenPriceFeed managedFeed = new OpenPriceFeed(address(this), singleAddress(address(0xBEEF)));
        WorkerTypes.PriceSnapshot memory snapshot = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 10 gwei,
            dstDataFeePerByteInSrcToken: 0,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });
        WorkerTypes.PriceSnapshotUpdate[] memory updates = singleUpdate(DST_EID, snapshot);

        expectRevert(
            address(managedFeed),
            abi.encodeCall(managedFeed.setPriceSnapshot, (updates)),
            WorkerErrors.UnauthorizedPriceSubmitter.selector
        );

        managedFeed.setSubmitter(address(this), true);
        managedFeed.setPriceSnapshot(updates);
        require(managedFeed.submitters(address(this)), "owner was not added as submitter");

        managedFeed.setSubmitter(address(this), false);
        require(!managedFeed.submitters(address(this)), "owner submitter was not removed");
        expectRevert(
            address(managedFeed),
            abi.encodeCall(managedFeed.setPriceSnapshot, (updates)),
            WorkerErrors.UnauthorizedPriceSubmitter.selector
        );
    }

    function test_priceFeedRejectsZeroSubmitter() public {
        expectRevert(
            address(priceFeed),
            abi.encodeCall(priceFeed.setSubmitter, (address(0), true)),
            WorkerErrors.InvalidPriceSubmitter.selector
        );
    }

    function test_priceFeedSubmitterCanBatchUpdateMultipleEIDs() public {
        WorkerTypes.PriceSnapshotUpdate[] memory updates = new WorkerTypes.PriceSnapshotUpdate[](2);
        updates[0] = WorkerTypes.PriceSnapshotUpdate({
            dstEid: DST_EID,
            snapshot: WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 20 gwei,
                dstDataFeePerByteInSrcToken: 2 gwei,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        });
        updates[1] = WorkerTypes.PriceSnapshotUpdate({
            dstEid: ALT_DST_EID,
            snapshot: WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 30 gwei,
                dstDataFeePerByteInSrcToken: 3 gwei,
                updatedAt: uint64(block.timestamp),
                staleAfter: 1 hours
            })
        });

        priceFeed.setPriceSnapshot(updates);

        (uint256 dstGasPriceInSrcToken, uint256 dstDataFeePerByteInSrcToken,, uint64 staleAfter) =
            priceFeed.priceSnapshot(DST_EID);
        require(dstGasPriceInSrcToken == 20 gwei, "primary batch snapshot not stored");
        require(dstDataFeePerByteInSrcToken == 2 gwei, "primary batch data fee not stored");
        require(staleAfter == 30 minutes, "primary batch staleAfter not stored");
        (dstGasPriceInSrcToken, dstDataFeePerByteInSrcToken,, staleAfter) = priceFeed.priceSnapshot(ALT_DST_EID);
        require(dstGasPriceInSrcToken == 30 gwei, "alternate batch snapshot not stored");
        require(dstDataFeePerByteInSrcToken == 3 gwei, "alternate batch data fee not stored");
        require(staleAfter == 1 hours, "alternate batch staleAfter not stored");
    }

    function test_priceFeedRejectsEmptyBatch() public {
        WorkerTypes.PriceSnapshotUpdate[] memory updates = new WorkerTypes.PriceSnapshotUpdate[](0);
        expectRevert(
            address(priceFeed),
            abi.encodeCall(priceFeed.setPriceSnapshot, (updates)),
            WorkerErrors.InvalidPriceSnapshotBatch.selector
        );
    }

    function test_priceFeedRejectsInvalidSnapshot() public {
        WorkerTypes.PriceSnapshot memory invalid = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 0,
            dstDataFeePerByteInSrcToken: 0,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });
        expectRevert(
            address(priceFeed),
            abi.encodeCall(priceFeed.setPriceSnapshot, (singleUpdate(DST_EID, invalid))),
            WorkerErrors.InvalidPriceSnapshot.selector
        );
    }

    function test_sharedPriceFeedUpdateChangesExecutorAndDVNQuotes() public {
        WorkerTypes.PriceSnapshot memory snapshot = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 20 gwei,
            dstDataFeePerByteInSrcToken: 0,
            updatedAt: uint64(block.timestamp),
            staleAfter: 30 minutes
        });
        setPriceSnapshot(priceFeed, DST_EID, snapshot);

        uint256 executorFee = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        uint256 dvnFee = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        require(executorFee == 1.302626 ether, "executor fee did not use shared price");
        require(dvnFee == 1.300026 ether, "dvn fee did not use shared price");
    }

    function test_workerFeeModelsStayIndependent() public {
        executor.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 2 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 3000})
        );

        uint256 executorFee = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        uint256 dvnFee = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        require(executorFee == 2.601313 ether, "executor fee model not applied");
        require(dvnFee == 1.300013 ether, "dvn fee model leaked");
    }

    function test_executorRejectsStalePrice() public {
        WorkerTypes.PriceSnapshot memory stale = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 10 gwei, dstDataFeePerByteInSrcToken: 0, updatedAt: 0, staleAfter: 30 minutes
        });
        PriceFeedMock staleFeed = new PriceFeedMock();
        staleFeed.setPriceSnapshot(DST_EID, stale);
        OpenExecutor staleExecutor = new OpenExecutor(address(this), address(staleFeed));
        staleExecutor.setAllowedSendLib(address(sendLib), true);
        staleExecutor.setPathwayConfig(DST_EID, OAPP, defaultPathwayConfig());
        staleExecutor.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 3000})
        );

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (staleExecutor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.PriceSnapshotStale.selector
        );
    }

    function test_executorRejectsInvalidBps() public {
        executor.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 10_001})
        );

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.InvalidBps.selector
        );
    }

    function test_executorRejectsZeroGasPriceWithNonZeroGasUnits() public {
        PriceFeedMock zeroGasFeed = new PriceFeedMock();
        zeroGasFeed.setPriceSnapshot(
            DST_EID,
            WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 0,
                dstDataFeePerByteInSrcToken: 0,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        );
        OpenExecutor zeroGasExecutor = new OpenExecutor(address(this), address(zeroGasFeed));
        zeroGasExecutor.setAllowedSendLib(address(sendLib), true);
        zeroGasExecutor.setPathwayConfig(DST_EID, OAPP, defaultPathwayConfig());
        zeroGasExecutor.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 3000})
        );

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.executorFee, (zeroGasExecutor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0))),
            WorkerErrors.InvalidDstGasPrice.selector
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

    function test_ownerCanRotateExecutorPriceFeed() public {
        PriceFeedMock rotatedFeed = new PriceFeedMock();
        rotatedFeed.setPriceSnapshot(
            DST_EID,
            WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 20 gwei,
                dstDataFeePerByteInSrcToken: 0,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        );

        executor.setPriceFeed(address(rotatedFeed));

        require(address(executor.priceFeed()) == address(rotatedFeed), "executor price feed not rotated");
        uint256 fee = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        require(fee == 1.302626 ether, "executor did not read rotated feed");
    }

    function test_workerRejectsInvalidPriceFeed() public {
        expectRevert(
            address(executor),
            abi.encodeCall(executor.setPriceFeed, (address(0))),
            WorkerErrors.InvalidPriceFeed.selector
        );
        expectRevert(
            address(dvn), abi.encodeCall(dvn.setPriceFeed, (address(0))), WorkerErrors.InvalidPriceFeed.selector
        );
    }

    function test_workerRejectsUnauthorizedPriceFeedRotation() public {
        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.callExecutorSetPriceFeed, (executor, address(priceFeed))),
            0x118cdaa7
        );
        expectRevert(
            address(sendLib), abi.encodeCall(sendLib.callDVNSetPriceFeed, (dvn, address(priceFeed))), 0x118cdaa7
        );
    }

    function test_executorFeeIncludesCalldataDataFee() public {
        PriceFeedMock dataFeed = new PriceFeedMock();
        dataFeed.setPriceSnapshot(
            DST_EID,
            WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 10 gwei,
                dstDataFeePerByteInSrcToken: 1 gwei,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        );
        executor.setPriceFeed(address(dataFeed));
        executor.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 100, marginBps: 0})
        );

        uint256 small = sendLib.executorFee(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        uint256 large = sendLib.executorFee(executor, DST_EID, OAPP, 1024, lzReceiveOption(100_000, 0));

        require(small == 1.001010612 ether, "executor data fee mismatch");
        require(large - small == 512 gwei, "executor data fee is not size based");
    }

    function test_dvnFeeSuccess() public view {
        uint256 fee = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        require(fee == 1.300013 ether, "dvn fee mismatch");
    }

    function test_executorWithdraw() public {
        uint256 beforeBalance = address(this).balance;
        (bool funded,) = payable(address(executor)).call{value: 1 ether}("");
        require(funded, "fund executor failed");
        executor.withdraw(payable(address(this)), 1 ether);
        require(address(this).balance == beforeBalance, "executor withdraw failed");
    }

    function test_executorWithdrawFeeFromAllowedSendLib() public {
        uint256 fee = sendLib.assignExecutor(executor, DST_EID, OAPP, 512, lzReceiveOption(100_000, 0));
        require(fee == 1.301313 ether, "unexpected executor fee");
        require(sendLib.fees(address(executor)) == fee, "executor fee accounting missing");

        uint256 beforeBalance = address(this).balance;
        (bool funded,) = payable(address(sendLib)).call{value: fee}("");
        require(funded, "fund send lib failed");
        executor.withdrawFee(address(sendLib), address(this), fee);

        require(address(this).balance == beforeBalance, "executor send lib fee withdraw failed");
        require(sendLib.fees(address(executor)) == 0, "executor fee accounting not debited");
    }

    function test_dvnRejectsStalePrice() public {
        WorkerTypes.PriceSnapshot memory stale = WorkerTypes.PriceSnapshot({
            dstGasPriceInSrcToken: 10 gwei, dstDataFeePerByteInSrcToken: 0, updatedAt: 0, staleAfter: 30 minutes
        });
        PriceFeedMock staleFeed = new PriceFeedMock();
        staleFeed.setPriceSnapshot(DST_EID, stale);
        OpenDVN staleDVN = new OpenDVN(address(this), address(staleFeed));
        staleDVN.setAllowedSendLib(address(sendLib), true);
        staleDVN.setPathwayConfig(DST_EID, OAPP, defaultPathwayConfig());
        staleDVN.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 0, marginBps: 3000})
        );

        expectRevert(
            address(sendLib),
            abi.encodeCall(sendLib.dvnFee, (staleDVN, DST_EID, 12, OAPP, "")),
            WorkerErrors.PriceSnapshotStale.selector
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

    function test_dvnAssignUsesSendLibInternalAccountingWithoutNativeValue() public {
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        uint256 fee = sendLib.assignDVN(dvn, param, "");

        require(fee == 1.300013 ether, "unexpected dvn fee");
        require(address(dvn).balance == 0, "dvn should not receive assignment value");
        require(sendLib.fees(address(dvn)) == fee, "send lib fee accounting missing");
    }

    function test_ownerCanRotateDVNPriceFeed() public {
        PriceFeedMock rotatedFeed = new PriceFeedMock();
        rotatedFeed.setPriceSnapshot(
            DST_EID,
            WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 20 gwei,
                dstDataFeePerByteInSrcToken: 0,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        );

        dvn.setPriceFeed(address(rotatedFeed));

        require(address(dvn.priceFeed()) == address(rotatedFeed), "dvn price feed not rotated");
        uint256 fee = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        require(fee == 1.300026 ether, "dvn did not read rotated feed");
    }

    function test_dvnDataFeeUsesConfiguredOverheadOnly() public {
        PriceFeedMock dataFeed = new PriceFeedMock();
        dataFeed.setPriceSnapshot(
            DST_EID,
            WorkerTypes.PriceSnapshot({
                dstGasPriceInSrcToken: 10 gwei,
                dstDataFeePerByteInSrcToken: 2 gwei,
                updatedAt: uint64(block.timestamp),
                staleAfter: 30 minutes
            })
        );
        dvn.setPriceFeed(address(dataFeed));
        dvn.setFeeModel(
            DST_EID,
            WorkerTypes.FeeModel({baseFee: 1 ether, dstGasOverhead: 1000, dataSizeOverheadBytes: 256, marginBps: 0})
        );

        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        uint256 quoted = sendLib.dvnFee(dvn, DST_EID, 12, OAPP, "");
        uint256 assigned = sendLib.assignDVN(dvn, param, "");

        require(quoted == 1.000010512 ether, "dvn data fee mismatch");
        require(assigned == quoted, "dvn assign/get fee mismatch");
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
        uint256 beforeBalance = address(this).balance;
        (bool funded,) = payable(address(dvn)).call{value: 1 ether}("");
        require(funded, "fund dvn failed");
        dvn.withdraw(payable(address(this)), 1 ether);
        require(address(this).balance == beforeBalance, "withdraw failed");
    }

    function test_dvnWithdrawFeeFromAllowedSendLib() public {
        ILayerZeroDVN.AssignJobParam memory param = ILayerZeroDVN.AssignJobParam({
            dstEid: DST_EID,
            packetHeader: hex"01020304",
            payloadHash: bytes32(uint256(1)),
            confirmations: 12,
            sender: OAPP
        });

        uint256 beforeBalance = address(this).balance;
        uint256 fee = sendLib.assignDVN(dvn, param, "");
        require(fee == 1.300013 ether, "unexpected dvn fee");
        require(sendLib.fees(address(dvn)) == fee, "dvn fee accounting missing");

        (bool funded,) = payable(address(sendLib)).call{value: fee}("");
        require(funded, "fund send lib failed");
        dvn.withdrawFee(address(sendLib), address(this), fee);

        require(address(this).balance == beforeBalance, "dvn send lib fee withdraw failed");
        require(sendLib.fees(address(dvn)) == 0, "dvn fee accounting not debited");
    }

    function test_workerWithdrawFeeRejectsUnauthorizedSendLib() public {
        expectRevert(
            address(executor),
            abi.encodeCall(executor.withdrawFee, (address(0xDEAD), address(this), uint256(1))),
            WorkerErrors.UnauthorizedSendLib.selector
        );
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

    function singleAddress(address value) internal pure returns (address[] memory values) {
        values = new address[](1);
        values[0] = value;
    }

    function setPriceSnapshot(OpenPriceFeed feed, uint32 dstEid, WorkerTypes.PriceSnapshot memory snapshot) internal {
        feed.setPriceSnapshot(singleUpdate(dstEid, snapshot));
    }

    function singleUpdate(uint32 dstEid, WorkerTypes.PriceSnapshot memory snapshot)
        internal
        pure
        returns (WorkerTypes.PriceSnapshotUpdate[] memory updates)
    {
        updates = new WorkerTypes.PriceSnapshotUpdate[](1);
        updates[0] = WorkerTypes.PriceSnapshotUpdate({dstEid: dstEid, snapshot: snapshot});
    }

    function expectRevert(address target, bytes memory callData, bytes4 selector) internal {
        (bool ok, bytes memory data) = target.call(callData);
        require(!ok, "expected revert");
        require(bytes4(data) == selector, "unexpected revert");
    }

    function defaultPathwayConfig() internal pure returns (WorkerTypes.PathwayConfig memory) {
        return WorkerTypes.PathwayConfig({
            enabled: true, maxMessageSize: 1024, minLzReceiveGas: 50_000, maxLzReceiveGas: 500_000
        });
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
