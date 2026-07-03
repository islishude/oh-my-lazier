# oh-my-lazier

Self-hosted LayerZero V2 Executor and DVN worker stack.

The repo contains:

- Solidity contracts for `TestOFT`, `OpenExecutor`, `OpenDVN`, worker options, access control, and price config.
- TypeScript scripts for deployment, LayerZero config, canaries, local E2E, ABI generation, runbook checks, and migration evidence checks.
- Go worker services for config validation, indexing, executor delivery, DVN verification, pricing, tx management, readiness, and metrics.
- Docker Compose setups for local Postgres, integration dependencies, and the dual-Anvil E2E.

## Layout

```text
contracts/contracts/   Solidity contracts
contracts/scripts/     Deployment, inspection, validation, and E2E scripts
contracts/test/        Solidity tests
go/                    Go worker, CLI, DB, config, and runtime packages
config/                Example worker config
docs/                  Runbooks, deployment policy, security records, and monitoring
```

## Requirements

- Node.js 26+
- Go 1.26+
- Docker, for integration/E2E/smoke targets
- Foundry `forge`, for Solidity formatting checks
- `golangci-lint`, for `make check`

Install dependencies:

```bash
npm install
```

## Main Commands

```bash
make check            # compile, typecheck, ABI drift checks, tests, docs checks, lint, format checks
make test-integration # Docker Compose Postgres plus Rustack KMS integration tests
make security-check   # security review check, npm audit disposition, govulncheck
make docker-smoke     # build worker image and verify its entrypoint
make e2e-local        # local Postgres + two Anvil chains + worker canary flow
```

ABI artifacts are committed under `go/internal/lzabi/abis`, `go/internal/configcheck/abis`, and `go/internal/pricing/abis`.

```bash
make generate-lzabi
make check-lzabi
make generate-pricing-abi
make check-pricing-abi
```

## Worker

The example config is [config/example.yaml](config/example.yaml). Start the default local stack with:

```bash
docker compose up
```

Run the same on-chain config gate used at worker startup:

```bash
go run ./go/cmd/configcheck -config config/example.yaml
```

`DATABASE_URL` may override the configured database URL at runtime. Other config is loaded once at startup; runtime config changes require a process restart.

Useful operator commands:

```bash
go run ./go/cmd/worker -config <worker.yaml> -log-level debug
go run ./go/cmd/configdiff -from <approved.yaml> -to <proposed.yaml>
go run ./go/cmd/readinesscheck -config <worker.yaml> -format json
go run ./go/cmd/pricebot-once -config <worker.yaml> -log-level debug
go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst> -format json
go run ./go/cmd/txretry -config <worker.yaml> -action retry-failed|replace -id <tx_outbox_id>
```

Worker binaries default to `-log-level info`. Use `-log-level debug` when investigating normal skip/defer reasons such as indexer caught-up windows, disabled pathways, not-yet-confirmed DVN jobs, or deferred tx manager work.

## Phase 1 Scope

Phase 1 is EVM-only.

- Worker chain configs must declare `family: evm`.
- Required DVNs are `OpenDVN` plus an independent LayerZero Labs DVN.
- Basic OFT send is supported.
- `composeMsg`, `lzCompose`, native drop, ordered execution, self-only DVN, and non-EVM chains are out of scope.
- Executor options must contain exactly one zero-value executor `lzReceive` option.
- `OpenDVN` rejects non-empty DVN options.
- Price config must be fresh.

`OpenExecutor` remains compatible with the pinned nonpayable `ILayerZeroExecutor.assignJob` interface. It quotes and emits assignment price information without collecting native fee there. `OpenDVN` is payable and requires `msg.value >= fee`.

## Runtime Notes

- Startup fails before durable loops if local config is invalid or live chain state does not match the loaded YAML.
- Every configured RPC URL must report the configured EVM chain ID.
- Address fields are parsed as EVM 20-byte hex addresses during config load.
- Worker contract addresses remain required in every pathway config, even when this process runs only one role.
- `services.executor.enabled` and `services.dvn.enabled` default to true when omitted; pricing remains independently controlled.
- Tx fees are selected at send time by `txmgr`, which estimates gas, reads current RPC fee suggestions, applies configured caps, signs, broadcasts, and persists the signed fee state.
- Indexers poll confirmed block windows and persist role-specific cursors in Postgres.
- Retryable loop errors are logged and supervised with backoff; non-retryable loop errors stop `App.Run`.

## Maintained Docs

- [contracts/scripts/README.md](contracts/scripts/README.md): deployment, LayerZero config, local E2E, canary, and rollback script usage.
- [docs/runbooks/mainnet-readiness.md](docs/runbooks/mainnet-readiness.md): required review sequence before mainnet.
- [docs/runbooks/config-diff.md](docs/runbooks/config-diff.md): config review and on-chain config check workflow.
- [docs/runbooks/key-management.md](docs/runbooks/key-management.md), [docs/runbooks/price-bot.md](docs/runbooks/price-bot.md), [docs/runbooks/rate-limit.md](docs/runbooks/rate-limit.md), [docs/runbooks/monitoring.md](docs/runbooks/monitoring.md): operator checklists.
- [docs/security/security-review.md](docs/security/security-review.md) and [docs/security/npm-audit-disposition.md](docs/security/npm-audit-disposition.md): release-readiness security records.
