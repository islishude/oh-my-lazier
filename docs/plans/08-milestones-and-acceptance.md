# 08 - Milestones and Acceptance Criteria

## M1 - Repo Scaffolding

Status: [x]

Tasks:

- initialize monorepo
- initialize Hardhat V3
- initialize Go module
- add Docker Compose
- add Postgres migration runner
- add PLANS.md and plans directory
- pin npm dependencies

Acceptance:

- `npm test` runs empty contract test suite
- `go test ./...` runs empty Go test suite
- `docker compose up` starts Postgres and worker skeleton

## M2 - Contracts v1

Status: [x]

Tasks:

- TestOFT
- OFTPauseAndRateLimit
- OpenExecutor
- OpenDVN
- WorkerAccess
- WorkerOptions
- WorkerFeeLib
- PriceFeedStore
- deploy scripts
- config scripts
- inspect scripts

Acceptance:

- all contract unit tests pass
- gas option rejection tested
- stale price revert tested
- pause/rate-limit tested
- deployment scripts work on Sepolia/Base Sepolia

## M3 - DB and Config

Status: [x]

Tasks:

- Postgres migrations
- config loader
- chain registry
- pathway registry
- worker contract registry
- startup validation

Acceptance:

- [x] worker boots with Sepolia/Base Sepolia config
- [x] invalid config fails fast
- [x] DB migrations apply cleanly

Evidence:

- `go/migrations/001_initial_schema.sql`
- `go/internal/db.Store.Migrate`
- `go/internal/db.Store.SyncConfig`
- `go/internal/config.Config.Validate`
- `go/internal/chain.Registry`
- `go test ./...`
- `TEST_POSTGRES_URL=... go test ./go/internal/db -run TestMigrateAndSyncConfig -count=1`

## M4 - Signer and Tx Manager

Status: [~]

Tasks:

- [x] signer interface
- [x] AWS KMS ECC_SECG_P256K1 signer
- [~] rustack integration test
- [x] geth keystore signer
- [x] tx_outbox
- [x] advisory lock nonce manager
- [x] EIP-1559 transaction sender

Acceptance:

- [x] KMS signer recovers expected Ethereum address
- [x] keystore signer signs valid EIP-1559 tx
- [x] tx_outbox assigns nonce without collisions
- [x] replacement tx works in tests

Evidence:

- `go/internal/signer.Signer`
- `go/internal/signer/keystore.Signer`
- `go test ./go/internal/signer/keystore -count=1`
- `go/internal/signer/kms.Signer`
- `go test ./go/internal/signer/kms -count=1`
- `RUSTACK_KMS_ENDPOINT=http://localhost:4566 go test ./go/internal/signer/kms -run TestRustackKMSIntegrationSignsEthereumTransaction -count=1` 当前会 skip：`ghcr.io/tyrchen/rustack:latest` 的 KMS `CreateKey` 返回 `ECC_SECG_P256K1 is not supported`。
- `go/internal/db.Store.EnqueueTx`
- `go/internal/db.Store.ClaimNextNonce`
- `go/internal/db.Store.ListBroadcastTx`
- `go/internal/db.Store.MarkTxConfirmed`
- `go/internal/db.Store.MarkTxFailed`
- `TEST_POSTGRES_URL=... go test ./go/internal/db -count=1`
- `go/internal/config.SignerConfig`
- `go/internal/app.App.txTargets`
- `go/internal/app.TestTxTargetsLoadsKeystoreSignerForEveryChain`
- `go/internal/txmgr.Manager.Run`
- `go/internal/txmgr.Manager.ProcessNext`
- `go/internal/txmgr.Manager.ProcessReceipts`
- `go/internal/txmgr.TestProcessReceiptsMarksBroadcastTxConfirmed`
- `go/internal/txmgr.TestRunProcessesTargetsUntilQueueIsEmpty`
- `go/internal/rpcquorum.Client.PendingNonceAt`
- `go/internal/rpcquorum.Client.SendTransaction`
- `go/internal/rpcquorum.Client.TransactionReceipt`
- `TEST_POSTGRES_URL=... go test ./go/internal/txmgr -count=1`

## M5 - Executor Active Path

Status: [~]

Tasks:

- [~] PacketSent indexer
- [~] ExecutorFeePaid indexer
- [~] OpenExecutor event indexer
- [~] packet decoder
- [~] committer
- [~] deliverer
- [x] lzReceive tx builder

Acceptance:

