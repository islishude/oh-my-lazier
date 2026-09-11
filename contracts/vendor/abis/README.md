# Vendored pricing ABIs

These complete interface ABI arrays are build inputs for the [pricing ABI generator](../../scripts/generate-pricing-abi.ts). Only the selected methods are emitted into the Go embedded ABIs. No bytecode is included.

[sources.json](sources.json) records pinned npm package versions, upstream repositories, original package paths, extraction rules, original file SHA-256 hashes, and saved ABI SHA-256 hashes. The generator verifies saved bytes before generation and drift checks; normal builds do not download these sources.

## Attribution and licenses

- Chainlink AggregatorV3Interface: SmartContract Kit; the source interface in `src/v0.8/shared/interfaces/AggregatorV3Interface.sol` declares `SPDX-License-Identifier: MIT`. See [MIT license](LICENSE-Chainlink). The package-level BUSL license is not substituted for the interface license.
- Uniswap IUniswapV3Pool: Uniswap; the source interface in `contracts/interfaces/IUniswapV3Pool.sol` declares `SPDX-License-Identifier: GPL-2.0-or-later`. See the original [interface license](LICENSE-Uniswap).

## Updating

Obtain the explicitly selected npm release archive and verify its registry integrity. Extract the source path recorded in the manifest, apply the recorded extraction rule, and update both hashes and the version together. Review the applicable interface license and retain its notices. Run `make generate-pricing-abi`, review the generated diff, then run `make check` and `make security-check`. Do not edit the generated Go JSON by hand.
