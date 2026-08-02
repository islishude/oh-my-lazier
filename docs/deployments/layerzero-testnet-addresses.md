# LayerZero Testnet Addresses

Latest refresh command:

```bash
npm run check:lz-addresses
```

Latest committed result: passed for Ethereum Sepolia `40161` and Hoodi `40449`.

Sources:

- LayerZero chain page: `https://docs.layerzero.network/v2/deployments/chains/sepolia`
- LayerZero chain page: `https://docs.layerzero.network/v2/deployments/chains/hoodi-testnet`
- Protocol contract data: `https://docs.layerzero.network/public/data/deploymentsV2.json`
- DVN data for optional external-DVN selection:
  `https://docs.layerzero.network/public/data/dvnDeployments.json`

Refresh check:

```bash
npm run check:lz-addresses
```

Run this immediately before any funded testnet migration or mainnet proposal.
The automated check validates protocol addresses from `deploymentsV2.json`;
operator-selected external DVNs are reviewed through profile and evidence
inputs instead.

## Ethereum Sepolia

| Field                                 | Value                                        |
| ------------------------------------- | -------------------------------------------- |
| Chain key                             | `sepolia`                                    |
| Native chain ID                       | `11155111`                                   |
| LayerZero EID                         | `40161`                                      |
| EndpointV2                            | `0x6EDCE65403992e310A62460808c4b910D972f10f` |
| SendUln302                            | `0xcc1ae8Cf5D3904Cef3360A9532B477529b177cCE` |
| ReceiveUln302                         | `0xdAf00F5eE2158dD58E0d3857851c432E34A3A851` |
| LayerZero Executor                    | `0x718B92b5CB0a5552039B593faF724D182A881eDA` |
| LayerZero `lzExecutor` metadata entry | `0x34a561197e4eAe356D41B0B02C59F12a5C576C5A` |
| Dead DVN                              | `0x8b450b0acF56E1B0e25C581bB04FBAbeeb0644b8` |
| LayerZero Labs DVN                    | `0x8eebf8b423b73bfca51a1db4b7354aa0bfca9193` |
| LayerZero Labs lzRead DVN             | `0x530fbe405189204ef459fa4b767167e4d41e3a37` |

LayerZero Labs DVN is one optional external DVN choice for Sepolia/Hoodi
rehearsal. If it is selected, use the push DVN address, not the lzRead DVN,
with `chains[].includeLayerZeroLabsDVN: true` in deployment profiles. For
lower-level commands, include it explicitly in the JSON
`input.requiredDVNs` address array.

## Hoodi

| Field                                 | Value                                            |
| ------------------------------------- | ------------------------------------------------ |
| Chain key                             | `hoodi-testnet`                                  |
| Native chain ID                       | `560048`                                         |
| LayerZero EID                         | `40449`                                          |
| EndpointV2                            | `0x3aCAAf60502791D199a5a5F0B173D78229eBFe32`     |
| SendUln302                            | `0x45841dd1ca50265Da7614fC43A361e526c0e6160`     |
| ReceiveUln302                         | `0xd682ECF100f6F4284138AA925348633B0611Ae21`     |
| LayerZero Executor                    | `0x701f3927871EfcEa1235dB722f9E608aE120d243`     |
| LayerZero `lzExecutor` metadata entry | `0x4Cf1B3Fa61465c2c907f82fC488B43223BA0CF93`     |
| Dead DVN                              | `0x88B27057A9e00c5F05DDa29241027afF63f9e6e0`     |
| LayerZero Labs DVN                    | `0xa78a78a13074ed93ad447a26ec57121f29e8fec2`     |
| LayerZero Labs lzRead DVN             | not currently published in `dvnDeployments.json` |

LayerZero Labs DVN is one optional external DVN choice for Sepolia/Hoodi
rehearsal. If it is selected, use the push DVN address, not the lzRead DVN,
with `chains[].includeLayerZeroLabsDVN: true` in deployment profiles. For
lower-level commands, include it explicitly in the JSON
`input.requiredDVNs` address array.

## Direction Inputs

For Ethereum Sepolia -> Hoodi:

- Hardhat network: `sepolia`
- `input.remoteEid`: `"40449"`
- `input.endpoint`: `"0x6EDCE65403992e310A62460808c4b910D972f10f"`
- `input.sendUln`: `"0xcc1ae8Cf5D3904Cef3360A9532B477529b177cCE"`
- `input.receiveUln`: `"0xdAf00F5eE2158dD58E0d3857851c432E34A3A851"`
- Profile convenience: `includeLayerZeroLabsDVN: true`
- `input.requiredDVNs`: `["<sepolia-open-dvn>", "0x8eebf8b423b73bfca51a1db4b7354aa0bfca9193"]`

For Hoodi -> Ethereum Sepolia:

- Hardhat network: `hoodi`
- `input.remoteEid`: `"40161"`
- `input.endpoint`: `"0x3aCAAf60502791D199a5a5F0B173D78229eBFe32"`
- `input.sendUln`: `"0x45841dd1ca50265Da7614fC43A361e526c0e6160"`
- `input.receiveUln`: `"0xd682ECF100f6F4284138AA925348633B0611Ae21"`
- Profile convenience: `includeLayerZeroLabsDVN: true`
- `input.requiredDVNs`: `["<hoodi-open-dvn>", "0xa78a78a13074ed93ad447a26ec57121f29e8fec2"]`

The lower-level examples use the strict envelope in
`config/scripts/examples`; pass them through `OML_SCRIPT_PARAMS` and specify
`--network sepolia` or `--network hoodi`. RPC URLs and credentials come from
Hardhat configuration variables, not these JSON inputs.

Re-check the source URLs immediately before any funded testnet migration or mainnet proposal; LayerZero metadata is external operational state.