- [ ] can deliver basic OFT send on Sepolia/Base Sepolia
- [x] unsupported options are rejected or marked manual review
- [x] failed delivery is retriable

Evidence:

- `go/internal/lz.DecodePacketV1`
- `go/internal/lzabi.DecodePacketSent`
- `go/internal/lzabi.DecodePacketVerified`
- `go/internal/lzabi.DecodePacketDelivered`
- `go/internal/lzabi.DecodeLzReceiveAlert`
- `go/internal/lzabi.DecodeExecutorFeePaid`
- `go/internal/lzabi.DecodeExecutorJobAssigned`
- `go/internal/indexer.PacketRecordFromSentLog`
- `go/internal/indexer.ExecutorJobFromAssignment`
- `go/internal/indexer.TestExecutorJobFromAssignmentMarksUnsupportedOptionsManualReview`
- `go/internal/indexer.TestIndexerProcessOnceMarksUnsupportedExecutorOptionsManualReview`
- `go/internal/indexer.ExecutorSourceTxRecordsFromLogs`
- `go/internal/indexer.Indexer.Run`
- `go/internal/indexer.Indexer.ProcessOnce`
- `go/internal/indexer.ApplyExecutorDestinationLogs`
- `go/internal/indexer.ApplyExecutorDestinationLog`
- `go/internal/lz.DecodeExecutorOptions`
- `go/internal/executor.BuildCommitVerificationTx`
- `go/internal/executor.BuildLzReceiveTx`
- `go/internal/executor.IsCommitVerifiable`
- `go/internal/executor.IsLzReceiveExecutable`
- `go/internal/executor.Worker.ProcessCommitterOnce`
- `go/internal/executor.Worker.ProcessDelivererOnce`
- `go/internal/executor.TestProcessDelivererOnceRetriesFailedLzReceive`
- `go/internal/rpcquorum.Client.CallContract`
- `go/internal/rpcquorum.Client.BlockNumber`
- `go/internal/rpcquorum.Client.FilterLogs`
- `go/internal/rpcquorum.Client.SubscribeFilterLogs`
- `go/migrations/001_initial_schema.sql` `indexer_cursors`
- `go/internal/db.Store.GetIndexerCursor`
- `go/internal/db.Store.UpdateIndexerCursor`
- `go/internal/db.Store.UpsertPacket`
- `go/internal/db.Store.GetPacket`
- `go/internal/db.Store.GetPacketByDestination`
- `go/internal/db.Store.UpsertExecutorJob`
- `go/internal/db.Store.ListExecutorWork`
- `go/internal/db.Store.EnqueueExecutorTx`
- `go/internal/db.Store.MarkExecutorCommitted`
- `go/internal/db.Store.MarkExecutorExecutable`
- `go/internal/db.Store.MarkExecutorDelivered`
- `go/internal/db.Store.MarkExecutorReceiveFailed`
- `go test ./go/internal/lz ./go/internal/lzabi ./go/internal/indexer ./go/internal/executor -count=1`
- `TEST_POSTGRES_URL=... go test ./go/internal/db -count=1`

## M6 - DVN Shadow Path

Status: [~]

Tasks:

- [~] DVN PacketSent indexer
- [~] DVNFeePaid indexer
- [~] OpenDVN event indexer
- [~] confirmation wait
- [x] RPC quorum verification
- [x] payload hash computation
- [x] would-verify report

Acceptance:

- shadow DVN observes packets
- waits 12 confirmations
- produces would-verify reports
- detects RPC conflicts
- does not submit active verification tx

Evidence:

- `go/internal/lzabi.DecodeDVNFeePaid`
- `go/internal/lzabi.DecodeDVNJobAssigned`
- `go/internal/indexer.DVNSourceTxRecordsFromLogs`
- `go/internal/db.Store.UpsertDVNJob`
- `go/internal/db.Store.ListDVNWork`
- `go/internal/db.Store.MarkDVNWaitingConfirmations`
- `go/internal/db.Store.MarkDVNQuorumChecking`
- `go/internal/db.Store.MarkDVNWouldVerify`
- `go/internal/db.Store.MarkDVNQuorumConflict`
- `go/internal/db.Store.PausePathwayForPacket`
- `go/internal/db.Store.PauseChain`
- `go/internal/dvn.Worker.ProcessConfirmationsOnce`
- `go/internal/dvn.Worker.ProcessQuorumOnce`
- `go/internal/rpcquorum.HeadConflictError`
- `go/internal/rpcquorum.IsHeadConflict`
- `go/internal/rpcquorum.Client.CheckHead`
- `go/internal/rpcquorum.ReceiptConflictError`
- `go/internal/rpcquorum.IsReceiptConflict`
- `go/internal/rpcquorum.Client.TransactionReceipt`
- `go/internal/rpcquorum.selectCanonicalHead`
- `go/internal/rpcquorum.receiptFingerprint`
- `go test ./go/internal/lzabi ./go/internal/indexer ./go/internal/db -count=1`
- `go test ./go/internal/dvn ./go/internal/rpcquorum -count=1`
- `TEST_POSTGRES_URL=... go test ./go/internal/db -count=1`

