# 08 - Milestones and Acceptance Criteria

## M1 - Repo Scaffolding

Status: [ ]

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

Status: [ ]

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

Status: [ ]

Tasks:

- Postgres migrations
- config loader
- chain registry
- pathway registry
- worker contract registry
- startup validation

Acceptance:

- worker boots with Sepolia/Base Sepolia config
- invalid config fails fast
- DB migrations apply cleanly

## M4 - Signer and Tx Manager

Status: [ ]

Tasks:

- signer interface
- AWS KMS ECC_SECG_P256K1 signer
- rustack integration test
- geth keystore signer
- tx_outbox
- advisory lock nonce manager
- EIP-1559 transaction sender

Acceptance:

- KMS signer recovers expected Ethereum address
- keystore signer signs valid EIP-1559 tx
- tx_outbox assigns nonce without collisions
- replacement tx works in tests

## M5 - Executor Active Path

Status: [ ]

Tasks:

- PacketSent indexer
- ExecutorFeePaid indexer
- OpenExecutor event indexer
- packet decoder
- committer
- deliverer
- lzReceive tx builder

Acceptance:

- can deliver basic OFT send on Sepolia/Base Sepolia
- unsupported options are rejected or marked manual review
- failed delivery is retriable

## M6 - DVN Shadow Path

Status: [ ]

Tasks:

- DVN PacketSent indexer
- DVNFeePaid indexer
- OpenDVN event indexer
- confirmation wait
- RPC quorum verification
- payload hash computation
- would-verify report

Acceptance:

- shadow DVN observes packets
- waits 12 confirmations
- produces would-verify reports
- detects RPC conflicts
- does not submit active verification tx

## M7 - Price Bot

Status: [ ]

Tasks:

- Binance client
- Uniswap client
- aggregator
- deviation check
- gas price fetch
- setPriceConfig tx enqueue

Acceptance:

- updates Executor and DVN price config on testnet
- stops update when deviation exceeds threshold
- stale config causes contract quote revert

## M8 - Testnet Migration Rehearsal

Status: [ ]

Tasks:

- deploy all contracts
- configure OFT
- configure OpenExecutor
- switch Executor
- run canary transfer
- run DVN shadow
- pause + drain + DVN join
- rollback rehearsal

Acceptance:

- Executor migration succeeds both directions
- DVN join succeeds both directions
- `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`
- confirmations = 12
- OFT send resumes after unpause
- rollback procedure tested

## M9 - Mainnet Readiness Review

Status: [ ]

Tasks:

- security review
- runbook review
- monitoring checklist
- key management review
- rate-limit review
- dry-run config diff

Acceptance:

- no open critical issues
- mainnet runbook approved
- alerting requirements documented
- self-only DVN explicitly rejected for phase 1
