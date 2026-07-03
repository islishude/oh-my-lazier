# oh-my-lazier

Self-hosted LayerZero V2 Executor and DVN worker stack for the initial Ethereum Sepolia <-> Base Sepolia EVM pathway.

The first implementation pass includes:

- Hardhat V3 Solidity workspace using Solidity `^0.8.35`.
- Fixed LayerZero and OpenZeppelin package versions in `package-lock.json`.
- `TestOFT` with burn/mint OFT behavior, per-pathway send/receive pause, and outbound token-bucket rate limits.
- `OpenExecutor` and `OpenDVN` contracts with allowed SendLib checks, pathway gating, stale price rejection, message-size limits, option validation, and assignment events.
- Go worker packages with config loading, chain registry, Postgres connection, metrics health endpoint, tx manager loop, role-aware indexer streams, optional executor/DVN loops, pathway DVN mode wiring, signer interfaces, and state enums.
- Docker Compose for Postgres plus the worker process.

## Repository Layout

```text
contracts/contracts/   Solidity contracts
contracts/scripts/     Deployment and operations scripts
contracts/test/        Contract tests
go/                    Go worker package tree
config/                Worker config examples
docs/                  Runbooks, deployment policies, security records, and migration evidence
```

## Tooling

Use Node.js 26+ and Go 1.26+.

```bash
npm install
make check
```

Common targets:

```bash
make compile        # Hardhat compile
make generate-lzabi # regenerate committed Go LayerZero ABI JSON
make check-lzabi    # verify generated Go LayerZero ABI JSON has no drift
make generate-pricing-abi # regenerate committed Go pricing ABI JSON
make check-pricing-abi    # verify generated Go pricing ABI JSON has no drift
make test-solidity  # Solidity tests
make test-go        # Go package tests
make test-integration # Postgres and Rustack-backed integration tests
make e2e-local      # Docker Compose Postgres/worker plus two local Anvil chains
make security-check # security doc/log guard, npm audit disposition, and Go vulnerability check
make runbook-check  # runbook coverage guard
make migration-evidence-check # validates the example migration evidence record
make lint-go        # golangci-lint
make fmt-go         # gofmt
make fmt-sol        # format solidity files
```

`npm run check` remains available for compile plus tests without the Go linter.
CI mirrors the local gates with contract/script/runbook/migration-evidence
checks, Go tests and lint, Docker image build validation, a dedicated
`make e2e-local` dual-Anvil job, and a dedicated `make security-check` job.

The Go worker embeds compact LayerZero ABI JSON from `go/internal/lzabi/abis`
and on-chain config-check ABI JSON from `go/internal/configcheck/abis`.
Regenerate those files with `make generate-lzabi` after changing local worker
contracts, config-check getters, or updating pinned LayerZero packages. The
generator reads local Hardhat artifacts for `OpenDVN`, `OpenExecutor`, and
`TestOFT`, and pinned LayerZero package artifacts for EndpointV2,
ReceiveUln302, and SendUln302 events. `make check` includes `make check-lzabi`
so ABI drift is caught before handoff.

The Go pricing package also embeds compact ABI JSON from
`go/internal/pricing/abis`. Regenerate it with `make generate-pricing-abi` after
worker price-config contract changes or pinned Uniswap package updates. The
generator reads local Hardhat artifacts for worker `setPriceConfig` calldata and
`@uniswap/v3-periphery` for the Uniswap V3 quoter interface. `make check`
includes `make check-pricing-abi`.

## Local Worker

Start Postgres and the worker with:

```bash
docker compose up
```

Run the maintained local two-chain E2E with:

```bash
make e2e-local
```

