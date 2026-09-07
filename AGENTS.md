# AGENTS.md

## Required reading

Before editing, read the applicable sections of [development rules](docs/development.md)
and [Phase 1 scope](docs/scope.md); they are mandatory repository instructions.
For runtime changes, also read [runtime behavior](docs/runtime.md) and the relevant
[runbook or deployment policy](docs/README.md).

## Core rules

- Keep behavior, maintained docs, examples, validator anchors, and migration evidence aligned in the same change. Use relative Markdown links; omit local paths, user names, and host-specific details.
- Do not retain compatibility shims, fallback config/schema paths, dual decoders, retired fixtures, or legacy tests unless explicitly requested.
- Phase 1 is EVM-only and requires `OpenDVN` plus an independently operated DVN. Update maintained scope docs before adding excluded features or live testnet/mainnet execution.
- Committed `go/migrations` files are immutable. Add the next numbered incremental migration; preserve initialized-database upgrades. Data backfills require an explicit request.
- Never log secrets or put keys, RPC credentials, or passwords in script JSON. Follow the development guide's signer, secret-reference, and connection rules.
- Chain-writing scripts require explicit `apply`; non-TTY `apply: true` also requires `confirmation: "approved"`.
- Prefer extending existing table-driven tests. Run focused checks first, then `make check` before handoff; run applicable integration, security, and smoke gates from the development guide.
