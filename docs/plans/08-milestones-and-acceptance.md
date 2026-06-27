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

Evidence:

- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsNonZeroLzReceiveValue`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsDuplicateLzReceiveOption`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsNativeDropOption`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsOrderedExecutionOption`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsMessageSize`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsWhenPaused`
- `npx hardhat test solidity`

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

Status: [x]

Tasks:

- [x] signer interface
- [x] AWS KMS ECC_SECG_P256K1 signer
- [x] rustack integration test
- [x] geth keystore signer
- [x] tx_outbox
- [x] advisory lock nonce manager
- [x] EIP-1559 transaction sender

Acceptance:

- [x] KMS signer recovers expected Ethereum address
- [x] keystore signer signs valid EIP-1559 tx
- [x] tx_outbox assigns nonce without collisions
- [x] replacement tx works in tests
- [x] failed tx retry requeues with a fresh nonce

Evidence:

- `go/internal/signer.Signer`
- `go/internal/signer/keystore.Signer`
- `go test ./go/internal/signer/keystore -count=1`
- `go/internal/signer/kms.Signer`
- `go/internal/signer/kms.TestParseDERSignatureRejectsTrailingBytes`
- `go test ./go/internal/signer/kms -count=1`
- `make test-integration`
- `docker-compose.integration.yml`
- `RUSTACK_KMS_ENDPOINT=http://localhost:4566 make test-kms-rustack`
- `go/internal/db.Store.EnqueueTx`
- `go/internal/db.Store.ClaimNextNonce`
- `go/internal/db.Store.ListBroadcastTx`
- `go/internal/db.Store.MarkTxConfirmed`
- `go/internal/db.Store.MarkTxFailed`
- `go/internal/db.Store.RetryFailedTx`
- `go/cmd/txretry`
- `go/internal/db.TestRetryFailedTxRequeuesWithFreshNonce`
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
- `go test ./go/cmd/txretry -count=1`

## M5 - Executor Active Path

Status: [~]

Tasks:

- [x] PacketSent indexer
- [x] ExecutorFeePaid indexer
- [x] OpenExecutor event indexer
- [x] packet decoder
- [x] committer
- [x] deliverer
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
- `go/internal/executor.TestIsCommitVerifiableRejectsEmptyPayloadHash`
- `go/internal/executor.IsLzReceiveExecutable`
- `go/internal/executor.Worker.ProcessCommitterOnce`
- `go/internal/executor.Worker.ProcessDelivererOnce`
- `go/internal/executor.TestProcessDelivererOnceRetriesFailedLzReceive`
- `contracts/scripts/send-oft-canary.ts`
- `contracts/scripts/oft-canary.test.ts` `buildLzReceiveOption encodes one zero-value executor lzReceive option`
- `contracts/scripts/oft-canary.test.ts` `buildCanarySendParam builds first-phase OFT send params`
- `go/internal/txmgr.Manager.ProcessReceipts`
- `go/internal/txmgr.TestProcessReceiptsMarksExecutorLzReceiveDelivered`
- `go/internal/txmgr.TestProcessReceiptsMarksExecutorLzReceiveFailed`
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

- [x] DVN PacketSent indexer
- [x] DVNFeePaid indexer
- [x] OpenDVN event indexer
- [x] confirmation wait
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
- `go/internal/lzabi.DecodePayloadVerified`
- `go/internal/indexer.DVNSourceTxRecordsFromLogs`
- `go/internal/indexer.ApplyDVNDestinationLogs`
- `go/internal/db.Store.UpsertDVNJob`
- `go/internal/db.Store.GetPacketByVerification`
- `go/internal/db.Store.ListDVNWork`
- `go/internal/db.Store.MarkDVNWaitingConfirmations`
- `go/internal/db.Store.MarkDVNQuorumChecking`
- `go/internal/db.Store.MarkDVNWouldVerify`
- `go/internal/db.Store.EnqueueDVNVerifyTx`
- `go/internal/db.Store.MarkDVNVerified`
- `go/internal/db.Store.MarkDVNQuorumConflict`
- `go/internal/db.Store.PausePathwayForPacket`
- `go/internal/db.Store.PauseChain`
- `go/internal/dvn.Worker.ProcessConfirmationsOnce`
- `go/internal/dvn.Worker.ProcessQuorumOnce`
- `go/internal/dvn.BuildVerifyTx`
- `go/internal/dvn.TestProcessQuorumOnceMarksWouldVerify`
- `go/internal/dvn.TestProcessQuorumOnceActiveEnqueuesVerifyTx`
- `go/internal/indexer.TestIndexerProcessOnceBackfillsDVNVerification`
- `go/internal/txmgr.TestProcessReceiptsMarksDVNVerifyTxVerified`
- `go/internal/rpcquorum.HeadConflictError`
- `go/internal/rpcquorum.IsHeadConflict`
- `go/internal/rpcquorum.Client.CheckHead`
- `go/internal/rpcquorum.ReceiptConflictError`
- `go/internal/rpcquorum.IsReceiptConflict`
- `go/internal/rpcquorum.Client.TransactionReceipt`
- `go/internal/rpcquorum.TestSelectCanonicalHeadAcceptsTwoOfThreeAgreement`
- `go/internal/rpcquorum.selectCanonicalHead`
- `go/internal/rpcquorum.receiptFingerprint`
- `go test ./go/internal/lzabi ./go/internal/indexer ./go/internal/db -count=1`
- `go test ./go/internal/dvn ./go/internal/rpcquorum -count=1`
- `TEST_POSTGRES_URL=... go test ./go/internal/db -count=1`

## M7 - Price Bot

Status: [~]

Tasks:

- [x] Binance client
- [x] CoinMarketCap client
- [x] CoinGecko client
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
- `go/internal/pricing.TestSelectPriceRejectsFallbackWhenDisabled`
- `go/internal/pricing.BinanceClient.PriceUSD`
- `go/internal/pricing.CoinMarketCapClient.PriceUSD`
- `go/internal/pricing.CoinGeckoClient.PriceUSD`
- `go/internal/pricing.UniswapV3Client.PriceUSD`
- `go/internal/pricing.BuildPriceConfig`
- `go/internal/pricing.BuildSetPriceConfigCalldata`
- `go/internal/pricing.BuildSetPriceConfigTx`
- `go/internal/pricing.Bot.EnqueueOnce`
- `go/internal/pricing.Bot.Run`
- `go/internal/app.App.RunPriceOnce`
- `go/cmd/pricebot-once`
- `package.json` `check:price-config`
- `contracts/scripts/price-config-check.ts`
- `contracts/scripts/price-config-check.test.ts`
- `docs/runbooks/price-bot.md`
- `go/internal/pricing/abis/price_config.json`
- `go/internal/pricing/abis/uniswap_v3_quoter.json`
- `go/internal/rpcquorum.Client.SuggestGasPrice`
- `go/internal/config.PricingConfig`
- `go/internal/app.App.priceBot`
- `go test ./go/internal/pricing ./go/internal/config ./go/internal/app -count=1`
- `npm run test:scripts`
- `go test ./go/cmd/pricebot-once -count=1`

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
- `package.json` `configure:lz-rollback`
- `package.json` `send:oft-canary`
- `package.json` `check:oft-canary`
- `package.json` `check:dvn-verification`
- `package.json` `check:migration-evidence`
- `package.json` `check:lz-addresses`
- `package.json` `check:deployment-preflight`
- `package.json` `oft:pathway`
- `contracts/scripts/lz-config.ts`
- `contracts/scripts/lz-addresses.ts`
- `contracts/scripts/deployment-preflight.ts`
- `contracts/scripts/oft-pathway-control.ts`
- `contracts/scripts/oft-canary.ts`
- `contracts/scripts/oft-canary-status.ts`
- `contracts/scripts/dvn-verification-status.ts`
- `contracts/scripts/migration-evidence.ts`
- `contracts/scripts/inspect-lz-config.ts`
- `contracts/scripts/configure-lz-executor.ts`
- `contracts/scripts/configure-lz-dvn.ts`
- `contracts/scripts/configure-lz-rollback.ts`
- `contracts/scripts/send-oft-canary.ts`
- `contracts/scripts/check-oft-canary.ts`
- `contracts/scripts/check-dvn-verification.ts`
- `contracts/scripts/check-lz-addresses.ts`
- `go/internal/db.Store.CheckDrainStatus`
- `go/cmd/draincheck`
- `go/cmd/txretry`
- `docs/deployments/testnet-migration-evidence.example.json`
- `docs/deployments/layerzero-testnet-addresses.md`
- `docs/deployments/test-oft-policy.md`
- `contracts/scripts/lz-config.test.ts`
- `contracts/scripts/lz-config.test.ts` `rollback config batches restore executor and both ULN configs`
- `contracts/scripts/lz-config.test.ts` `rollback config batches reject mismatched DVN counts`
- `contracts/scripts/oft-canary.test.ts`
- `contracts/scripts/oft-canary-status.test.ts`
- `contracts/scripts/dvn-verification-status.test.ts`
- `contracts/scripts/migration-evidence.test.ts`
- `contracts/scripts/lz-addresses.test.ts`
- `contracts/scripts/deployment-preflight.test.ts`
- `contracts/scripts/oft-pathway-control.test.ts`
- `go/internal/db.TestCheckDrainStatusReportsPendingWork`
- `go/internal/db.TestCheckDrainStatusAcceptsDeliveredShadowPathway`
- `package.json` `test:scripts`
- `npm run typecheck`
- `npm run test:scripts`
- `go test ./go/internal/db ./go/cmd/draincheck -count=1`

## M9 - Mainnet Readiness Review

Status: [~]

Tasks:

- [~] security review
- [x] runbook review
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
- `go/internal/readiness.Evaluate`
- `go/internal/readiness.TestEvaluateRejectsMissingOrUnstartedRequiredIndexerCursors`
- `go/cmd/readinesscheck`
- `go/internal/config.LoadStatic`
- `go/internal/configdiff.Diff`
- `go/cmd/configdiff`
- `docs/runbooks/monitoring.md`
- `docs/runbooks/config-diff.md`
- `docs/runbooks/key-management.md`
- `docs/runbooks/rate-limit.md`
- `docs/runbooks/mainnet-readiness.md`
- `contracts/scripts/runbook-review.ts`
- `npm run check:runbooks`
- `docs/security/security-review.md`
- `make security-check`
- `go test ./go/internal/metrics ./go/internal/db ./go/internal/app -count=1`
- `go test ./go/internal/readiness ./go/cmd/readinesscheck -count=1`
- `go test ./go/internal/config ./go/internal/configdiff ./go/cmd/configdiff -count=1`
- `go test ./go/internal/signer/keystore ./go/internal/signer/kms -count=1`
- `npx hardhat test solidity`
- `make security-check`
- `npm run check:npm-audit-disposition`
- `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`: 0 called Go vulnerabilities
- `package.json` retains `@nomicfoundation/hardhat-toolbox-viem`, depends on `viem` directly, and pins `overrides` for `axios`, `elliptic`, `undici`, and `ws`
- `docs/security/npm-audit-disposition.md`
- `npm audit --audit-level=moderate --json`: 0 critical, 6 high and 4 moderate remaining in pinned LayerZero and retained Hardhat toolbox transitive dependencies; all current high/moderate findings are tracked by `contracts/scripts/npm-audit-disposition.ts`, but M9 final approval remains open
