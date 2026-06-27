# LayerZero Testnet Addresses

Latest refresh command:

```bash
npm run check:lz-addresses
```

Latest committed result: passed for Ethereum Sepolia `40161` and Base Sepolia `40245`.

Sources:

- LayerZero chain page: `https://docs.layerzero.network/v2/deployments/chains/sepolia`
- LayerZero chain page: `https://docs.layerzero.network/v2/deployments/chains/base-sepolia`
- Protocol contract data: `https://docs.layerzero.network/public/data/deploymentsV2.json`
- DVN data: `https://docs.layerzero.network/public/data/dvnDeployments.json`

Refresh check:

```bash
npm run check:lz-addresses
```

Run this immediately before any funded testnet migration or mainnet proposal.

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

Use `LayerZero Labs DVN`, not the lzRead DVN, for the phase-1 `requiredDVNs = [OpenDVN, LayerZero Labs DVN]` push-DVN configuration.

## Base Sepolia

| Field                                 | Value                                        |
| ------------------------------------- | -------------------------------------------- |
| Chain key                             | `base-sepolia`                               |
| Native chain ID                       | `84532`                                      |
| LayerZero EID                         | `40245`                                      |
| EndpointV2                            | `0x6EDCE65403992e310A62460808c4b910D972f10f` |
| SendUln302                            | `0xC1868e054425D378095A003EcbA3823a5D0135C9` |
| ReceiveUln302                         | `0x12523de19dc41c91F7d2093E0CFbB76b17012C8d` |
| LayerZero Executor                    | `0x8A3D588D9f6AC041476b094f97FF94ec30169d3D` |
| LayerZero `lzExecutor` metadata entry | `0xD8C74c92a59c2b5b6390eD54f13193C59249e561` |
| Dead DVN                              | `0x78551ADC2553EF1858a558F5300F7018Aad2FA7e` |
| LayerZero Labs DVN                    | `0xe1a12515f9ab2764b887bf60b923ca494ebbb2d6` |
| LayerZero Labs lzRead DVN             | `0xbf6ff58f60606edb2f190769b951d825bcb214e2` |

Use `LayerZero Labs DVN`, not the lzRead DVN, for the phase-1 `requiredDVNs = [OpenDVN, LayerZero Labs DVN]` push-DVN configuration.

## Direction Inputs

For Ethereum Sepolia -> Base Sepolia:

- `REMOTE_EID=40245`
- `ENDPOINT=0x6EDCE65403992e310A62460808c4b910D972f10f`
- `SEND_ULN=0xcc1ae8Cf5D3904Cef3360A9532B477529b177cCE`
- `RECEIVE_ULN=0xdAf00F5eE2158dD58E0d3857851c432E34A3A851`
- `LAYERZERO_LABS_DVN=0x8eebf8b423b73bfca51a1db4b7354aa0bfca9193`

For Base Sepolia -> Ethereum Sepolia:

- `REMOTE_EID=40161`
- `ENDPOINT=0x6EDCE65403992e310A62460808c4b910D972f10f`
- `SEND_ULN=0xC1868e054425D378095A003EcbA3823a5D0135C9`
- `RECEIVE_ULN=0x12523de19dc41c91F7d2093E0CFbB76b17012C8d`
- `LAYERZERO_LABS_DVN=0xe1a12515f9ab2764b887bf60b923ca494ebbb2d6`

Re-check the source URLs immediately before any funded testnet migration or mainnet proposal; LayerZero metadata is external operational state.
