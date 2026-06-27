# 07 - Deployment and Migration Plan

## Phase 1 - Dependency Pinning

执行：

```bash
npm install --save-exact \
  @layerzerolabs/lz-evm-protocol-v2@latest \
  @layerzerolabs/lz-evm-oapp-v2@latest \
  @layerzerolabs/lz-evm-messagelib-v2@latest \
  @openzeppelin/contracts@5
```

提交：

- package.json
- package-lock.json

并将精确版本记录到 `plans/01-project-scope-and-decisions.md`。

## Phase 2 - Contract Deployment

部署到 Ethereum Sepolia 与 Base Sepolia：

- TestOFT
- OpenExecutor
- OpenDVN

配置：

- OFT peers
- OFT pause/rate limit
- worker allowed SendLibs
- worker allowed OApp senders
- worker maxMessageSize
- worker min/max lzReceive gas
- price configs

当前仓库证据：

- `contracts/scripts/deploy-workers.ts` 部署 TestOFT、OpenExecutor、OpenDVN。
- `contracts/scripts/configure-workers.ts` 配置 OFT peer、可选 outbound rate limit、worker SendLib 白名单、pathway limits 与 price config。
- `contracts/scripts/inspect-lz-config.ts` 读取当前 Endpoint send/receive library 与 SendUln302/ReceiveUln302 configs。
- `contracts/scripts/configure-lz-executor.ts` 通过 Endpoint `setConfig` 写入 SendUln302 `ExecutorConfig`。
- `contracts/scripts/configure-lz-dvn.ts` 通过 Endpoint `setConfig` 写入 SendUln302 与 ReceiveUln302 `UlnConfig`，并显式设置 `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`、`optionalDVNCount = NIL`。
- `contracts/scripts/configure-lz-rollback.ts` 使用 `inspect:lz-config` 保存的 JSON 快照重放旧 `ExecutorConfig`、SendUln302 `UlnConfig` 与 ReceiveUln302 `UlnConfig`，作为本地 rollback 输入。
- `contracts/scripts/check-oft-canary.ts` 检查 canary source receipt 的 EndpointV2 `PacketSent`、SendLib `ExecutorFeePaid.executor = OpenExecutor`，并可独立检查 destination receipt 的 `PacketDelivered`/`LzReceiveAlert` 与 recipient TestOFT 最小余额。
- `contracts/scripts/check-dvn-verification.ts` 检查 destination-chain ReceiveUln302 `PayloadVerified` 是否包含 OpenDVN 与 LayerZero Labs DVN，并可要求同 receipt 内存在 EndpointV2 `PacketVerified`。
- `contracts/scripts/check-lz-addresses.ts` 从 LayerZero 官方 metadata 重新核对本地记录的 Sepolia/Base Sepolia EndpointV2、ULN302、Executor、LayerZero Labs DVN 地址。
- `contracts/scripts/deployment-preflight.ts` 只读检查 TestOFT/OpenExecutor/OpenDVN `owner()`、owner native balance、可选 canary treasury native/TestOFT balance 与 TestOFT totalSupply。
- `contracts/scripts/oft-pathway-control.ts` 检查或执行 TestOFT `pauseSend`、`pauseReceive`、zero-capacity drain 与 steady-state outbound rate-limit 更新，并在写入后回读确认。
- `contracts/scripts/price-config-check.ts` 只读检查 OpenExecutor/OpenDVN `priceConfig(dstEid)` 的 fresh `updatedAt`、非零 `dstGasPriceInSrcToken` 与可选 `staleAfter` 预期值。
- `contracts/scripts/migration-evidence.ts` 校验迁移记录 JSON 是否包含 `make check`、LayerZero 地址刷新、DB readiness、key/price/rate-limit/monitoring/runbook/security review、Ethereum Sepolia `40161` <-> Base Sepolia `40245` 双向 direction、非零 owner/signer/canary EVM 地址、config diff、preflight、LayerZero config before/after、price config、drain、canary 金额/发送账户/接收账户/最小到账余额/source receipt/destination receipt/余额检查、`confirmations = 12` 且 `requiredDVNs = [OpenDVN, LayerZero Labs DVN]` 的 DVN join、DVN verification、rollback 后 config/canary 与 manual retry 证据引用。
- `docs/deployments/testnet-migration-evidence.example.json` 提供 Ethereum Sepolia <-> Base Sepolia 双向迁移记录模板。
- `docs/deployments/layerzero-testnet-addresses.md` 记录 Ethereum Sepolia 与 Base Sepolia 的 EndpointV2、SendUln302、ReceiveUln302、LayerZero Executor、LayerZero Labs DVN 地址。
- `docs/deployments/test-oft-policy.md` 固定 TestOFT name、symbol、constructor mint、owner 与 post-deploy minting policy。
- `go run ./go/cmd/pricebot-once -config <worker.yaml>` 可用 worker 配置读取价格源和 RPC gas price，并为 OpenExecutor/OpenDVN enqueue 一次 `setPriceConfig` 更新。
- `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>` 汇总 pathway 下未完成 executor job、DVN job、tx outbox 与 verified-but-undelivered 计数，作为切换 Executor/DVN config 前的 drain 门禁。
- `go run ./go/cmd/txretry -config <worker.yaml> -action retry-failed|replace -id <tx_outbox_id>` 支持 rollback/manual retry 时重新排队 failed tx，或对已分配 nonce 的 tx 准备 replacement fee bump。
- `contracts/scripts/README.md` 记录脚本环境变量与本地方向配置方式。
- `npm run typecheck` 覆盖部署与配置脚本类型检查。