The E2E target uses `docker-compose.e2e.yml` with isolated ports and
`tmp/e2e` artifacts, starts Postgres, two Anvil chains, and the worker, deploys
local EndpointV2, SendUln302, ReceiveUln302, TestOFT, OpenExecutor, primary
OpenDVN, and secondary OpenDVN contracts, writes a generated worker config and
keystore, skips the price bot, and uses fresh hard-coded price configs. The
canary runner sends OFT messages in both directions and requires source
`PacketSent`/worker fee events, destination `PayloadVerified` events from both
OpenDVNs, `PacketDelivered`, and the recipient TestOFT balance increase.
Set `ANVIL_IMAGE` to use a prebuilt local Foundry image, or set
`E2E_WORKER_UP_FLAGS=--no-build` after tagging a compatible
`oh-my-lazier-worker:e2e` image, when registry access is unavailable.

The example worker config is [config/example.yaml](config/example.yaml). `DATABASE_URL` can override the configured database URL at runtime.

Build the worker image locally with:

```bash
make docker-build
```

Smoke-test the image entrypoint without loading worker config:

```bash
make docker-smoke
```

The image runs `/usr/local/bin/worker` and defaults to `/app/config/example.yaml`. CI builds and smoke-tests the local worker image. The release publishing path builds native amd64 and arm64 images on separate GitHub runners, pushes them by digest, and then creates a multi-arch manifest with `docker buildx imagetools`.

Before starting durable loops or enqueuing price updates, the worker runs an
on-chain config check against the configured RPC endpoints. The check confirms
every configured RPC URL reports the configured EVM chain ID, then confirms
LayerZero endpoint IDs, deployed contract code, OApp peers, active
send/receive libraries, source SendUln required DVNs, destination ReceiveUln
required DVNs, pathway-level source OpenExecutor/OpenDVN settings, destination
OpenDVN code, and active DVN verifier authorization. Operators can run the
same gate directly:

```bash
go run ./go/cmd/configcheck -config config/example.yaml
```

## Current Scope

The initial target is Ethereum Sepolia <-> Base Sepolia with:

- `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`
- explicitly configured source-chain confirmations that match the approved LayerZero ULN configuration
- basic OFT send only
- no `composeMsg`
- no `lzCompose`
- no native drop
- no ordered execution

The installed LayerZero `ILayerZeroExecutor` interface has a nonpayable `assignJob`, so `OpenExecutor` currently quotes and emits the assignment price while remaining interface-compatible. `OpenDVN` remains payable and enforces `msg.value >= fee`.

## Contract Behavior Notes

- Executor options must be LayerZero type-3 options containing exactly one executor `lzReceive` option.
- `lzReceive` value must be zero. Compose, native drop, ordered execution, unknown executor options, and other worker IDs are rejected.
- OFT send pause is keyed by destination endpoint ID; receive pause is keyed by source endpoint ID.
- OFT outbound rate limits are token buckets. Unconfigured pathways are unrestricted, while an explicitly configured `capacity=0` and `refillPerSecond=0` pathway is a migration drain setting that rejects new sends.
- Price config is valid only while `updatedAt + staleAfter` has not expired.

## Worker Behavior Notes