## M7 - Price Bot

Status: [~]

Tasks:

- [x] Binance client
- [x] Uniswap client
- [x] aggregator
- [x] deviation check
- [x] gas price fetch
- [x] setPriceConfig tx enqueue

Acceptance:

- updates Executor and DVN price config on testnet
- stops update when deviation exceeds threshold
- stale config causes contract quote revert

Evidence:

- `go/internal/pricing.SelectPrice`
- `go/internal/pricing.DeviationBps`
- `go/internal/pricing.BinanceClient.PriceUSD`
- `go/internal/pricing.UniswapV3Client.PriceUSD`
- `go/internal/pricing.BuildPriceConfig`
- `go/internal/pricing.BuildSetPriceConfigCalldata`
- `go/internal/pricing.BuildSetPriceConfigTx`
- `go/internal/pricing.Bot.EnqueueOnce`
- `go/internal/pricing.Bot.Run`
- `go/internal/pricing/abis/price_config.json`
- `go/internal/pricing/abis/uniswap_v3_quoter.json`
- `go/internal/rpcquorum.Client.SuggestGasPrice`
- `go/internal/config.PricingConfig`
- `go/internal/app.App.priceBot`
- `go test ./go/internal/pricing ./go/internal/config ./go/internal/app -count=1`

## M8 - Testnet Migration Rehearsal

Status: [~]

Tasks:

- [~] deploy all contracts
- [~] configure OFT
- [~] configure OpenExecutor
- [~] switch Executor
- run canary transfer
- [~] run DVN shadow
- [~] pause + drain + DVN join
- rollback rehearsal

Acceptance:

- Executor migration succeeds both directions
- DVN join succeeds both directions
- `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`
- confirmations = 12
- OFT send resumes after unpause
- rollback procedure tested

Evidence:

- `contracts/scripts/deploy-workers.ts`
- `contracts/scripts/configure-workers.ts`
- `contracts/scripts/README.md`
- `package.json` `deploy:workers`
- `package.json` `configure:workers`
- `package.json` `inspect:lz-config`
- `package.json` `configure:lz-executor`
- `package.json` `configure:lz-dvn`
- `contracts/scripts/lz-config.ts`
- `contracts/scripts/inspect-lz-config.ts`
- `contracts/scripts/configure-lz-executor.ts`
- `contracts/scripts/configure-lz-dvn.ts`
- `npm run typecheck`

## M9 - Mainnet Readiness Review

Status: [~]

Tasks:

- [~] security review
- [~] runbook review
- [x] monitoring checklist
- [x] key management review
- [x] rate-limit review
- [x] dry-run config diff

Acceptance:

- no open critical issues
- mainnet runbook approved
- alerting requirements documented
- self-only DVN explicitly rejected for phase 1

Evidence:

- `go/internal/metrics.Handler`
- `go/internal/metrics.RenderPrometheus`
- `go/internal/db.Store.Stats`
- `go/internal/config.LoadStatic`
- `go/internal/configdiff.Diff`
- `go/cmd/configdiff`
- `docs/runbooks/monitoring.md`
- `docs/runbooks/config-diff.md`
- `docs/runbooks/key-management.md`
- `docs/runbooks/rate-limit.md`
- `docs/runbooks/mainnet-readiness.md`
- `docs/security/parent-agent-security-review.md`
- `go test ./go/internal/metrics ./go/internal/db ./go/internal/app -count=1`
- `go test ./go/internal/config ./go/internal/configdiff ./go/cmd/configdiff -count=1`
- `go test ./go/internal/signer/keystore ./go/internal/signer/kms -count=1`
- `npx hardhat test solidity`
- `govulncheck ./...` via temporary `GOBIN`: 0 called Go vulnerabilities
- `npm audit --audit-level=moderate --json`: open critical/high transitive JavaScript toolchain advisories; M9 `no open critical issues` not satisfied
