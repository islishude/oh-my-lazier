# 06 - Testing Plan

Status: [~]

Repo-local gates:

- `make check` runs contract compile, Solidity tests, Go tests, `golangci-lint`, `gofmt -l go`, and `forge fmt --check contracts`.
- `npm run typecheck` type-checks deployment and LayerZero configuration scripts.
- `TEST_POSTGRES_URL=... go test ./go/internal/db ./go/internal/txmgr -count=1` runs Postgres-backed state-machine and tx manager integration tests.
- `make test-integration` starts `docker-compose.integration.yml` with Postgres and Rustack, runs Postgres-backed DB/tx manager tests and the Rustack KMS signer integration test, then tears the containers down and removes `.tmp/integration`.

## Contract Unit Tests

### TestOFT

- deploy succeeds
- owner can configure peers
- send pause works
- receive pause works
- outbound rate limit works
- outbound rate limit refills
- rate limit zero blocks send
- unauthorized config reverts

### OpenExecutor

- `getFee` returns expected value
- stale price reverts
- unsupported sendLib reverts
- unsupported sender OApp reverts
- calldata too large reverts
- lzReceive gas below min reverts
- lzReceive gas above max reverts
- lzReceive value nonzero reverts
- lzCompose option reverts
- nativeDrop option reverts
- orderedExecution option reverts
- assignJob remains nonpayable to match the pinned `ILayerZeroExecutor` interface
- pause blocks assignJob
- withdraw only allowed role

Current evidence:

- `contracts/test/OpenWorkers.t.sol` `test_executorFeeSuccess`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsStalePrice`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsUnauthorizedSendLib`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsUnauthorizedOAppSender`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsMessageSize`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsGasBelowMinimum`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsGasAboveMaximum`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsNonZeroLzReceiveValue`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsUnsupportedOptions`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsNativeDropOption`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsOrderedExecutionOption`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsDuplicateLzReceiveOption`
- `contracts/test/OpenWorkers.t.sol` `test_executorRejectsWhenPaused`
- `contracts/test/OpenWorkers.t.sol` `test_executorWithdraw`

### OpenDVN

- `getFee` returns expected value
- stale price reverts
- unsupported sendLib reverts
- unsupported sender OApp reverts
- message size too large reverts
- non-empty DVN options revert
- assignJob requires sufficient msg.value
- pause blocks assignJob
- withdraw only allowed role

Current evidence:

- `contracts/test/OpenWorkers.t.sol` `test_dvnFeeSuccess`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsStalePrice`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsUnauthorizedSendLib`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsUnauthorizedOAppSender`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsMessageSize`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsNonEmptyOptions`
- `contracts/test/OpenWorkers.t.sol` `test_dvnAssignRejectsInsufficientFee`
- `contracts/test/OpenWorkers.t.sol` `test_dvnRejectsWhenPaused`
- `contracts/test/OpenWorkers.t.sol` `test_dvnWithdraw`

## Go Unit Tests

### rpcquorum

- 2-of-3 success
- provider lag detection
- lagging provider degraded
- block hash conflict pauses chain
- receipt/log conflict pauses pathway

Current evidence:

- `go/internal/rpcquorum.TestSelectCanonicalHeadAcceptsTwoOfThreeAgreement`
- `go/internal/rpcquorum.TestSelectCanonicalHeadIgnoresLaggingProvider`
- `go/internal/rpcquorum.TestSelectCanonicalHeadRejectsSameHeightHashConflict`
- `go/internal/rpcquorum.TestReceiptFingerprintIncludesLogEvidence`
- `go/internal/rpcquorum.TestIsReceiptConflict`

### signer

- AWS KMS DER parse
- low-S normalization
- v recovery
- recovered address validation
- geth keystore decrypt
- geth keystore sign tx

Current evidence:

- `go/internal/signer/kms.TestParseDERSignatureRejectsTrailingBytes`
- `go/internal/signer/kms.TestSignHashNormalizesHighS`
- `go/internal/signer/kms.TestSignHashRecoversExpectedAddress`
- `go/internal/signer/kms.TestSignHashRejectsWrongRecoveredAddress`
- `go/internal/signer/keystore.TestResolvePasswordSources`
- `go/internal/signer/keystore.TestSignerSignsEIP1559Transaction`

### txmgr

- advisory lock
- nonce assignment
- DB max nonce greater than RPC nonce
- RPC nonce greater than DB max nonce
- replacement tx
- receipt confirmation
- retry after failure
- drain readiness before migration config switch

Current evidence:

- `go/internal/db.TestClaimNextNonceAvoidsCollisions`
- `go/internal/txmgr.TestPrepareReplacementTxPreservesNonceAndBumpsFees`
- `go/internal/txmgr.TestProcessReceiptsMarksBroadcastTxConfirmed`
- `go/internal/db.TestRetryFailedTxRequeuesWithFreshNonce`
- `go/internal/db.TestCheckDrainStatusReportsPendingWork`
- `go/internal/db.TestCheckDrainStatusAcceptsDeliveredShadowPathway`
- `go/internal/txmgr.TestProcessReceiptsMarksExecutorLzReceiveDelivered`
- `go/internal/txmgr.TestProcessReceiptsMarksExecutorLzReceiveFailed`

