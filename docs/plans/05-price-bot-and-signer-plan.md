# 05 - Price Bot and Signer Plan

## Price Bot

Price bot 负责更新 worker contracts 的 `PriceConfig`。

第一阶段更新目标：

- `OpenExecutor.setPriceConfig`
- `OpenDVN.setPriceConfig`

## Data Sources

使用：

- Binance
- CoinMarketCap
- CoinGecko
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

- `primary_source` 默认为 Binance，也可配置为 CoinMarketCap 或 CoinGecko。
- Uniswap 是链上 sanity check。
- CoinMarketCap/CoinGecko 配置后可作为 primary 或额外 off-chain sanity check。
- 如果 primary 与任一 sanity source deviation 超过阈值，不更新并告警。
- 如果 primary 失败但 sanity source 健康，可以按配置 fallback。
- 如果无健康 source，不更新，等待 stale threshold 触发合约 quote revert。

## Update Frequency

- 定期 bot 更新
- gas spike 时立即更新
- 第一阶段 price bot 生成直接调用 `setPriceConfig` 的 tx outbox 请求，由 tx manager 使用配置 signer 广播
- `go run ./go/cmd/pricebot-once -config <worker.yaml>` 可执行一次价格计算并 enqueue OpenExecutor/OpenDVN 更新，用于测试网更新和人工演练

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

当前证据：

- `go/internal/signer/kms.Signer`
- `go/internal/signer/kms.TestParseDERSignatureRejectsTrailingBytes`
- `go/internal/signer/kms.TestSignHashRecoversExpectedAddress`
- `go/internal/signer/kms.TestSignHashNormalizesHighS`
- `docker-compose.integration.yml`
- `make test-integration`

## Local Keystore Requirements

- 支持 geth keystore JSON
- password 来自环境变量或 password file
- 不记录 private key material
- 支持 EIP-1559 transaction signing

当前证据：

- `go/internal/signer/keystore.Signer`
- `go/internal/signer/keystore.TestResolvePasswordSources`
- `go/internal/signer/keystore.TestSignerSignsEIP1559Transaction`

## Price Bot Evidence

- `go/internal/pricing.Bot.EnqueueOnce`
- `go/internal/pricing.Bot.EnqueueOnGasSpike`
- `go/internal/pricing.GasIncreaseBps`
- `go/internal/pricing.TestBotEnqueueOnGasSpikeQueuesOnlyAboveThreshold`
- `go/internal/config.PricingConfig.GasSpikeBps`
- `go/internal/configdiff.Diff`
