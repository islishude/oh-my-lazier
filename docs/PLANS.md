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
| [ ]  | `plans/01-project-scope-and-decisions.md`   | 固定决策、非目标与假设                |
| [ ]  | `plans/02-contracts-implementation-plan.md` | Solidity 合约实现计划                 |
| [ ]  | `plans/03-go-worker-implementation-plan.md` | Go worker 实现计划                    |
| [ ]  | `plans/04-database-and-config-plan.md`      | Postgres schema 与配置文件            |
| [ ]  | `plans/05-price-bot-and-signer-plan.md`     | Price updater、AWS KMS、本地 keystore |
| [ ]  | `plans/06-testing-plan.md`                  | 单元测试、集成测试、测试网验证        |
| [ ]  | `plans/07-deployment-and-migration-plan.md` | Sepolia/Base Sepolia 部署与迁移       |
| [ ]  | `plans/08-milestones-and-acceptance.md`     | 里程碑、任务与验收标准                |

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

## 当前开放项

- 执行 `npm install --save-exact ...@latest` 后，记录 LayerZero 三个包的精确版本。
- 从 LayerZero deployed contracts 表确认 Ethereum Sepolia 与 Base Sepolia 的 EndpointV2、SendUln302、ReceiveUln302、Executor、DVN 地址。
- 确认 LayerZero Labs DVN 在两条测试网的地址。
- 确认 TestOFT 的 name、symbol、initial supply、owner、minting policy。
