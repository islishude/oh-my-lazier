# Contract Scripts

These scripts use the compiled Hardhat artifacts and `viem`. They require `npm run compile` before execution. TypeScript scripts accept CLI flags using kebab-case names derived from the previous environment variable names, such as `--rpc-url` for `RPC_URL` and `--test-oft` for `TEST_OFT`; environment variables remain a fallback for secrets and automation. Deployment uses the Hardhat Ignition `TestOFTWorkers` module; the scripts in this directory handle post-deploy configuration, inspection, evidence checks, canaries, and rollback.

Before any funded testnet migration, confirm the committed LayerZero address list still matches current official metadata:

```bash
npm run check:lz-addresses
```

The check compares `docs/deployments/layerzero-testnet-addresses.md` values encoded in the script against LayerZero's current `deploymentsV2.json` and `dvnDeployments.json` metadata.

Run the local dual-Anvil E2E from the repository root:

```bash
make e2e-local
```

The target writes all generated files under `tmp/e2e`, starts disposable
Postgres, isolated Anvil, and worker services through `docker-compose.e2e.yml`,
runs `npm run e2e:deploy-local` to deploy local EndpointV2, SendUln302,
ReceiveUln302, TestOFT, OpenExecutor, a primary OpenDVN, and a secondary
OpenDVN on both chains, generates a worker keystore, runs `configcheck`, and
then runs `npm run e2e:run-local`. The local runner sends OFT canaries in both
directions. It does not start the price bot; deployment writes fresh hard-coded
worker price configs, with source OpenExecutor pricing non-zero and local
OpenDVN pricing zero because pinned SendUln302 accounts DVN fees internally
without forwarding native value to DVN `assignJob`.

The active DVN transaction target is the destination OpenDVN, not
ReceiveUln302 directly. `e2e:run-local` requires the worker-submitted primary
OpenDVN verification and a script-submitted secondary OpenDVN verification,
then checks that ReceiveUln302 emitted `PayloadVerified` for both OpenDVNs,
EndpointV2 emitted `PacketDelivered`, and the recipient TestOFT balance
increased.

When registry access is unavailable, set `ANVIL_IMAGE` to a compatible local
Foundry image. If `oh-my-lazier-worker:e2e` already exists, set
`E2E_WORKER_UP_FLAGS=--no-build` to reuse it instead of rebuilding the worker
image.

Before approving a migration ticket, validate that the ticket has attached the required local evidence references:

```bash
npm run check:migration-evidence -- \
  --migration-evidence docs/deployments/testnet-migration-evidence.example.json
```

The migration evidence checker verifies that the ticket includes `make check`, LayerZero address refresh, DB-backed readiness check, key/price/rate-limit/monitoring/runbook/security review evidence, that the only phase-1 directions are Ethereum Sepolia `40161` <-> Hoodi `40449`, that each direction records source and destination worker contracts plus config diff, deployment preflight, LayerZero config before/after, price config evidence tied to the destination EID and freshness window, drain, canary amount/sender/recipient/minimum balance/receipt/balance-check evidence, DVN join config with positive `confirmations` and `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`, and DVN verification evidence tied to the exact payload hash and PacketV1 identity, and that rollback evidence includes previous Executor/ULN configs, rollback dry-run output, restored config check, post-rollback canary, owner pause account, signer account, drain status, and manual retry plan.

Deploy the local pathway contracts with Hardhat Ignition:

```bash
SEPOLIA_RPC_URL=... \
PRIVATE_KEY=... \
npm run deploy:workers -- \
  --network sepolia \
  --parameters ignition/parameters/sepolia.json \
  --deployment-id sepolia-test-oft-workers
```

```bash
HOODI_RPC_URL=... \
PRIVATE_KEY=... \
npm run deploy:workers -- \
  --network hoodi \
  --parameters ignition/parameters/hoodi.json \
  --deployment-id hoodi-test-oft-workers
```

Use `docs/deployments/test-oft-policy.md` for the approved TestOFT name, symbol, owner, constructor mint, and minting policy. The committed parameter files default `OWNER` and `INITIAL_RECIPIENT` to the deploying account; include explicit `owner` and `initialRecipient` module parameters when the approved operations owner or canary treasury differs from the deployer.