待执行：

- 测试网执行前运行 `npm run check:lz-addresses`，重新核对 `docs/deployments/layerzero-testnet-addresses.md` 的外部 LayerZero 地址。
- 按 `docs/deployments/test-oft-policy.md` 设置 TestOFT `TOKEN_NAME`、`TOKEN_SYMBOL`、`OWNER`、`INITIAL_RECIPIENT` 与 `INITIAL_SUPPLY`。
- 分别使用 Ethereum Sepolia 与 Base Sepolia 的 EndpointV2、SendUln302、eid、RPC 与 owner/signer 环境变量运行部署脚本。
- 部署后运行 `npm run check:deployment-preflight`，确认 TestOFT/OpenExecutor/OpenDVN owner 符合预期，owner 有足够 native token，canary treasury 有计划 canary 所需的 native token 与 TestOFT 余额。
- 两条链部署完成后，按方向运行配置脚本，确保 remote OFT 与 local SendLib 地址匹配。
- 迁移 pause/drain/unpause 阶段使用 `npm run oft:pathway` 执行并回读 TestOFT pathway 状态。
- 切换 Executor/DVN config 前运行 `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>`，确认 `ready: true`。
- 迁移阶段执行前后都运行 `npm run inspect:lz-config`，记录旧 Executor/DVN config 以支持 rollback。
- 每次修改 worker YAML 前运行 `go run ./go/cmd/configdiff -from <current.yaml> -to <proposed.yaml>`，将输出附到迁移记录。
- 执行 worker price config 更新前后按 `docs/runbooks/price-bot.md` 记录价格源、outbox 交易、receipt，并运行 `npm run check:price-config` 验证链上 `priceConfig(dstEid)`。
- 执行 pause/drain/rate-limit 前按 `docs/runbooks/rate-limit.md` 记录容量、refill、canary size 与 owner 可用性。
- 执行 signer 变更前按 `docs/runbooks/key-management.md` 记录 signer inventory、KMS key spec 或 keystore password source、rollback signer。
- 批准迁移记录前运行 `MIGRATION_EVIDENCE=<record.json> npm run check:migration-evidence`，确认双向迁移、结构化 canary 证据和 rollback 证据引用齐全。

## Phase 3 - Executor Migration

目标：

- 先切换 Executor。
- 保持现有第三方 DVN 配置不变。

每个方向执行：

