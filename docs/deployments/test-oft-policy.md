# TestOFT Deployment Policy

This policy fixes the phase-1 TestOFT deployment parameters for Ethereum Sepolia and Hoodi rehearsal.

## Token Identity

| Field          | Value                                             |
| -------------- | ------------------------------------------------- |
| `TOKEN_NAME`   | `Oh My Lazier Test OFT`                           |
| `TOKEN_SYMBOL` | `OMLTOFT`                                         |
| Local decimals | inherited from LayerZero OFT / ERC20 default `18` |

## Ownership

`OWNER` must be the testnet operations owner for the Sepolia/Hoodi rehearsal
contracts deployed by the split Hardhat Ignition modules:

- `TestOFT` from the `TestOFT` rehearsal module
- `OpenPriceFeed`, `OpenExecutor`, and `OpenDVN` from the `OpenWorkers` module

The owner must be able to:

- configure OFT peers
- mint TestOFT supply with the owner-only `mint(address,uint256)` function
- exercise TestOFT `multiSend` during local and testnet rehearsal checks
- pause and unpause TestOFT send/receive pathways
- configure outbound rate limits
- configure worker allowlists, pathway limits, PriceFeed submitters, and fee models
- withdraw worker balances during rollback or cleanup

Do not use the worker hot signer as `OWNER` unless the migration ticket explicitly approves that temporary testnet shortcut.

After deployment and before any funded migration step, run
`npm run check:deployment-preflight` on each chain with `EXPECTED_OWNER` set to
the approved operations owner. Set `CANARY_TREASURY`, the minimum native
balance, and the chain-specific minimum TestOFT balance when canary transfers
will be sent from a treasury instead of directly from the owner. Hoodi's
initial minimum TestOFT balance is `0` because its initial supply is `0`; raise
that threshold only after a successful inbound Sepolia -> Hoodi canary.

For the Sepolia/Hoodi rehearsal, keep these values in the testnet deployment
profile and generate downstream artifacts from it:

```bash
SEPOLIA_RPC_URL=... \
HOODI_RPC_URL=... \
npm run deploy:profile -- \
  --profile config/deployments/template.json \
  --phase render
```

The profile is the maintained operator input for owner, long-term PriceFeed
submitters, initial recipient, per-chain canary TestOFT balance thresholds,
worker signer addresses, fee caps, worker fee models, and the environment
variable names that hold RPC URLs. The owner is
added as a temporary deployment submitter only while the initial worker pathway
price snapshot is configured, then `OpenWorkersPathwayConfig` revokes that
temporary authorization. Hardhat private key configuration variables
are defined in `hardhat.config.ts` and must be stored with `hardhat-keystore`
before state-changing Ignition commands. Do not copy contract
addresses from terminal output into worker YAML or pathway parameter files by
hand; regenerate from the Ignition deployment state instead. The normal
configuration path uses `OAppEndpointConfig` for rehearsal OApp/Endpoint state
and `OpenWorkersPathwayConfig` for worker-side state, generated from the same
profile and deployment state. The generated worker YAML is an operational
artifact under `tmp/`, not a maintained policy document. For fresh deployments,
omit `chains[].startBlockNumber` so the renderer writes the latest RPC block
height to worker `chains[].start_block_number`; set it explicitly only when a
fixed historical backfill is approved.

## Initial Supply

Use a single constructor mint on Ethereum Sepolia:

| Chain            | `INITIAL_SUPPLY`            | `INITIAL_RECIPIENT`                         |
| ---------------- | --------------------------- | ------------------------------------------- |
| Ethereum Sepolia | `1000000000000000000000000` | testnet operations owner or canary treasury |
| Hoodi            | `0`                         | testnet operations owner or canary treasury |

The value above is `1,000,000 OMLTOFT` with 18 decimals. Hoodi starts with zero supply so destination balances are created only by LayerZero receive-side minting during canary transfers. Reverse-direction canaries must first use tokens minted on Hoodi by a successful Ethereum Sepolia -> Hoodi transfer.

## Minting Policy

`TestOFT` includes an owner-only post-deploy `mint(address,uint256)` function.
The approved default direct mint is still the optional constructor mint
controlled by `INITIAL_SUPPLY`; any post-deploy owner mint requires an explicit
migration ticket approval that records the chain, recipient, amount, rationale,
and supply-risk acceptance before signing.

Without an approved post-deploy owner mint, post-deploy supply movement is
limited to the OFT burn/mint flow:

- source-chain send burns local tokens
- destination-chain receive mints local tokens
- pause and rate-limit controls gate the flow per pathway

Changing this policy requires updating this document, the affected runbooks, and migration evidence expectations before deployment.

## Multi-Send Rehearsal

`TestOFT` includes `quoteMultiSend` and `multiSend` so local and testnet
rehearsals can produce multiple OFT sends from one source-chain transaction.
The local dual-Anvil E2E uses this to check that the DB-backed indexer stores
both packet rows and their executor/DVN jobs for the same source transaction
hash. Do not add separate Ignition deployment modules or deployment-profile
phases for this capability; it is part of the TestOFT rehearsal contract.
