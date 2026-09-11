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

Current npm audit metadata (2026-09-11, after ABI dependency cleanup and the OpenZeppelin v4 override):

```text
critical: 0
high: 1
moderate: 3
low: 24
total: 28
```

## Remediation Applied

- Removed direct `@chainlink/contracts = 1.5.0` and `@uniswap/v3-core = 1.0.1` dependencies. Complete interface ABI arrays and their licenses are retained as [vendored inputs](../../contracts/vendor/abis/README.md), with source provenance and SHA-256 verification in both generation and check modes.
- Generated Go pricing ABI bytes remain unchanged. The Go worker embeds these files and has no Node.js runtime dependency.
- The ABI cleanup removed 176 package entries, added none, and changed no retained package versions. LayerZero dependencies and the Hardhat/Viem/Ignition toolchain remain pinned.
- Existing npm overrides are retained. A scoped `@chainlink/contracts-ccip@0.7.6` override pins its `@openzeppelin/contracts` dependency to `4.9.6` instead of `4.3.3`. The path is `@layerzerolabs/lz-evm-messagelib-v2 -> @chainlink/contracts-ccip -> @openzeppelin/contracts`. The direct v5 dependency remains `5.6.1`; no other installed package version changes in this override step.
- After this override, audit reports critical=0, high=2, moderate=3, low=24, total=29. The installed `@openzeppelin/contracts@4.9.6` is absent from the current advisory nodes. CCIP and its LayerZero parents now have moderate aggregate findings. At that step, the CCIP aliases `@openzeppelin/contracts-v0.7` (actual version `3.4.2`) and `@openzeppelin/contracts-upgradeable-4.7.3` accounted for the two high findings.
- A fresh audit before cleanup reported critical=0, high=6, moderate=3, low=25, total=34. After cleanup it reports critical=0, high=5, moderate=0, low=24, total=29. The removed high/moderate findings are `tmp`, `@chainlink/contracts`, `@arbitrum/nitro-contracts`, and `@offchainlabs/upgrade-executor`. Previously recorded findings absent from the fresh audit are also removed from the disposition set.

- A subsequent override under the same CCIP scope maps `@openzeppelin/contracts-upgradeable-4.7.3` to `npm:@openzeppelin/contracts-upgradeable@4.9.6`. The alias name is preserved for upstream imports; its installed version is 4.9.6. This step changes only that package version and retains upgradeable v5.6.1. Audit now reports critical=0, high=1, moderate=3, low=24, total=28; the upgradeable finding is removed.

## Remaining High Findings

| Package | Direct | Source | Disposition |
| --- | --- | --- | --- |
| `@openzeppelin/contracts` | no | CCIP alias `@openzeppelin/contracts-v0.7`, actual version `3.4.2` | Open. Outside the v4 override; project contracts directly use v5.6.1. |

## Remaining Moderate Findings

| Package | Direct | Source | Disposition |
| --- | --- | --- | --- |
| `@chainlink/contracts-ccip` | no | LayerZero peer dependency | Open. Remaining aggregate finding from its dependency graph. |
| `@layerzerolabs/lz-evm-messagelib-v2` | yes | pinned LayerZero package | Open. Required for current LayerZero interfaces. |
| `@layerzerolabs/lz-evm-oapp-v2` | yes | pinned LayerZero package | Open. Required for current OFT base contracts. |

New high or moderate findings and severity changes still fail the disposition gate; these results are not mainnet approval.

## Release Decision

- Critical npm findings are closed for the current dependency graph.
- High and moderate findings remain open release-readiness items.
- Do not apply npm's suggested LayerZero downgrade automatically; the project
  relies on the currently pinned package interfaces.
- Mainnet readiness requires one of:
  - compatible dependency updates that clear these advisories
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