1. 如有需要，暂停 test send。
2. 运行 `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>`，确认没有异常 in-flight packet。
3. 设置 LayerZero `ExecutorConfig.executor = OpenExecutor`。
4. 发送 canary OFT transfer。
5. 使用 `npm run check:oft-canary` 确认 `ExecutorFeePaid` 指向 OpenExecutor。
6. 确认 worker commit flow。
7. 确认 worker delivery flow。
8. 使用 `npm run check:oft-canary` 检查 destination `PacketDelivered` 且无 `LzReceiveAlert`，并通过 `DESTINATION_TEST_OFT` / `RECIPIENT` / `MIN_RECIPIENT_BALANCE` 确认目标链余额达到 canary 预期。
9. 反方向重复。

Rollback：

- 将 `ExecutorConfig.executor` 重置为之前的 Executor。
- 用 `draincheck` 的 `verified_but_undelivered_count` 定位已经 verified 但未 delivered 的 packet，并用 `go run ./go/cmd/txretry` 对相关 `tx_outbox` 行执行 `retry-failed` 或 `replace`。

当前仓库证据：

- `npm run configure:lz-executor` 使用 pinned `ILayerZeroEndpointV2` artifact 调用 Endpoint `setConfig`，只修改指定 OApp、remote eid 与 SendUln302 的 `ExecutorConfig`。
- `npm run inspect:lz-config` 可在切换前后输出 active send/receive library 与 decoded Executor/ULN config，作为 rollback 输入。
- `npm run configure:lz-rollback` 可从切换前保存的 `inspect:lz-config` JSON 恢复旧 ExecutorConfig 与 ULN config。

## Phase 4 - DVN Shadow Mode

目标：

- 运行 OpenDVN 链下流程，但不提交 verification tx。

步骤：

1. 保持 DVN config 不变。
2. 启动 DVN worker shadow mode。
3. Index `PacketSent`。
4. 匹配 `DVNFeePaid`。
5. 等待 12 confirmations。
6. 运行 RPC quorum verification。
7. 计算 payload hash。
8. 与第三方 DVN 实际验证结果对比。
9. 记录 would-verify latency。

## Phase 5 - DVN Join

目标：

```text
requiredDVNs = [OpenDVN, LayerZero Labs DVN]
confirmations = 12
```

迁移策略：

- pause
- drain
- switch config
- canary
- unpause

每个方向执行：

1. Pause TestOFT send for target `dstEid`。
2. 运行 `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>`，等待所有 source-chain sent packets delivered。
3. 设置 source send ULN config，加入 OpenDVN + LayerZero Labs DVN。
4. 设置 destination receive ULN config，匹配 OpenDVN + LayerZero Labs DVN。
5. 验证 DVN address ordering。
6. 验证 confirmations = 12。
7. 发送 canary transfer。
8. 使用 `npm run check:dvn-verification` 确认 OpenDVN verification submitted。
9. 使用 `npm run check:dvn-verification` 确认 LayerZero Labs DVN verification observed。
10. 确认 Executor commits and delivers。
11. Unpause TestOFT send。
12. 反方向重复。

Rollback：

1. Pause send。
2. Drain 当前配置下生成的 packets。
3. 恢复之前的 send/receive DVN configs。
4. Canary transfer。
5. Unpause。

当前仓库证据：

- `npm run oft:pathway` 可对指定 `REMOTE_EID` 执行 `pause-send`、`unpause-send`、`pause-receive`、`unpause-receive`、`drain`、`set-rate-limit` 或 `inspect`，并回读状态确认。
- `npm run configure:lz-dvn` 使用 pinned `ILayerZeroEndpointV2` artifact 调用 Endpoint `setConfig`，在本地链的 SendUln302 与 ReceiveUln302 上写入同一组 required DVNs。
- DVN 地址会按 LayerZero `UlnConfig` 要求升序排序，并拒绝重复地址。
- `optionalDVNCount` 写入 LayerZero NIL 值，避免 first-phase 迁移继承默认 optional DVNs。
- `npm run configure:lz-rollback` 可恢复旧 SendUln302/ReceiveUln302 UlnConfig，包括非 first-phase 的 optional DVN/default count 字段。

## Phase 6 - Gradual Rollout

测试网成功后：

- 编写主网 runbook
- 不迁移到 self-only DVN
- 保留独立第三方 DVN required
- 主网前加入完整监控与告警
