# AGENTS.md

## Source of Truth

- Use [README.md](README.md), runbooks under [docs/runbooks](docs/runbooks), deployment policy under [docs/deployments](docs/deployments), and release-readiness records under [docs/security](docs/security) as the maintained project documentation.
- Keep runbook, deployment, security, and migration evidence documents aligned with behavior changes.
- Do not mark operational or release-readiness work complete unless repository evidence and checks prove it.

## Development Stage

- This repository is still in active code development. Do not preserve backward compatibility for retired repo-local config, schema, state, APIs, generated artifacts, fixtures, or test expectations.
- When behavior changes, update callers, docs, tests, and examples in lockstep; delete obsolete fallback paths, dual-decode paths, migration shims, and legacy compatibility tests unless explicitly requested.
- Do not add data migrations for repo-local schema changes unless explicitly requested; update the single maintained initial schema migration file instead.

## Phase-1 Scope

- The first phase is EVM-only and scoped to Ethereum Sepolia <-> Base Sepolia.
- Required DVNs are `OpenDVN` and an independent LayerZero Labs DVN.
- Confirmations are fixed at `12` unless the maintained scope documentation is explicitly updated.
- Do not add `composeMsg`, `lzCompose`, native drop, ordered execution, or non-EVM support without an explicit scope documentation update.
- Do not migrate to self-only DVN.

## Contracts

- Use Solidity `^0.8.35` and Hardhat V3.
- Keep LayerZero interfaces imported from pinned packages; do not copy interface definitions into this repository.
- `OpenExecutor` must stay compatible with the pinned `ILayerZeroExecutor` interface. The current interface exposes nonpayable `assignJob`, so fee collection cannot be added there without breaking compatibility.
- `OpenDVN` rejects non-empty DVN options in phase 1.
- Executor options must accept exactly one zero-value `lzReceiveOption` and reject duplicate, compose, native drop, ordered execution, unknown options, and unsupported worker IDs.
- Price config is invalid when stale.
- Public Solidity interfaces, public state, external/public functions, events, and libraries need NatSpec when added or changed.

## Go Worker

- Keep config load-on-start; do not add hot reload in phase 1.
- Keep signer implementations behind `internal/signer.Signer`.
- Never log private key material, decrypted keystore content, KMS signatures, API keys, or raw secrets.
- Store durable packet, DVN, executor, and tx manager state in Postgres-backed state machines.
- Keep price sources configurable. Binance remains the default primary source; CoinMarketCap and CoinGecko are supported as primary or sanity sources, and Uniswap V3 remains the on-chain sanity route.
- CoinMarketCap API keys must be referenced through environment variable names, not stored in YAML.

## Documentation

- Add or update docs in the same change as behavior changes.
- Keep Markdown free of local machine paths, user names, tool-cache paths, and host-specific execution details.
- Use relative repository paths in links.
- Keep `docs/security` as release-readiness documentation, not personal or machine-specific scan notes.

## Tests

- Prefer adding coverage to existing table-driven tests, or converting adjacent similar cases into table-driven coverage when practical.
- Avoid adding near-duplicate one-off tests for each bug fix or feature when the same behavior can be covered by an existing table or a shared table-driven test.

## Checks

Run before handoff:

```bash
make check
```

`make check` runs:

- Hardhat compile
- TypeScript typecheck
- Solidity tests
- TypeScript script tests
- Go tests
- runbook coverage check
- migration evidence example check
- `golangci-lint`
- Go formatting check
- Solidity formatting check

Additional targeted gates:

```bash
make test-integration
make security-check
npm run check:runbooks
MIGRATION_EVIDENCE=docs/deployments/testnet-migration-evidence.example.json npm run check:migration-evidence
```

Use `make test-integration` for the dedicated Postgres/Rustack integration stack. It starts the containers with Docker Compose, waits for health checks, runs DB/tx manager and Rustack KMS tests, then tears down containers and temporary database files.

`make security-check` runs the security review document and secret logging guard, npm audit disposition gate, and Go vulnerability check.