### pricing

- Binance primary price
- CoinMarketCap primary or sanity price
- CoinGecko primary or sanity price
- Uniswap sanity check
- deviation threshold
- stale source handling
- setPriceConfig transaction enqueue

Current evidence:

- `go/internal/pricing.TestBinanceClientPriceUSD`
- `go/internal/pricing.TestCoinMarketCapClientPriceUSD`
- `go/internal/pricing.TestCoinGeckoClientPriceUSD`
- `go/internal/pricing.TestUniswapV3ClientPriceUSD`
- `go/internal/pricing.TestSelectPriceRejectsDeviationAboveThreshold`
- `go/internal/pricing.TestSelectPriceFallsBackWhenPrimaryUnavailable`
- `go/internal/pricing.TestSelectPriceRejectsFallbackWhenDisabled`
- `go/internal/pricing.TestBuildPriceConfigConvertsDestinationGasPriceToSourceToken`
- `go/internal/pricing.TestBuildSetPriceConfigTx`
- `go/internal/pricing.TestBotEnqueueOnceQueuesExecutorAndDVNPriceUpdates`
- `go/internal/pricing.TestBotEnqueueOnceRejectsDeviationWithoutEnqueue`

### executor

- PacketSent decode
- ExecutorFeePaid assignment
- commit verification tx build
- lzReceive tx build
- unsupported options cause `MANUAL_REVIEW`

Current evidence:

- `go/internal/lzabi.TestDecodePacketSent`
- `go/internal/lzabi.TestDecodeExecutorFeePaid`
- `go/internal/indexer.TestExecutorSourceTxRecordsFromLogs`
- `go/internal/executor.TestBuildCommitVerificationTx`
- `go/internal/executor.TestBuildLzReceiveTx`
- `go/internal/executor.TestBuildLzReceiveTxRejectsUnsupportedOptions`
- `go/internal/executor.TestIsCommitVerifiableRejectsEmptyPayloadHash`
- `go/internal/indexer.TestExecutorJobFromAssignmentMarksUnsupportedOptionsManualReview`
- `go/internal/indexer.TestIndexerProcessOnceMarksUnsupportedExecutorOptionsManualReview`

### dvn

- PacketSent decode
- DVNFeePaid assignment
- confirmation wait
- quorum verification
- payload hash computation
- shadow would-verify report

Current evidence:

- `go/internal/lzabi.TestDecodePacketSent`
- `go/internal/lzabi.TestDecodeDVNFeePaid`
- `go/internal/indexer.TestDVNSourceTxRecordsFromLogs`
- `go/internal/dvn.TestProcessConfirmationsOnceWaitsForSourceConfirmations`
- `go/internal/dvn.TestProcessConfirmationsOnceMarksQuorumChecking`
- `go/internal/dvn.TestProcessQuorumOnceMarksWouldVerify`
- `go/internal/dvn.TestProcessQuorumOnceMarksConflictOnMismatchedReceipt`
- `go/internal/dvn.TestProcessQuorumOnceMarksConflictOnRPCDisagreement`

### metrics

- `/healthz` does not require DB stats
- `/readyz` fails closed when DB stats are unavailable
- `/metrics` renders chain pause, pathway pause, packet, executor, DVN, tx outbox, and indexer cursor metrics

Current evidence:

- `go/internal/metrics.TestHandlerHealthDoesNotRequireStats`
- `go/internal/metrics.TestHandlerReadyReportsStatsFailure`
- `go/internal/metrics.TestHandlerMetricsRendersPrometheusSnapshot`

### configdiff

- static config loads ignore environment overrides
- chain, pathway, and pricing-chain diffs use semantic keys instead of list positions
- text renderer reports no-op and changed configs

Current evidence:

- `go/internal/config.TestLoadStaticIgnoresDatabaseURLEnvOverride`
- `go/internal/configdiff.TestDiffUsesSemanticKeysForLists`
- `go/internal/configdiff.TestRenderTextReportsNoConfigChanges`
- `go/internal/configdiff.TestRenderTextIncludesChangedPath`

## Testnet Integration Tests

目标：

- Ethereum Sepolia <-> Base Sepolia

流程：

1. Deploy TestOFT on both chains.
2. Deploy OpenExecutor on both chains.
3. Deploy OpenDVN on both chains.
4. Configure OFT peers.
5. Configure worker allowlist.
6. Configure price config.
7. Configure LayerZero ExecutorConfig to OpenExecutor.
8. Keep third-party DVN unchanged.
9. Send small OFT amount.
10. Confirm source `PacketSent`.
11. Confirm `ExecutorFeePaid` points to OpenExecutor.
12. Confirm worker commits verification.
13. Confirm worker calls `lzReceive`.
14. Confirm destination token balance increased with `npm run check:oft-canary` and `DESTINATION_TEST_OFT` / `RECIPIENT` / `MIN_RECIPIENT_BALANCE`.
15. Enable DVN shadow mode.
16. Confirm DVN would-verify report matches third-party verification.
17. Enable DVN active mode only after shadow evidence is accepted.
18. Confirm active mode enqueues `ReceiveUln302.verify` and the tx manager marks the DVN job `VERIFIED` after a successful receipt.
