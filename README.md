# oh-my-lazier

Self-hosted LayerZero V2 Executor and DVN worker stack for the initial Ethereum Sepolia <-> Base Sepolia EVM pathway.

The first implementation pass includes:

- Hardhat V3 Solidity workspace using Solidity `^0.8.35`.
- Fixed LayerZero and OpenZeppelin package versions in `package-lock.json`.
- `TestOFT` with burn/mint OFT behavior, per-pathway send/receive pause, and outbound token-bucket rate limits.
- `OpenExecutor` and `OpenDVN` contracts with allowed SendLib checks, pathway gating, stale price rejection, message-size limits, option validation, and assignment events.
- Go worker packages with config loading, chain registry, Postgres connection, metrics health endpoint, tx manager loop, indexer loop, executor committer/deliverer loop, DVN mode wiring, signer interfaces, and state enums.
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
make security-check # security doc/log guard, npm audit disposition, and Go vulnerability check
make runbook-check  # runbook coverage guard
make migration-evidence-check # validates the example migration evidence record
make lint-go        # golangci-lint
make fmt-go         # gofmt
make fmt-sol        # format solidity files
```

`npm run check` remains available for compile plus tests without the Go linter.
CI mirrors the local gates with contract/script/runbook/migration-evidence
checks, Go tests and lint, Docker image build validation, and a dedicated
`make security-check` job.

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
chain IDs, LayerZero endpoint IDs, deployed contract code, OApp peers, active
send/receive libraries, ULN required DVNs, and OpenExecutor/OpenDVN pathway
settings. Operators can run the same gate directly:

```bash
go run ./go/cmd/configcheck -config config/example.yaml
```

## Current Scope

The initial target is Ethereum Sepolia <-> Base Sepolia with:

- `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`
- `confirmations = 12`
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
- The worker starts metrics, per-chain indexers, tx manager, executor committer/deliverer, DVN verifier, and price bot loops under one cancellation context.
- In active DVN mode, the DVN flow is `ASSIGNED -> WAITING_CONFIRMATIONS -> QUORUM_CHECKING -> READY_TO_VERIFY -> VERIFY_TX_ENQUEUED -> VERIFIED`. The verifier enqueues `ReceiveUln302.verify`; tx manager is the only component that signs and broadcasts it.
- The executor flow is `ASSIGNED -> WAITING_DVN_VERIFICATION -> VERIFIABLE -> COMMIT_TX_ENQUEUED -> COMMITTED -> EXECUTABLE -> LZ_RECEIVE_TX_ENQUEUED -> DELIVERED`. The worker polls destination readiness before commit and delivery, and tx receipts or destination logs persist the final outcomes.
- Failed `lzReceive` receipts or destination `LzReceiveAlert` logs move jobs to `LZ_RECEIVE_FAILED`, where they remain retryable. Failed `dvn_verify` receipts fail the outbox row and do not mark the DVN job verified.
- Per-chain `start_block_number` is optional and defaults to `0`. It only seeds the first indexer backfill when no durable cursor exists; after a cursor is written, the database cursor is authoritative. Per-chain `indexer_query_block_range` defaults to `500` and caps each indexer `FilterLogs` request. Indexers poll confirmed block windows and persist progress through durable cursors.
- Indexer polling errors are logged, exposed through process-local metrics, and retried without advancing the failed stream cursor. Non-indexer durable loop errors still cancel the worker process so packet state does not advance with missing components.
- Worker startup and one-shot price updates fail before database sync when on-chain config does not match the loaded YAML.
