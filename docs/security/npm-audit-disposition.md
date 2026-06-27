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
make security-check
```

`npm run check:npm-audit-disposition` runs `npm audit --audit-level=moderate
--json` and requires:

- zero critical findings
- every high or moderate finding to be present in the recorded disposition set

`make security-check` runs the npm audit disposition gate and `govulncheck`.

Current npm audit metadata:

```text
critical: 0
high: 6
moderate: 4
low: 21
total: 31
```

## Remediation Applied

- `@nomicfoundation/hardhat-toolbox-viem` remains a direct dependency.
- `viem` remains a direct pinned dependency for scripts.
- Independent vulnerable transitive packages are pinned through npm overrides:
  - `axios = 1.18.1`
  - `elliptic = 6.6.1`
  - `undici = 6.27.0`
  - `ws = 8.21.0`

These changes remove all critical npm audit findings without changing pinned
LayerZero package versions or removing Hardhat toolbox support.

## Remaining High Findings

| Package                               | Direct | Source                                              | Disposition                                                               |
| ------------------------------------- | ------ | --------------------------------------------------- | ------------------------------------------------------------------------- |
| `@chainlink/contracts-ccip`           | no     | LayerZero messagelib transitive dependency          | Open. Do not auto-downgrade LayerZero packages.                           |
| `@openzeppelin/contracts`             | no     | LayerZero and Chainlink transitive dependency graph | Open. Project contracts directly use pinned OpenZeppelin v5.              |
| `@openzeppelin/contracts-upgradeable` | no     | LayerZero and Chainlink transitive dependency graph | Open. Project contracts do not import upgradeable OpenZeppelin contracts. |
| `@layerzerolabs/lz-evm-messagelib-v2` | yes    | pinned LayerZero package                            | Open. Required for current LayerZero interface compatibility.             |
| `@layerzerolabs/lz-evm-oapp-v2`       | yes    | pinned LayerZero package                            | Open. Required for current OFT base contracts.                            |
| `lodash-es`                           | no     | Hardhat toolbox transitive dependency               | Open. Keep Hardhat V3 toolbox support.                                    |

## Remaining Moderate Findings

The remaining moderate findings are attached to retained Hardhat toolbox
dependencies:

- `@nomicfoundation/hardhat-toolbox-viem`
- `@nomicfoundation/hardhat-ignition`
- `@nomicfoundation/hardhat-ignition-viem`
- `@nomicfoundation/ignition-core`

These are not accepted for mainnet by this document. They require either a
compatible upstream fix or explicit approval before mainnet readiness.

## Release Decision

- Critical npm findings are closed for the current dependency graph.
- High and moderate findings remain open release-readiness items.
- Do not apply npm's suggested LayerZero downgrade automatically; the project
  relies on the currently pinned package interfaces.
- Mainnet readiness requires one of:
  - a compatible LayerZero/Hardhat package update that clears these advisories
  - an explicit security approval accepting the remaining transitive toolchain
    exposure for the planned release

## Verification

```bash
npm run check:npm-audit-disposition
make security-check
make check
git diff --check
```
