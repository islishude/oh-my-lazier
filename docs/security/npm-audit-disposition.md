# npm Audit Disposition

Date: 2026-06-27

Scope:

- `package.json`
- `package-lock.json`
- Hardhat contract compile and Solidity tests
- TypeScript deployment and LayerZero configuration scripts

This document records the current disposition for npm audit findings that affect the contract build, test, and deployment toolchain. It is not final mainnet approval.

## Current Result

Command:

```bash
npm audit --audit-level=moderate --json
```

Current metadata:

```text
critical: 0
high: 6
moderate: 4
low: 21
total: 31
```

`npm audit --audit-level=critical --json` exits successfully with 0 critical vulnerabilities.

`make security-check` runs the critical npm audit gate and Go called-vulnerability gate together.

## Remediation Applied

- Kept `@nomicfoundation/hardhat-toolbox-viem` as a direct dependency and Hardhat V3 plugin.
- Kept `viem` as a direct pinned dependency for deployment and LayerZero configuration scripts.
- Added npm `overrides` for independent vulnerable transitive packages:
  - `axios = 1.18.1`
  - `elliptic = 6.6.1`
  - `undici = 6.27.0`
  - `ws = 8.21.0`

These changes reduced the audit result from 3 critical / 19 high to 0 critical / 6 high without changing pinned LayerZero package versions or removing Hardhat toolbox support.

## Remaining High Findings

The remaining high findings are:

| Package | Direct | Path | npm suggested fix | Disposition |
| --- | --- | --- | --- | --- |
| `@chainlink/contracts-ccip` | no | `@layerzerolabs/lz-evm-messagelib-v2 -> @chainlink/contracts-ccip` | downgrade `@layerzerolabs/lz-evm-messagelib-v2` to `2.0.6` | Open. Do not auto-downgrade LayerZero from pinned `3.0.168`. |
| `@openzeppelin/contracts` | no | old contract packages under `@chainlink/contracts-ccip` and LayerZero v1 compatibility packages | downgrade `@layerzerolabs/lz-evm-messagelib-v2` to `2.0.6` | Open. Project contracts directly use pinned OpenZeppelin `5.6.1`; remaining finding is transitive. |
| `@openzeppelin/contracts-upgradeable` | no | transitive LayerZero/Chainlink contract package chain | downgrade `@layerzerolabs/lz-evm-messagelib-v2` to `2.0.6` | Open. No project contract imports upgradeable OpenZeppelin contracts. |
| `@layerzerolabs/lz-evm-messagelib-v2` | yes | pinned direct dependency `3.0.168` | downgrade to `2.0.6` | Open. Pinned package provides current LayerZero interfaces used by contracts. |
| `@layerzerolabs/lz-evm-oapp-v2` | yes | pinned direct dependency `3.0.168` via messagelib/protocol/v1 compatibility packages | downgrade to `2.0.6` | Open. Pinned package provides current OFT base contracts used by `TestOFT`. |
| `lodash-es` | no | `@nomicfoundation/hardhat-toolbox-viem -> @nomicfoundation/ignition-core -> lodash-es` | downgrade `@nomicfoundation/hardhat-toolbox-viem` to `4.1.2` | Open. Keep toolbox per project requirement; do not downgrade Hardhat V3 toolbox automatically. |

## Remaining Moderate Findings

The remaining moderate findings are attached to the retained Hardhat toolbox chain:

- `@nomicfoundation/hardhat-toolbox-viem`
- `@nomicfoundation/hardhat-ignition`
- `@nomicfoundation/hardhat-ignition-viem`
- `@nomicfoundation/ignition-core`

These are not accepted for mainnet. They require either a compatible upstream fix or explicit operational disposition before mainnet approval.

## Decision

- Critical npm findings are closed for the current dependency graph.
- Remaining high and moderate findings are not accepted for mainnet.
- Do not apply npm's suggested LayerZero downgrade automatically. The project plan requires pinned LayerZero package compatibility, and `OpenExecutor` currently depends on the interface shape exposed by the pinned package.
- Before mainnet readiness approval, either:
  - update to a newer LayerZero package set that clears these advisories while preserving contract/interface compatibility, or
  - update to a compatible Hardhat toolbox release that clears the retained toolbox advisories, or
  - obtain an explicit security approval documenting why the remaining transitive package advisories do not affect compiled/deployed bytecode or deployment operations.

## Verification

Commands run after remediation:

```bash
npm run check
npm run typecheck
npm audit --audit-level=critical --json
make security-check
make check
git diff --check
```

All listed checks passed.
