# LayerZero Self-Hosted DVN / Executor Implementation Plans

## 状态标识

- `[ ]` 未开始
- `[~]` 进行中
- `[x]` 已完成
- `[!]` 阻塞 / 需要决策

## 全局范围

本仓库用于从零实现一套自托管 LayerZero V2 Executor 与 DVN worker stack，第一阶段仅支持 EVM 链。

初始目标：

- Ethereum Sepolia <-> Base Sepolia
- 自定义 OFT，继承官方 OFT 基类
- burn/mint token 模型
- 基础 OFT send
- 不支持 `composeMsg`
- 不支持 `lzCompose`
- 不支持 native drop
- 不支持 ordered execution

安全模型：

- `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`
- 不做 self-only DVN
- confirmations = 12
- DVN 使用多 RPC quorum 做源链事件与 receipt 校验
- DVN 迁移使用 pause + drain + switch config 方案

Worker 模型：

- Go monorepo
- 第一阶段单进程同时处理 Ethereum Sepolia 与 Base Sepolia
- Docker Compose 部署
- Compose 第一版只包含 Postgres + worker
- 配置启动时加载，修改后重启生效

合约模型：

- Hardhat V3
- Solidity `^0.8.35`
- OpenZeppelin v5
- LayerZero 依赖安装时使用 latest，但保存为精确版本：
  - `@layerzerolabs/lz-evm-protocol-v2`
  - `@layerzerolabs/lz-evm-oapp-v2`
  - `@layerzerolabs/lz-evm-messagelib-v2`

## 计划索引

| 状态 | 文件                                        | 用途                                  |
| ---- | ------------------------------------------- | ------------------------------------- |
| [x]  | `plans/01-project-scope-and-decisions.md`   | 固定决策、非目标与假设                |
| [x]  | `plans/02-contracts-implementation-plan.md` | Solidity 合约实现计划                 |
| [~]  | `plans/03-go-worker-implementation-plan.md` | Go worker 实现计划                    |
| [x]  | `plans/04-database-and-config-plan.md`      | Postgres schema 与配置文件            |
| [~]  | `plans/05-price-bot-and-signer-plan.md`     | Price updater、AWS KMS、本地 keystore |
| [ ]  | `plans/06-testing-plan.md`                  | 单元测试、集成测试、测试网验证        |
| [~]  | `plans/07-deployment-and-migration-plan.md` | Sepolia/Base Sepolia 部署与迁移       |
| [~]  | `plans/08-milestones-and-acceptance.md`     | 里程碑、任务与验收标准                |

## 推荐执行顺序

1. 初始化 monorepo、Hardhat V3、Go module、Docker Compose。
2. 安装并精确固定 LayerZero / OpenZeppelin 依赖。
3. 实现 TestOFT、OFTPauseAndRateLimit、OpenExecutor、OpenDVN。
4. 实现 Postgres schema、config loader、chain/pathway registry。
5. 实现 signer 与 tx manager。
6. 实现 Executor active path。
7. 实现 DVN shadow path。
8. 在 Ethereum Sepolia 与 Base Sepolia 部署并配置。
9. 切换测试网 Executor 到 OpenExecutor。
10. 运行 OpenDVN shadow。
11. 将 OpenDVN 与 LayerZero Labs DVN 共同加入 required DVNs。
12. 完成迁移演练并编写主网 runbook。

## 推荐执行顺序状态

| 状态 | 步骤                                                           | 当前证据                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| ---- | -------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [x]  | 1. 初始化 monorepo、Hardhat V3、Go module、Docker Compose      | `package.json`、`hardhat.config.ts`、`go.mod`、`docker-compose.yml`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| [x]  | 2. 安装并精确固定 LayerZero / OpenZeppelin 依赖                | `package-lock.json`，精确版本记录在 `plans/02-contracts-implementation-plan.md`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| [x]  | 3. 实现 TestOFT、OFTPauseAndRateLimit、OpenExecutor、OpenDVN   | `contracts/contracts/oft/*`、`contracts/contracts/workers/*`，`contracts/test/OpenWorkers.t.sol`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| [x]  | 4. 实现 Postgres schema、config loader、chain/pathway registry | `go/migrations/001_initial_schema.sql`、startup migration runner、config validation、chain/pathway registry、config sync                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| [~]  | 5. 实现 signer 与 tx manager                                   | signer interface、本地 keystore signer、AWS KMS signer、tx manager loop、tx_outbox enqueue、advisory-lock nonce assignment、EIP-1559 签名广播与 replacement tx 已有；rustack KMS 集成测试待后续步骤                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| [~]  | 6-12. Executor active path 到主网 runbook                      | M5 source-chain `PacketSent` / `ExecutorFeePaid` / `ExecutorJobAssigned` 解码、destination `PacketVerified` / `PacketDelivered` / `LzReceiveAlert` 解码与状态应用、同交易关联校验、packet/job upsert、confirmed-window polling backfill、持久 indexer cursor、live subscription wakeup、严格 executor options 解码、commit/lzReceive tx builder、DB 原子 enqueue/status/receipt transition、committer/deliverer one-shot 处理、EndpointV2/ReceiveUln302 readiness eth_call 已有；M6 DVN `PacketSent` / `DVNFeePaid` / `DVNJobAssigned` 解码、关联校验、`dvn_jobs` 持久化、confirmation wait transition、source head/receipt/log verification、healthy-provider head/receipt comparison、RPC conflict persistence、block-hash conflict chain pause、receipt/log conflict pathway pause 与 shadow `WOULD_VERIFY` report 已有；测试网验证与告警闭环待后续阶段 |
| [~]  | 7. 实现 Price Bot                                              | Binance public ticker client、Uniswap V3 quoter sanity client、RPC gas price fetch、price aggregator、deviation check、dst gas price to src token formula、`setPriceConfig` calldata/outbox request builder、pricing config、定时 enqueue loop 已有；测试网实际更新待后续阶段                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| [~]  | 8. 在 Ethereum Sepolia 与 Base Sepolia 部署并配置              | repo-local viem 部署脚本、OFT peer/限流、worker SendLib/pathway/price config 脚本、LayerZero config inspect、ExecutorConfig 切换脚本、双 required DVN ULNConfig 脚本已添加；真实测试网部署与配置执行待后续阶段                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| [~]  | 12. 完成迁移演练并编写主网 runbook                             | DB-backed `/healthz`/`/readyz`/`/metrics` endpoint、monitoring checklist、validated config diff CLI/runbook、key management review、rate-limit review、mainnet readiness runbook、parent-agent security review 已有；npm audit critical/high toolchain advisories、exhaustive security review 与最终主网 approval 待后续阶段                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

## 当前开放项

- 已执行 `npm install --save-exact ...@latest`，当前精确版本记录在 `plans/02-contracts-implementation-plan.md`。
- 从 LayerZero deployed contracts 表确认 Ethereum Sepolia 与 Base Sepolia 的 EndpointV2、SendUln302、ReceiveUln302、Executor、DVN 地址。
- 确认 LayerZero Labs DVN 在两条测试网的地址。
- 确认 TestOFT 的 name、symbol、initial supply、owner、minting policy。