- Config is loaded once at startup. Runtime config changes require a process restart.
- Each chain must set `family: evm` in phase 1. Non-EVM chain families are rejected until the maintained scope documentation is updated.
- Transaction type is derived at send time from the latest block header: headers with `baseFee` use EIP-1559 dynamic-fee transactions, while headers without `baseFee` use legacy gas-price transactions. The tx manager reads current RPC gas/tip suggestions and estimates outer transaction gas before signing each outbox row; configured `max_fee_per_gas_wei` is the send cap for both legacy gas price and dynamic `GasFeeCap`, and `max_priority_fee_per_gas_wei` is required only when the chain header supports dynamic fees. Signer nonce progression is local and durable: the first queued transaction for a `(chain_eid, signer_id)` bootstraps a Postgres cursor from RPC pending nonce once, then all later nonce assignment increments the local cursor only. Failed outbox rows are retried automatically up to five attempts with exponential backoff. Gas-estimate or no-nonce failures requeue in place, sign/broadcast failures replace the same nonce with a fresh fee quote, and failed receipts clone a fresh queued row with a new local nonce.
- Address fields in worker config are parsed as EVM 20-byte hex addresses at load time. `pathways[].source_workers.open_executor`, `pathways[].source_workers.open_dvn`, and `pathways[].destination_workers.open_dvn` are always required so the config fully describes both source assignment and destination verification, even if this process only runs one worker role. `chains[].tx_roles.executor` is required only when `services.executor.enabled` is true; active `chains[].tx_roles.dvn` is required only when `services.dvn.enabled` is true and at least one pathway uses `dvn.mode: active`. `pricing.base_fee_wei` is the worker contract quote base fee, not the EIP-1559 block base fee.
- `services.executor.enabled` and `services.dvn.enabled` default to true when omitted. A process may run both roles, only executor, only DVN, or only pricing/auxiliary loops. `pricing.enabled` keeps its existing independent meaning. `signers` may be empty only when executor, active DVN, and pricing are all disabled for this process.
- The worker always starts metrics. It starts per-chain indexers only for enabled role streams, starts txmgr only when at least one executor/DVN/pricing tx target exists, starts executor committer/deliverer only when executor is enabled, starts the DVN verifier only when DVN is enabled, and starts the price bot only when pricing is enabled. Retryable loop errors are logged, counted in process-local metrics, and restarted with backoff; non-retryable loop errors stop `App.Run`.
- In a pathway's active DVN mode, the DVN flow is `ASSIGNED -> WAITING_CONFIRMATIONS -> QUORUM_CHECKING -> READY_TO_VERIFY -> VERIFY_TX_ENQUEUED -> VERIFIED`. The verifier enqueues `OpenDVN.submitVerification` to `pathways[].destination_workers.open_dvn` with the destination chain's `tx_roles.dvn`; the destination OpenDVN then calls `ReceiveUln302.verify`, so `PayloadVerified.dvn` is the configured OpenDVN. Tx manager is the only component that signs and broadcasts it.
- The executor flow is `ASSIGNED -> WAITING_DVN_VERIFICATION -> VERIFIABLE -> COMMIT_TX_ENQUEUED -> COMMITTED -> EXECUTABLE -> LZ_RECEIVE_TX_ENQUEUED -> DELIVERED`. The worker polls destination readiness before commit and delivery, and tx receipts or destination logs persist the final outcomes.
- Before building an `OpenDVN.submitVerification`, `ReceiveUln302.commitVerification`, or `EndpointV2.lzReceive` outbox transaction, workers reconcile destination-chain state. If the endpoint already has the expected inbound payload hash, the DVN/executor job is marked complete without enqueueing a duplicate verify or commit transaction. If `ReceiveUln302.hashLookup` already has the local OpenDVN submission with enough confirmations, active DVN verification is marked complete without enqueueing. If `lzReceive` already cleared the payload and the packet nonce is covered by `lazyInboundNonce`, delivery is marked complete without enqueueing.
- Failed `lzReceive` receipts or destination `LzReceiveAlert` logs move jobs to `LZ_RECEIVE_FAILED`; when txmgr creates a fresh receipt retry, it restores the affected `lzReceive` workflow to `LZ_RECEIVE_TX_ENQUEUED` so the retry receipt can mark delivery. Failed `dvn_verify` receipts fail the outbox row and do not mark the DVN job verified; txmgr can still fresh-retry the failed outbox row without advancing the DVN job until a successful receipt arrives.
- Per-chain `start_block_number` is optional and defaults to `0`. It only seeds the first indexer backfill when no durable cursor exists; after a cursor is written, the database cursor is authoritative. Per-chain `indexer_query_block_range` defaults to `500` and caps each indexer `FilterLogs` request. Indexers poll confirmed block windows and persist progress through durable cursors split by role: `executor_source`, `executor_destination`, `dvn_source`, and `dvn_destination`.
- Source indexers support transactions with multiple EndpointV2 sends by correlating worker assignment, fee, and `PacketSent` logs with transaction hash, log index, and send-library address.
- Indexer polling errors are logged, exposed through process-local metrics, and retried without advancing the failed stream cursor. Other retryable durable loop errors are logged and supervised with restart backoff instead of canceling the worker process.
- Worker startup and one-shot price updates fail before database sync when on-chain config does not match the loaded YAML.
