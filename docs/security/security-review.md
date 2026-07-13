# Security Review

This document records the repository-level security review status for the
first-phase LayerZero worker stack. It is a release-readiness artifact, not a
mainnet approval.

## Scope

- Solidity contracts under `contracts/contracts`
- Go worker code under `go`
- TypeScript deployment and operations scripts under `contracts/scripts`
- dependency metadata in `package.json`, `package-lock.json`, `go.mod`, and `go.sum`
- runbooks and config examples that affect operational security

## Repeatable Checks

Run these local checks before attaching this review to a migration or release
ticket:

```bash
make check
make security-check
npm run check:security-review
npm run check:npm-audit-disposition
```

Useful manual review searches:

```bash
rg "(?i)(private[_-]?key|secret|password|signature|keystore|kms|api[_-]?key)" -n go contracts/scripts config docs --glob '!contracts/artifacts/**' --glob '!node_modules/**'
rg "(?i)(logger\\.|slog\\.|fmt\\.Print|log\\.|console\\.log)" -n go contracts/scripts --glob '!contracts/artifacts/**' --glob '!node_modules/**'
rg "(?i)(delegatecall|selfdestruct|tx\\.origin|assembly|unchecked|\\.call\\{|withdraw|onlyOwner|setAllowedSendLib|setPaused)" contracts/contracts contracts/test -n --glob '!contracts/artifacts/**'
```

## Current Status

- `make security-check` requires `npm run check:npm-audit-disposition` to pass and
  runs `npm run check:security-review`, `npm run check:npm-audit-disposition`,
  and `govulncheck` against the Go code.
- `npm run check:security-review` verifies that the release-readiness security
  documents retain the required phase-1 boundaries, open release blockers, and
  no host-specific scan notes. It also includes a secret logging guard that
  fails when Go or TypeScript log calls mention private keys, secrets,
  passwords, signatures, API keys, access keys, session tokens, keystores, or
  credentials.
- `npm run check:npm-audit-disposition` requires zero critical npm findings and
  fails if any high or moderate finding appears outside the recorded
  disposition set.
- `govulncheck` currently reports no vulnerabilities in called Go code.
- The remaining npm audit findings are toolchain and artifact-source findings
  described in `docs/security/npm-audit-disposition.md`; they block final
  mainnet approval until upgraded or formally accepted.

## Reviewed Controls

Signer and key boundary:

- KMS signing requires `ECC_SECG_P256K1`.
- KMS signatures are parsed from DER, normalized to low-S, and recovered against
  the configured Ethereum address.
- Keystore passwords can come from environment variables or password files.
- Missing or empty keystore password sources are rejected.
- Private key material, decrypted keystore JSON, KMS signatures, and raw secrets
  must never be logged.
- The `npm run check:security-review` secret logging guard blocks newly added
  log calls that mention secret-bearing material.

Contract authority:

- `OpenExecutor`, `OpenDVN`, `WorkerAccess`, and OFT controls use owner-gated
  configuration.
- SendLib allowlists are explicit.
- OpenDVN verifier allowlists are explicit; active worker verification submits
  to destination `OpenDVN.submitVerification`, and OpenDVN calls
  `ReceiveUln302.verify` so the recorded `PayloadVerified.dvn` is OpenDVN.
- Worker assignment can be paused.
- OFT sends and receives can be paused per endpoint direction.

LayerZero phase-1 boundary:

- Phase 1 remains EVM-only and scoped to Ethereum Sepolia <-> Hoodi.
- Worker chain configs must declare `family: evm`; other chain families are rejected in phase 1.
- `composeMsg`, `lzCompose`, native drop, ordered execution, and non-EVM chains
  remain out of scope.
- Self-only DVN is rejected; required DVNs must include OpenDVN and at least
  one independent external DVN. LayerZero Labs DVN is an optional external DVN
  choice, not a required provider; deployment profiles can opt into the
  repo-known Sepolia/Hoodi address with `chains[].includeLayerZeroLabsDVN`.
- Confirmations are fixed at 12 unless the maintained scope documentation is updated.

RPC quorum and safety:

- Source head conflicts pause chains.
- Receipt and log conflicts pause pathways.
- Pause state is exposed through `/metrics`.
- Readiness checks reject paused chains/pathways, failed active outbox rows, and
  missing or unstarted indexer cursors required by the process's enabled
  executor/DVN services.

Operational readiness:

- Monitoring, config diff, key management, price bot, rate-limit, and mainnet
  readiness runbooks exist.
- `npm run check:runbooks` verifies the required runbook coverage anchors.
- Migration evidence must pass `npm run check:migration-evidence`.
- Sepolia/Hoodi deployment evidence is attached under
  `docs/deployments/sepolia-hoodi/` and passes
  `npm run check:migration-evidence`; this is deployment evidence, not final
  mainnet approval.

## Open Release-Readiness Item

### S-001: Open npm audit high and moderate toolchain advisories

Severity: release blocker for mainnet approval.

Evidence:

- `npm audit --audit-level=moderate --json` reports zero critical findings.
- The remaining high and moderate findings are limited to pinned LayerZero
  transitive dependencies, retained Hardhat toolbox transitive dependencies,
  and transitive tooling bundled by the pinned Chainlink AggregatorV3 ABI source.
- `npm run check:npm-audit-disposition` tracks the current accepted disposition
  set and fails on new high or moderate findings.

Assessment:

- The findings are in the build, test, deployment, and ABI-generation toolchain
  rather than the Go worker runtime.
- They still matter for live deployment because deployment scripts run with
  network access and signer material.
- Automatic npm audit fixes currently suggest incompatible or unsafe package
  downgrades for this project, including LayerZero package downgrades that would
  break the pinned interface assumptions.

Required closure before M9 completion:

- Upgrade the affected packages while preserving contract/interface
  compatibility, or
- formally accept the remaining transitive advisories for the planned release
  with an explicit operational rationale.

## Remaining Approval Gaps

- No final exhaustive security review or equivalent human approval has been
  recorded.
- No final migration ticket, rollback evidence, or mainnet approval record has
  been attached.
- M9 must remain in progress until S-001, migration-specific rollback evidence,
  and final human approval are closed.
