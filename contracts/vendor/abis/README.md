# Third-party pricing ABI sources

[sources.json](sources.json) records the pinned npm package version, upstream
repository and tarball, original ABI/artifact and interface paths, extraction
method, original source SHA-256, and saved ABI SHA-256 for each file.
Paths in that record are relative to the corresponding npm package; saved file
names are relative to this directory. SHA-256 values cover raw file bytes.

- [AggregatorV3Interface.json](AggregatorV3Interface.json): the complete Chainlink
  contracts 1.5.0 ABI, copied without modification (5 entries).
- [IUniswapV3Pool.json](IUniswapV3Pool.json): the complete Uniswap V3 core 1.0.1
  pool interface artifact `abi` array (35 entries). Only JSON formatting changes;
  no bytecode or other build data is retained.

## Licenses and notices

The original `AggregatorV3Interface.sol` declares
`// SPDX-License-Identifier: MIT`. [Chainlink MIT license](LICENSE-Chainlink-MIT.txt)
retains the copyright and MIT portion of the upstream `contracts-v1.5.0/LICENSE`;
its unrelated Go binding LGPL section does not apply to this interface.
Copyright (c) 2018 SmartContract ChainLink Limited SEZC.

The original `IUniswapV3Pool.sol` and its six inherited pool interfaces declare
`// SPDX-License-Identifier: GPL-2.0-or-later`.
[Uniswap interface license](LICENSE-Uniswap-GPL-2.0.txt) is copied verbatim from
`contracts/interfaces/LICENSE` in the package. The upstream package identifies
Uniswap Labs, copyright 2021, as the owner of Uniswap V3 Core. Its package-level
BUSL-1.1 license is not substituted for the interface-specific GPL declaration.
The ABI array is unmodified; this extraction does not change the interfaces.

## Reviewed updates

Normal generation and checks read these local files and verify their saved-byte
hashes before generating any output. They never download ABI sources. A future
update must explicitly replace the local ABI, update the source record and both
hashes, and review the applicable interface license and notices together.
Retrieve the pinned tarball in a separate review step, hash its original source
file, apply the recorded extraction, then hash the saved file. Do not install
these packages as application dependencies to regenerate the Go ABI.

Run `make generate-pricing-abi` and `make check-pricing-abi`; review every generated
byte change under [Go pricing ABIs](../../../go/internal/pricing/abis).
