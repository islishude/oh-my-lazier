# AGENTS.md

## Project Conventions

- Treat [docs/PLANS.md](/Users/sudoless/codespace/coding/oh-my-lazier/docs/PLANS.md) as the top-level implementation source of truth.
- Keep LayerZero interfaces imported from pinned packages; do not copy interface definitions into this repository.
- Use Solidity `^0.8.35` and Hardhat V3 for contract work.
- Keep the first phase EVM-only and scoped to Ethereum Sepolia <-> Base Sepolia.
- Do not add support for `composeMsg`, `lzCompose`, native drop, ordered execution, or non-EVM chains unless the plan is explicitly updated.
- When adding or changing behavior, add the necessary comments and documentation in the same change. Go exported functions and methods must have Go doc comments. Solidity contracts, libraries, public/external functions, events, and public state that form the contract interface must have NatSpec. Comments should explain non-obvious invariants, version constraints, security boundaries, and operational assumptions, not restate the code.

## Checks

Run these before handing off changes:

```bash
make check
```

`make check` runs contract compile, Solidity tests, Go tests, and `golangci-lint`. `npm run check` remains available when only compile/tests are needed.

## Contract Notes

- `OpenExecutor` must match the pinned `ILayerZeroExecutor` interface. The current package version exposes nonpayable `assignJob`, so fee collection cannot be implemented there without breaking interface compatibility.
- `OpenDVN` should reject non-empty DVN options in the first phase.
- Executor options should accept exactly one `lzReceiveOption` with zero value and reject duplicate, compose, native drop, ordered execution, and unknown options.
- Price config must be considered invalid when stale.

## Go Worker Notes

- Keep config load-on-start; do not add hot reload in the first phase.
- Keep signer implementations behind `internal/signer.Signer`.
- Never log private key material, decrypted keystore content, KMS signatures, or raw secrets.
- Use Postgres-backed state machines for durable packet, DVN, and tx manager state as implementation deepens.
