# 05 - Price Bot and Signer Plan

## Price Bot

Price bot 负责更新 worker contracts 的 `PriceConfig`。

第一阶段更新目标：

- `OpenExecutor.setPriceConfig`
- `OpenDVN.setPriceConfig`

## Data Sources

使用：

- Binance
- Uniswap

## Price Formula

Worker 需要将目标链 gas price 转成源链 native token 单位。

```text
dstGasPriceInSrcToken =
  dstGasPriceInDstNative
  * price(dstNative/USD)
  / price(srcNative/USD)
```

Ethereum Sepolia 与 Base Sepolia 的 native asset 都近似 ETH，但抽象必须支持未来不同 native asset。

## Aggregation Policy

- Binance 是 primary。
- Uniswap 是 sanity check。
- 如果 Binance 与 Uniswap deviation 超过阈值，不更新并告警。
- 如果 Binance 失败但 Uniswap 健康，可以按配置 fallback。
- 如果无健康 source，不更新，等待 stale threshold 触发合约 quote revert。

## Update Frequency

- 定期 bot 更新
- gas spike 时立即更新
- 第一阶段 price bot 直接调用 `setPriceConfig`

## Stale Threshold

测试网初始值：

```text
30 minutes
```

如果 stale：

- `getFee` reverts
- `assignJob` reverts

## Signers

支持 signer 类型：

- AWS KMS ECC_SECG_P256K1
- local geth keystore JSON

Signer 按链配置选择。

## AWS KMS Signer Requirements

- 只支持 `ECC_SECG_P256K1`
- 使用 AWS KMS Sign API
- 解析 DER encoded signature
- extract r/s
- normalize low-S
- recover v
- validate recovered address
- 使用 rustack 做 AWS-compatible mock 集成测试

## Local Keystore Requirements

- 支持 geth keystore JSON
- password 来自环境变量或 password file
- 不记录 private key material
- 支持 EIP-1559 transaction signing
