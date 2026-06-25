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

## Phase 3 - Executor Migration

目标：

- 先切换 Executor。
- 保持现有第三方 DVN 配置不变。

每个方向执行：

1. 如有需要，暂停 test send。
2. 确认没有异常 in-flight packet。
3. 设置 LayerZero `ExecutorConfig.executor = OpenExecutor`。
4. 发送 canary OFT transfer。
5. 确认 `ExecutorFeePaid` 指向 OpenExecutor。
6. 确认 worker commit flow。
7. 确认 worker delivery flow。
8. 确认目标链余额增加。
9. 反方向重复。

Rollback：

- 将 `ExecutorConfig.executor` 重置为之前的 Executor。
- 对已经 verified 但未 delivered 的 packet 做 manual retry。

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
2. 等待所有 source-chain sent packets delivered。
3. 设置 source send ULN config，加入 OpenDVN + LayerZero Labs DVN。
4. 设置 destination receive ULN config，匹配 OpenDVN + LayerZero Labs DVN。
5. 验证 DVN address ordering。
6. 验证 confirmations = 12。
7. 发送 canary transfer。
8. 确认 OpenDVN verification submitted。
9. 确认 LayerZero Labs DVN verification observed。
10. 确认 Executor commits and delivers。
11. Unpause TestOFT send。
12. 反方向重复。

Rollback：

1. Pause send。
2. Drain 当前配置下生成的 packets。
3. 恢复之前的 send/receive DVN configs。
4. Canary transfer。
5. Unpause。

## Phase 6 - Gradual Rollout

测试网成功后：

- 编写主网 runbook
- 不迁移到 self-only DVN
- 保留独立第三方 DVN required
- 主网前加入完整监控与告警
