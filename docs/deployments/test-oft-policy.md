# TestOFT Deployment Policy

This policy fixes the phase-1 TestOFT deployment parameters for Ethereum Sepolia and Hoodi rehearsal.

## Token Identity

| Field          | Value                                             |
| -------------- | ------------------------------------------------- |
| `TOKEN_NAME`   | `Oh My Lazier Test OFT`                           |
| `TOKEN_SYMBOL` | `OMLTOFT`                                         |
| Local decimals | inherited from LayerZero OFT / ERC20 default `18` |

## Ownership

`OWNER` must be the testnet operations owner for all three contracts deployed by the `TestOFTWorkers` Hardhat Ignition module:

- `TestOFT`
- `OpenExecutor`
- `OpenDVN`

The owner must be able to:

- configure OFT peers
- pause and unpause TestOFT send/receive pathways
- configure outbound rate limits
- configure worker allowlists, pathway limits, and price config
- withdraw worker balances during rollback or cleanup

Do not use the worker hot signer as `OWNER` unless the migration ticket explicitly approves that temporary testnet shortcut.

After deployment and before any funded migration step, run `npm run check:deployment-preflight` on each chain with `EXPECTED_OWNER` set to the approved operations owner. Set `CANARY_TREASURY` and the minimum native/TestOFT balances when canary transfers will be sent from a treasury instead of directly from the owner.

## Initial Supply

Use a single constructor mint on Ethereum Sepolia:

| Chain            | `INITIAL_SUPPLY`            | `INITIAL_RECIPIENT`                         |
| ---------------- | --------------------------- | ------------------------------------------- |
| Ethereum Sepolia | `1000000000000000000000000` | testnet operations owner or canary treasury |
| Hoodi             | `0`                         | testnet operations owner or canary treasury |

The value above is `1,000,000 OMLTOFT` with 18 decimals. Hoodi starts with zero supply so destination balances are created only by LayerZero receive-side minting during canary transfers. Reverse-direction canaries must first use tokens minted on Hoodi by a successful Ethereum Sepolia -> Hoodi transfer.

## Minting Policy

`TestOFT` has no post-deploy owner mint function. The only direct mint is the optional constructor mint controlled by `INITIAL_SUPPLY`.

After deployment, supply movement is limited to the OFT burn/mint flow:

- source-chain send burns local tokens
- destination-chain receive mints local tokens
- pause and rate-limit controls gate the flow per pathway

Changing this policy requires updating this document, the affected runbooks, and migration evidence expectations before deployment.
