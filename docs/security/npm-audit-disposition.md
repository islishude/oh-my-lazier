# npm Audit Disposition

This document records the current npm audit disposition for the contract build,
test, and deployment toolchain. It is not final mainnet approval.

## Scope

- `package.json`
- `package-lock.json`
- Hardhat compile and Solidity tests
- TypeScript deployment and LayerZero configuration scripts

## Current Gate

```bash
npm run check:npm-audit-disposition
npm run check:security-review
make security-check
```

`npm run check:npm-audit-disposition` runs `npm audit --audit-level=moderate
--json` and requires:

- zero critical findings
- every high or moderate finding to be present in the recorded disposition set

`make security-check` runs the security review document gate, npm audit
disposition gate, and `govulncheck`.

## ABI source migration audit evidence

The 2026-09-21 `npm audit --json` snapshots before and after removal used the
same registry and unchanged remaining direct dependency versions:

| Severity | Before | After |
| --- | ---: | ---: |
| Critical | 0 | 0 |
| High | 6 | 5 |
| Moderate | 3 | 0 |
| Low | 25 | 24 |
| Total | 34 | 29 |

Audit dependency totals fell from 497 to 321. Counts describe npm package-level
findings, including inherited findings, not unique advisories. The older 43-item
snapshot is superseded; it is not the migration baseline.

## Remediation Applied

- Removed direct `@chainlink/contracts = 1.5.0` and `@uniswap/v3-core = 1.0.1`
  source dependencies and their unreferenced transitive dependencies.
- The complete ABI arrays, source hashes, extraction details, and interface-specific
  MIT / GPL-2.0-or-later licenses are retained in
  [the local ABI source directory](../../contracts/vendor/abis/README.md).
- Generation and checks verify saved-file SHA-256 locally; they do not download
  ABI sources. All four generated Go pricing ABI files remain byte-identical.
- `npm ci` verifies the updated lockfile. Both removed packages are absent from
  the dependency tree; LayerZero's `@chainlink/contracts-ccip` remains installed.
- All other direct dependency versions and overrides remain unchanged, including
  Hardhat 3.16.0, toolbox 5.0.7, Ignition core 3.1.9, OpenZeppelin 5.6.1,
  and the pinned LayerZero versions. Existing overrides remain axios 1.20.0,
  elliptic 6.6.1, undici 6.28.1, ws 8.21.0, js-yaml 4.3.2, adm-zip 0.6.1.

The findings for `@chainlink/contracts`, `@arbitrum/nitro-contracts`,
`@offchainlabs/upgrade-executor`, `tmp`, and `patch-package` disappeared with
this removal. The high/moderate disposition map now contains only the five
remaining high findings below. Obsolete high/moderate exceptions for retained
Hardhat/Ignition tooling and other packages were also removed based on the
current audit; no exemption was added or broadened. Hardhat/Ignition packages
with remaining low findings are still recorded below.

## Remaining High Findings

| Package | Affected source | Disposition |
| --- | --- | --- |
| `@chainlink/contracts-ccip` | LayerZero transitive dependency | Open. Retain required CCIP interfaces; do not auto-downgrade LayerZero. |
| `@layerzerolabs/lz-evm-messagelib-v2` | Direct pinned LayerZero package | Open. Required messaging interfaces; inherits CCIP findings. |
| `@layerzerolabs/lz-evm-oapp-v2` | Direct pinned LayerZero package | Open. Required OFT base contracts; inherits messaging findings. |
| `@openzeppelin/contracts` | CCIP nested dependency and `@openzeppelin/contracts-v0.7` alias | Open. Legacy copies remain; the project's direct 5.6.1 copy is not flagged. |
| `@openzeppelin/contracts-upgradeable` | CCIP's `@openzeppelin/contracts-upgradeable-4.7.3` alias | Open. Project contracts do not import upgradeable contracts. |

## Remaining Moderate and Low Findings

There are no moderate package-level findings in this snapshot. The 24 low
findings remain in the Ethers v5 / elliptic dependency graph, Optimism contracts
and core-utils, LayerZero protocol/v1 packages, hardhat-deploy/zksync-ethers,
and retained Hardhat Ignition, Ignition-Viem, toolbox, and verify packages.
They are not represented as high/moderate exceptions. This document grants no
mainnet approval for them. The Go worker embeds the generated ABI and has no
Node.js runtime dependency.

## Release Decision

- Critical npm findings are closed for the current dependency graph.
- High and moderate findings remain open release-readiness items.
- Do not apply npm's suggested LayerZero downgrade automatically; the project
  relies on the currently pinned package interfaces.
- Mainnet readiness requires one of:
  - compatible updates to the affected dependency graph that clear these advisories
  - an explicit security approval accepting the remaining transitive toolchain
    exposure for the planned release

## Verification

```bash
npm run check:npm-audit-disposition
npm run check:security-review
make security-check
make check
git diff --check
```

The migration passed a clean `npm ci`, the five table-driven local-source hash
cases (valid, tampered, missing file, missing hash, invalid hash), pricing ABI
generation/check, Go pricing tests, `make check`, `make security-check`, and
`git diff --check`. The full gate included 61 Solidity tests and 245 script tests.
No database/KMS integration, container smoke, deployment, commit, or push was
part of this ABI-only change.

All pricing output files were compared byte for byte against a copy taken
before migration. The following SHA-256 values are identical before and after:

| File under `go/internal/pricing/abis` | SHA-256 (before = after) |
| --- | --- |
| `chainlink_aggregator_v3.json` | `72b79d4d834322a799fbf399f52052edd4870f883448a728c9833d8382423b74` |
| `erc20_metadata.json` | `532593ecc4a3664731a6ca49ce18982e48ddaa768bdf49896549b5ebc345ab65` |
| `price_snapshot.json` | `942fb043a044f9181da0e9583181c22b3a48bf370a53a6d1e23d373f3ec56b6f` |
| `uniswap_v3_pool.json` | `c133837cdfe1e8e37d4231984cd9e181755bf4efb1f13e57d6e677ff619e74c2` |
