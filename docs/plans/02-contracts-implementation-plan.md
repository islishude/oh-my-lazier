# 02 - Contracts Implementation Plan

## Tooling

使用：

```bash
npm install --save-exact \
  @layerzerolabs/lz-evm-protocol-v2@latest \
  @layerzerolabs/lz-evm-oapp-v2@latest \
  @layerzerolabs/lz-evm-messagelib-v2@latest \
  @openzeppelin/contracts@latest
```

Solidity：

```solidity
pragma solidity ^0.8.35;
```

要求：

- Hardhat V3
- TypeScript scripts
- 安装后将解析出的精确版本记录到本计划中
- 不复制 LayerZero interface，直接从固定版本 package import

## Contract Layout

```text
contracts/contracts/
  oft/
    TestOFT.sol
    OFTPauseAndRateLimit.sol

  workers/
    OpenExecutor.sol
    OpenDVN.sol

  common/
    WorkerAccess.sol
    WorkerErrors.sol
    WorkerFeeLib.sol
    WorkerOptions.sol
    WorkerTypes.sol
    PriceFeedStore.sol
```

## TestOFT.sol

### 目标

继承官方 LayerZero OFT 基类，添加 pause 与 rate limit 能力。

功能：

- per-destination send pause
- per-source receive pause
- per-destination outbound token bucket rate limit
- burn/mint OFT model

### API

```solidity
function pauseSend(uint32 dstEid, bool paused) external;
function pauseReceive(uint32 srcEid, bool paused) external;

function setOutboundRateLimit(
    uint32 dstEid,
    RateLimitConfig calldata config
) external;
```

### 验收标准

- `sendPaused[dstEid] == true` 时 send revert。
- `receivePaused[srcEid] == true` 时 receive revert。
- send 在 debit 前消耗 outbound rate limit。
- rate limit 基于 elapsed time 自动 refill。
- rate limit 可以设置为 0，用于迁移 drain。

## OpenExecutor.sol

### 目标

实现 LayerZero Executor fee quoting 与 job assignment。

### Interface

必须实现 `ILayerZeroExecutor`。

```solidity
function assignJob(
    uint32 dstEid,
    address sender,
    uint256 calldataSize,
    bytes calldata options
) external payable returns (uint256 price);

function getFee(
    uint32 dstEid,
    address sender,
    uint256 calldataSize,
    bytes calldata options
) external view returns (uint256 price);
```

### assignJob 约束

必须校验：

- `msg.sender` 是 allowed SendLib
- `(dstEid, sender)` pathway enabled
- `calldataSize <= maxMessageSize`
- price config fresh
- options 只包含 `lzReceiveOption`
- `lzReceiveGas >= minLzReceiveGas`
- `lzReceiveGas <= maxLzReceiveGas`
- `lzReceiveValue == 0`
- `msg.value >= quoted price`

第一阶段必须拒绝：

- `lzComposeOption`
- `lzNativeDropOption`
- `orderedExecutionOption`
- unknown option types

### Storage

```solidity
mapping(address sendLib => bool allowed) public allowedSendLib;

mapping(
    uint32 dstEid =>
    mapping(address sender => WorkerTypes.PathwayConfig config)
) public pathwayConfig;

mapping(uint32 dstEid => WorkerTypes.PriceConfig config) public priceConfig;
```

### Events

```solidity
event ExecutorJobAssigned(
    uint32 indexed dstEid,
    address indexed sender,
    address indexed sendLib,
    uint256 calldataSize,
    uint256 price,
    bytes options
);

event PathwayConfigSet(
    uint32 indexed dstEid,
    address indexed sender,
    WorkerTypes.PathwayConfig config
);

event PriceConfigSet(
    uint32 indexed dstEid,
    WorkerTypes.PriceConfig config
);
```

## OpenDVN.sol

### 目标

实现 LayerZero DVN fee quoting 与 assignment。

### Interface

必须实现 `ILayerZeroDVN`。

```solidity
function assignJob(
    AssignJobParam calldata param,
    bytes calldata options
) external payable returns (uint256 fee);

function getFee(
    uint32 dstEid,
    uint64 confirmations,
    address sender,
    bytes calldata options
) external view returns (uint256 fee);
```

`AssignJobParam` 必须从固定版本 `@layerzerolabs/lz-evm-messagelib-v2` 中 import。

### assignJob 约束

必须校验：

- `msg.sender` 是 allowed SendLib
- `(dstEid, sender)` pathway enabled
- message size <= maxMessageSize
- price config fresh
- DVN options 在第一阶段必须为空
- `msg.value >= quoted fee`

### Events

```solidity
event DVNJobAssigned(
    bytes32 indexed jobId,
    uint32 indexed dstEid,
    address indexed sender,
    address sendLib,
    uint64 confirmations,
    uint256 fee
);
```

## WorkerOptions.sol

第一阶段只接受：

- `lzReceiveOption`

第一阶段拒绝：

- `lzComposeOption`
- `lzNativeDropOption`
- ordered execution
- unknown option types

规则：

- 必须存在 exactly one `lzReceiveOption`
- duplicate `lzReceiveOption` revert
- `lzReceiveValue` 必须为 0
- unsupported option revert

## Contract Test Tasks

- Executor fee success
- Executor stale price revert
- Executor gas below min revert
- Executor gas above max revert
- Executor unsupported options revert
- Executor unauthorized SendLib revert
- Executor unauthorized OApp sender revert
- Executor message size revert
- Executor pause and withdraw tests
- DVN fee success
- DVN stale price revert
- DVN non-empty options revert
- DVN unauthorized SendLib revert
- DVN unauthorized OApp sender revert
- OFT pause tests
- OFT rate limit tests