After deployment, check that the deployed contracts are still controlled by the expected operations owner and, when used, that the canary treasury has enough native token and TestOFT balance for the planned transfer:

```bash
npm run check:deployment-preflight -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --test-oft ... \
  --open-executor ... \
  --open-dvn ... \
  --expected-owner ... \
  --min-owner-native-balance 10000000000000000 \
  --canary-treasury ... \
  --min-canary-native-balance 10000000000000000 \
  --min-canary-token-balance 1000000000000000 \
  --expected-total-supply 1000000000000000000000000
```

`--canary-treasury`, `--min-canary-native-balance`, `--min-canary-token-balance`, and `--expected-total-supply` are optional. Use `--expected-total-supply 0` on Hoodi when checking the initial zero-supply deployment before any inbound canary mint.

Inspect or update one TestOFT pathway pause/rate-limit state during migration:

```bash
npm run oft:pathway -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --test-oft ... \
  --remote-eid 40449 \
  --oft-pathway-action inspect
```

For state-changing actions, include `--private-key`. Supported `--oft-pathway-action` values are `pause-send`, `unpause-send`, `pause-receive`, `unpause-receive`, `drain`, and `set-rate-limit`. `drain` sets `capacity=0` and `refillPerSecond=0`; `set-rate-limit` requires `--rate-limit-capacity` and `--rate-limit-refill-per-second`.

```bash
npm run oft:pathway -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --test-oft ... \
  --remote-eid 40449 \
  --oft-pathway-action drain
```

Configure the OFT and Endpoint baseline for one local direction after both sides are deployed. The render step produces the `TestOFTPathwayConfig` Ignition parameters for:

- `TestOFT.setDelegate`
- `TestOFT.setPeer`
- `EndpointV2.setSendLibrary`
- `EndpointV2.setReceiveLibrary`
- `EndpointV2.setConfig` for SendUln302 `ExecutorConfig` and `UlnConfig`
- `EndpointV2.setConfig` for ReceiveUln302 `UlnConfig`
- `TestOFT.setEnforcedOptions`
- `OpenExecutor.setAllowedSendLib` and `OpenDVN.setAllowedSendLib`
- `OpenExecutor.setPathwayConfig` and `OpenDVN.setPathwayConfig`
- `OpenExecutor.setPriceConfig` and `OpenDVN.setPriceConfig`
- `OpenDVN.setVerifier` for the local verifier signer

```bash
npm run render:oft-pathway-params -- \
  --test-oft ... \
  --endpoint 0x6EDCE65403992e310A62460808c4b910D972f10f \
  --remote-eid 40449 \
  --remote-oft ... \
  --send-uln 0xcc1ae8Cf5D3904Cef3360A9532B477529b177cCE \
  --receive-uln 0xdAf00F5eE2158dD58E0d3857851c432E34A3A851 \
  --open-executor ... \
  --open-dvn ... \
  --layerzero-labs-dvn 0x8eebf8b423b73bfca51a1db4b7354aa0bfca9193 \
  --confirmations 12 \
  --executor-max-message-size 10000 \
  --enforced-lz-receive-gas 200000 \
  --max-lz-receive-gas 1000000 \
  --executor-price-base-fee 0 \
  --executor-price-dst-gas-price-in-src-token 1 \
  --executor-price-buffer-bps 1000 \
  --executor-price-stale-after 1800 \
  --dvn-price-base-fee 0 \
  --dvn-price-dst-gas-price-in-src-token 1 \
  --dvn-price-buffer-bps 1000 \
  --dvn-price-stale-after 1800 > ignition/parameters/sepolia-to-hoodi.generated.json
```

`--min-lz-receive-gas` defaults to `--enforced-lz-receive-gas`. `--executor-price-updated-at` and `--dvn-price-updated-at` default to the render script's current Unix timestamp; regenerate parameters shortly before signing so the price configs remain fresh for the approved stale window. `--dvn-verifier` defaults to the Ignition sender, and must match the destination-chain `tx_roles.dvn.signer` before switching a pathway to active DVN mode.

