# AGENTS.md

## Source of Truth

- Use [docs/PLANS.md](docs/PLANS.md) as the top-level implementation plan.
- Keep plan, runbook, and milestone documents aligned with behavior changes.
- Do not mark plan items complete unless repository evidence and checks prove them.

## Phase-1 Scope

- The first phase is EVM-only and scoped to Ethereum Sepolia <-> Base Sepolia.
- Required DVNs are `OpenDVN` and an independent LayerZero Labs DVN.
- Confirmations are fixed at `12` unless the plan is explicitly updated.
- Do not add `composeMsg`, `lzCompose`, native drop, ordered execution, or non-EVM support without an explicit plan update.
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

`make security-check` runs the npm audit disposition gate and Go vulnerability check.
