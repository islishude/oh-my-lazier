# 01 - Project Scope and Fixed Decisions

## 目标

从零实现自托管 LayerZero V2 Executor 与 DVN stack，包括：

- Solidity worker contracts
- Custom OFT contract
- Go worker process
- Postgres-backed job state machine
- 多 RPC quorum verification
- Price updater
- AWS KMS 与本地 geth keystore signer
- 测试网部署与迁移流程

## 初始链与 pathway

第一阶段只支持 EVM-based 链：

- Ethereum Sepolia
- Base Sepolia

初始 pathway：

- Ethereum Sepolia -> Base Sepolia
- Base Sepolia -> Ethereum Sepolia

## LayerZero Worker 模型

DVN contract：

- 实现 `ILayerZeroDVN`
- 实现 `assignJob`
- 实现 `getFee`
- emit 内部 assignment event 方便对账
- 链下 worker 监听 `PacketSent` 与 `DVNFeePaid`

Executor contract：

- 实现 `ILayerZeroExecutor`
- 实现 `assignJob`
- 实现 `getFee`
- 链下 worker 监听 `PacketSent` 与 `ExecutorFeePaid`
- 链下 worker 实现 Committer 与 Deliverer workflow

## 安全模型

第一阶段目标配置：

```text
requiredDVNs = [OpenDVN, LayerZero Labs DVN]
optionalDVNs = []
optionalDVNThreshold = 0
confirmations = 12
```

不做 self-only DVN。

## OFT 模型

使用官方 LayerZero OFT 基类，并添加：

- per-destination send pause
- per-source receive pause
- per-destination outbound rate limit

Token 模型：

- 源链 burn
- 目标链 mint

第一阶段只支持基础 OFT send。

不支持：

- `composeMsg`
- `lzCompose`
- `lzNativeDrop`
- ordered execution
- non-EVM
- 外部第三方 OApp

## 费用模型

内部服务，目标是覆盖 gas 成本，不追求利润。

Executor fee：

```text
fee = baseFee + lzReceiveGas * dstGasPriceInSrcToken
fee = fee + bufferBps
```

DVN fee：

```text
fee = baseFee + bufferBps
```

建议初始配置：

```text
Executor:
  baseFee = 0.0001 source native token
  bufferBps = 3000

DVN:
  baseFee = 0.00005 source native token
  bufferBps = 3000

staleAfter:
  30 minutes on testnet
```

如果 price config stale，则 `getFee` 与 `assignJob` 必须 revert。

## 部署模型

第一阶段：

- Docker Compose
- Postgres
- 单 Go worker 进程
- 第一版 Compose 不包含 Prometheus/Grafana 容器
- Worker 可以暴露 Prometheus metrics endpoint，但不强制部署 Prometheus

## 配置模型

配置文件启动时加载。

第一阶段不做 hot reload，配置变更通过进程重启生效。