```bash
SEPOLIA_RPC_URL=... \
PRIVATE_KEY=... \
npm run configure:oft-pathway -- \
  --network sepolia \
  --parameters ignition/parameters/sepolia-to-hoodi.generated.json \
  --deployment-id sepolia-to-hoodi-oft-pathway-config
```

Repeat the same flow on Hoodi with Hoodi's local endpoint/message libraries, `REMOTE_EID=40161`, the Sepolia `REMOTE_OFT`, and `--network hoodi`. `DELEGATE` is optional in the render step; when set, it must match the account that will sign the same configuration run, because the later Endpoint calls require the signer to be the OApp delegate.

Refresh local OpenExecutor/OpenDVN price inputs or rate limits separately when needed:

```bash
npm run configure:workers -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --test-oft ... \
  --open-executor ... \
  --open-dvn ... \
  --remote-eid 40449 \
  --send-lib ... \
  --max-message-size 10000 \
  --min-lz-receive-gas 200000 \
  --max-lz-receive-gas 1000000 \
  --price-base-fee 0 \
  --price-dst-gas-price-in-src-token 1 \
  --price-buffer-bps 1000 \
  --price-stale-after 3600
```

For standard pathway setup, `configure:oft-pathway` already writes the worker send-lib allowlist, pathway limits, initial price configs, and verifier authorization. `configure:workers` remains the manual entrypoint for later worker price refreshes and outbound rate limit changes. `--src-oapp` defaults to `--test-oft`. Set `--rate-limit-capacity` and `--rate-limit-refill-per-second` together to configure outbound rate limiting.

Inspect the current LayerZero libraries and message-lib configs for one local direction:

```bash
npm run inspect:lz-config -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --endpoint ... \
  --oapp ... \
  --remote-eid 40449 \
  --send-uln ... \
  --receive-uln ...
```

Save this output before executor or DVN migration. The saved JSON is the rollback input for restoring the previous ExecutorConfig and both SendUln302/ReceiveUln302 UlnConfig values:

```bash
npm run inspect:lz-config -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --endpoint ... \
  --oapp ... \
  --remote-eid 40449 \
  --send-uln ... \
  --receive-uln ... > sepolia-to-hoodi-lz-config.before.json
```

Switch the source send library executor config to OpenExecutor:

```bash
npm run configure:lz-executor -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --endpoint ... \
  --oapp ... \
  --remote-eid 40449 \
  --send-uln ... \
  --open-executor ... \
  --executor-max-message-size 10000
```

Configure ULN302 on one local chain so OpenDVN and the LayerZero Labs DVN are both required:

```bash
npm run configure:lz-dvn -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --endpoint ... \
  --oapp ... \
  --remote-eid 40449 \
  --send-uln ... \
  --receive-uln ... \
  --open-dvn ... \
  --layerzero-labs-dvn ... \
  --confirmations 12
```

Restore a previously inspected LayerZero config snapshot:

```bash
npm run configure:lz-rollback -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --lz-config-snapshot sepolia-to-hoodi-lz-config.before.json
```

Use `--dry-run` with the same `--lz-config-snapshot` before signing. The dry run validates the snapshot and prints the exact `Endpoint.setConfig` batches and encoded config bytes without requiring RPC or `--private-key`.

`TestOFT` has no post-deploy owner mint function, so there is no `mint:oft` script. Sepolia's test supply is minted only by the `TestOFTWorkers` constructor parameters, and Hoodi supply is created by successful inbound OFT receive-side minting.

Send a basic OFT transfer after the local and remote OFTs are peered and the pathway is configured:

```bash
npm run send:oft -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --private-key ... \
  --test-oft ... \
  --dst-eid 40449 \
  --recipient ... \
  --amount-ld 1000000000000000 \
  --min-amount-ld 1000000000000000
```

`send:oft` calls `quoteSend` first and pays the quoted native fee. By default it sends empty caller `extraOptions` and relies on the pathway's enforced `lzReceiveOption`; this avoids duplicate executor options after `configure:oft-pathway` has set enforced options. Pass `--extra-options 0x...` only for an explicitly approved custom options payload, or `--lz-receive-gas <gas>` only on pathways without enforced lzReceive options. `composeMsg` and `oftCmd` are always empty for the first phase. `send:oft-canary` remains as an alias with canary-specific logging for migration evidence flows.

