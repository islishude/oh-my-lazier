# AGENTS.md

## Operating Rules

- Treat [README.md](README.md), [docs/runbooks](docs/runbooks), [docs/deployments](docs/deployments), and [docs/security](docs/security) as maintained documentation. Keep them aligned with behavior changes.
- This repo is still in active development. Do not keep compatibility shims, fallback config/schema paths, dual decoders, retired fixtures, or legacy tests unless explicitly requested.
- Phase 1 supports EVM chains only. required DVNs are `OpenDVN` plus an independent 3rd-party DVN.
- Do not add non-EVM support, `composeMsg`, `lzCompose`, native drop, ordered execution, self-only DVN, hot config reload, or live testnet/mainnet execution unless maintained scope docs are updated first.
- For repo-local schema changes, update `go/migrations/001_initial_schema.sql`. Do not add separate data migrations unless explicitly requested.

## Repository Layout

- `contracts/contracts`: Solidity contracts. Shared worker code lives under `common`, OFT code under `oft`, and `OpenExecutor`/`OpenDVN` under `workers`.
- `contracts/test`: Solidity tests. Prefer table coverage in the existing test file over one-off near-duplicates.
- `contracts/scripts`: TypeScript deploy, inspect, check, generation, and runbook/security validator scripts. Script behavior changes need script tests when practical.
- `go/cmd`: CLI entrypoints only. Keep business logic in `go/internal`.
- `go/internal/config`, `configcheck`, `configdiff`, `chain`: config loading, validation, on-chain checks, and static chain metadata.
- `go/internal/db`, `packets`, `dvn`, `executor`, `indexer`, `txmgr`, `readiness`: durable worker state machines and runtime flows. Store packet, DVN, executor, indexer, and tx manager state in Postgres.
- `go/internal/lzabi/abis`, `go/internal/pricing/abis`, `go/internal/configcheck/abis`: committed embedded ABI inputs. Regenerate with the Makefile targets; do not hand-edit generated JSON.
- `docs/runbooks`, `docs/deployments`, `docs/security`: operational docs, deployment policy/evidence, and release-readiness records.

## Contracts

- Use Solidity `^0.8.35` and Hardhat V3.
- Import LayerZero interfaces from pinned packages; do not copy interface definitions into this repo.
- Keep `OpenExecutor` compatible with the pinned nonpayable `ILayerZeroExecutor.assignJob` interface; do not add fee collection there.
- `OpenDVN` rejects non-empty DVN options in phase 1.
- Executor options must accept exactly one zero-value `lzReceiveOption` and reject duplicates, compose, native drop, ordered execution, unknown options, and unsupported worker IDs.
- Price config is invalid when stale.
- Add NatSpec for new or changed public Solidity interfaces, public state, external/public functions, events, and libraries.

## Go Worker

- Load config once at startup. Fail fast on invalid local config or mismatched on-chain config before durable loops start.
- Keep signer implementations behind `internal/signer.Signer`.
- Never log private keys, decrypted keystores, KMS signatures, API keys, raw secrets, or secret-bearing config values.
- Validate every configured RPC URL against the configured chain ID; one healthy endpoint is not enough.
- Keep price sources configurable. Binance is the default primary source; CoinMarketCap and CoinGecko may be primary or sanity sources; Uniswap V3 remains the on-chain sanity route.
- CoinMarketCap API keys must be referenced by environment variable name, not stored in YAML.
- Go exported functions, methods, types, and packages need doc comments when they are part of a maintained package surface.

## Documentation

- Update docs, examples, validator anchors, and migration evidence examples in the same change as behavior changes.
- Keep Markdown free of local machine paths, user names, tool-cache paths, and host-specific execution details.
- Use relative repository links.
- Keep `docs/security` for release-readiness records, not personal scan notes.
- If generated ABI source dependencies change, update npm audit/security dispositions as needed.

## Tests and Checks

- Prefer extending existing table-driven tests, or converting adjacent cases to tables, before adding one-off tests.
- Run focused tests first for the code touched, then run the repo gate before handoff:

```bash
make check
```

`make check` currently runs compile, TypeScript typecheck, LayerZero ABI drift check, pricing ABI drift check, Solidity tests, TypeScript script tests, Go tests, runbook check, migration evidence check, `golangci-lint`, Go format check, and Solidity format check.

Use targeted gates when the change touches those areas:

```bash
make test-integration
make security-check
make docker-smoke
npm run check:runbooks
MIGRATION_EVIDENCE=docs/deployments/testnet-migration-evidence.example.json npm run check:migration-evidence
```

- `make test-integration` is the Docker Compose Postgres plus Rustack KMS stack.
- `make security-check` runs security review validation, npm audit disposition validation, and Go vulnerability checking.
- Run `make generate-lzabi` or `make generate-pricing-abi` when their source artifacts or pinned packages change, then keep the corresponding `make check-*` target green.