Check the source-chain canary receipt after the send transaction is mined:

```bash
npm run check:oft-canary -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --endpoint ... \
  --source-tx-hash ... \
  --send-lib ... \
  --open-executor ...
```

`check:oft-canary` verifies that the source receipt includes EndpointV2 `PacketSent`, that `PacketSent.sendLibrary` matches `SEND_LIB`, and that SendLib `ExecutorFeePaid.executor` matches `OPEN_EXECUTOR`.

After the destination delivery transaction is known, run the same command against the destination RPC with `DESTINATION_TX_HASH` and optional `DESTINATION_ENDPOINT`:

```bash
npm run check:oft-canary -- \
  --rpc-url ... \
  --chain-id 560048 \
  --endpoint ... \
  --destination-tx-hash ... \
  --destination-endpoint ... \
  --destination-test-oft ... \
  --recipient ... \
  --min-recipient-balance 1000000000000000
```

The destination check requires EndpointV2 `PacketDelivered`, rejects receipts containing `LzReceiveAlert`, and can optionally require the recipient's TestOFT balance to be at least `MIN_RECIPIENT_BALANCE`. `SOURCE_TX_HASH` checks are intended for the source-chain RPC; `DESTINATION_TX_HASH` checks are intended for the destination-chain RPC.

The migration evidence record must capture each direction's source OpenExecutor/OpenDVN contracts, destination OpenDVN contract, canary `AMOUNT_LD`, sender account, recipient account, `MIN_RECIPIENT_BALANCE`, source receipt, destination receipt, and recipient balance check. The `priceConfigCheck` object must capture the `DST_EID`, `MAX_PRICE_AGE_SECONDS`, `EXPECTED_STALE_AFTER`, `checkedAt`, and each worker's `updatedAt`, `staleAfter`, and non-zero `dstGasPriceInSrcToken` values from `check:price-config`. The `dvnVerificationReceipt` object must also capture the `EXPECTED_PAYLOAD_HASH`, `EXPECTED_SRC_EID`, `EXPECTED_DST_EID`, `EXPECTED_NONCE`, `EXPECTED_SENDER`, and `EXPECTED_RECEIVER` values used by `check:dvn-verification`. This keeps the approval artifact tied to the exact transfer size, accounts, worker contracts, price freshness, packet, and direction used in the rehearsal.

After DVN join, check a destination-chain verification receipt for both required DVNs:

```bash
npm run check:dvn-verification -- \
  --rpc-url ... \
  --chain-id 560048 \
  --tx-hash ... \
  --receive-uln ... \
  --open-dvn ... \
  --layerzero-labs-dvn ... \
  --confirmations 12 \
  --endpoint ...
```

`check:dvn-verification` requires ReceiveUln302 `PayloadVerified` logs for both OpenDVN and the LayerZero Labs DVN with at least `CONFIRMATIONS`. For the active OpenDVN path, the worker transaction calls `OpenDVN.submitVerification`, and OpenDVN calls `ReceiveUln302.verify` so the `PayloadVerified.dvn` field is the OpenDVN address. When `ENDPOINT` is set, it also requires EndpointV2 `PacketVerified` in the same receipt. `EXPECTED_PAYLOAD_HASH` is optional and filters the checked `PayloadVerified` logs to one payload hash.
Set `EXPECTED_SRC_EID`, `EXPECTED_DST_EID`, `EXPECTED_NONCE`, `EXPECTED_SENDER`, and `EXPECTED_RECEIVER` to also require the `PayloadVerified` PacketV1 header to match the exact migration direction and packet identity.

Run the LayerZero config module or lower-level scripts on both chains with the local endpoint, local OApp, local message libraries, and local DVN addresses for each direction. `configure:oft-pathway` and `configure:lz-dvn` explicitly set `optionalDVNCount` to LayerZero's NIL value so default optional DVNs are not inherited during the first-phase required-DVN migration.
