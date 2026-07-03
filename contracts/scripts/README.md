# Contract Scripts

These scripts use the compiled Hardhat artifacts and `viem`. They require `npm run compile` before execution and read all network-specific values from environment variables.

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
MIGRATION_EVIDENCE=docs/deployments/testnet-migration-evidence.example.json \
npm run check:migration-evidence
```

The migration evidence checker verifies that the ticket includes `make check`, LayerZero address refresh, DB-backed readiness check, key/price/rate-limit/monitoring/runbook/security review evidence, that the only phase-1 directions are Ethereum Sepolia `40161` <-> Base Sepolia `40245`, that each direction records source and destination worker contracts plus config diff, deployment preflight, LayerZero config before/after, price config evidence tied to the destination EID and freshness window, drain, canary amount/sender/recipient/minimum balance/receipt/balance-check evidence, DVN join config with positive `confirmations` and `requiredDVNs = [OpenDVN, LayerZero Labs DVN]`, and DVN verification evidence tied to the exact payload hash and PacketV1 identity, and that rollback evidence includes previous Executor/ULN configs, rollback dry-run output, restored config check, post-rollback canary, owner pause account, signer account, drain status, and manual retry plan.

Deploy the local pathway contracts:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OWNER=... \
TOKEN_NAME="Oh My Lazier Test OFT" \
TOKEN_SYMBOL=OMLTOFT \
INITIAL_RECIPIENT=<owner-or-canary-treasury> \
INITIAL_SUPPLY=1000000000000000000000000 \
npm run deploy:workers
```

Use `docs/deployments/test-oft-policy.md` for the approved TestOFT name, symbol, owner, constructor mint, and minting policy. For Base Sepolia deployment, keep the same `TOKEN_NAME` and `TOKEN_SYMBOL` but set `INITIAL_SUPPLY=0`.

After deployment, check that the deployed contracts are still controlled by the expected operations owner and, when used, that the canary treasury has enough native token and TestOFT balance for the planned transfer:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
TEST_OFT=... \
OPEN_EXECUTOR=... \
OPEN_DVN=... \
EXPECTED_OWNER=... \
MIN_OWNER_NATIVE_BALANCE=10000000000000000 \
CANARY_TREASURY=... \
MIN_CANARY_NATIVE_BALANCE=10000000000000000 \
MIN_CANARY_TOKEN_BALANCE=1000000000000000 \
EXPECTED_TOTAL_SUPPLY=1000000000000000000000000 \
npm run check:deployment-preflight
```

`CANARY_TREASURY`, `MIN_CANARY_NATIVE_BALANCE`, `MIN_CANARY_TOKEN_BALANCE`, and `EXPECTED_TOTAL_SUPPLY` are optional. Use `EXPECTED_TOTAL_SUPPLY=0` on Base Sepolia when checking the initial zero-supply deployment before any inbound canary mint.

Inspect or update one TestOFT pathway pause/rate-limit state during migration:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
TEST_OFT=... \
REMOTE_EID=40245 \
OFT_PATHWAY_ACTION=inspect \
npm run oft:pathway
```

For state-changing actions, include `PRIVATE_KEY`. Supported `OFT_PATHWAY_ACTION` values are `pause-send`, `unpause-send`, `pause-receive`, `unpause-receive`, `drain`, and `set-rate-limit`. `drain` sets `capacity=0` and `refillPerSecond=0`; `set-rate-limit` requires `RATE_LIMIT_CAPACITY` and `RATE_LIMIT_REFILL_PER_SECOND`.

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
TEST_OFT=... \
REMOTE_EID=40245 \
OFT_PATHWAY_ACTION=drain \
npm run oft:pathway
```

Configure one local direction after both sides are deployed:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
TEST_OFT=... \
OPEN_EXECUTOR=... \
OPEN_DVN=... \
REMOTE_EID=40245 \
REMOTE_OFT=... \
SEND_LIB=... \
MAX_MESSAGE_SIZE=10000 \
MIN_LZ_RECEIVE_GAS=200000 \
MAX_LZ_RECEIVE_GAS=1000000 \
PRICE_BASE_FEE=0 \
PRICE_DST_GAS_PRICE_IN_SRC_TOKEN=1 \
PRICE_BUFFER_BPS=1000 \
PRICE_STALE_AFTER=3600 \
npm run configure:workers
```

`SRC_OAPP` defaults to `TEST_OFT`. Set `RATE_LIMIT_CAPACITY` and `RATE_LIMIT_REFILL_PER_SECOND` together to configure outbound rate limiting.

Inspect the current LayerZero libraries and message-lib configs for one local direction:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
RECEIVE_ULN=... \
npm run inspect:lz-config
```

Save this output before executor or DVN migration. The saved JSON is the rollback input for restoring the previous ExecutorConfig and both SendUln302/ReceiveUln302 UlnConfig values:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
RECEIVE_ULN=... \
npm run inspect:lz-config > sepolia-to-base-lz-config.before.json
```

Switch the source send library executor config to OpenExecutor:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
OPEN_EXECUTOR=... \
EXECUTOR_MAX_MESSAGE_SIZE=10000 \
npm run configure:lz-executor
```

Configure ULN302 on one local chain so OpenDVN and the LayerZero Labs DVN are both required:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
ENDPOINT=... \
OAPP=... \
REMOTE_EID=40245 \
SEND_ULN=... \
RECEIVE_ULN=... \
OPEN_DVN=... \
LAYERZERO_LABS_DVN=... \
CONFIRMATIONS=12 \
npm run configure:lz-dvn
```

Restore a previously inspected LayerZero config snapshot:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
LZ_CONFIG_SNAPSHOT=sepolia-to-base-lz-config.before.json \
npm run configure:lz-rollback
```

Use `DRY_RUN=1` with the same `LZ_CONFIG_SNAPSHOT` before signing. The dry run validates the snapshot and prints the exact `Endpoint.setConfig` batches and encoded config bytes without requiring RPC or `PRIVATE_KEY`.

Send a basic OFT canary transfer after the local and remote OFTs are peered and the pathway is configured:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
PRIVATE_KEY=... \
TEST_OFT=... \
DST_EID=40245 \
RECIPIENT=... \
AMOUNT_LD=1000000000000000 \
MIN_AMOUNT_LD=1000000000000000 \
LZ_RECEIVE_GAS=200000 \
npm run send:oft-canary
```

`send:oft-canary` calls `quoteSend` first and pays the quoted native fee. It builds exactly one canonical zero-value executor `lzReceiveOption`; `composeMsg` and `oftCmd` are always empty for the first phase.

Check the source-chain canary receipt after the send transaction is mined:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
ENDPOINT=... \
SOURCE_TX_HASH=... \
SEND_LIB=... \
OPEN_EXECUTOR=... \
npm run check:oft-canary
```

`check:oft-canary` verifies that the source receipt includes EndpointV2 `PacketSent`, that `PacketSent.sendLibrary` matches `SEND_LIB`, and that SendLib `ExecutorFeePaid.executor` matches `OPEN_EXECUTOR`.

After the destination delivery transaction is known, run the same command against the destination RPC with `DESTINATION_TX_HASH` and optional `DESTINATION_ENDPOINT`:

```bash
RPC_URL=... \
CHAIN_ID=84532 \
ENDPOINT=... \
DESTINATION_TX_HASH=... \
DESTINATION_ENDPOINT=... \
DESTINATION_TEST_OFT=... \
RECIPIENT=... \
MIN_RECIPIENT_BALANCE=1000000000000000 \
npm run check:oft-canary
```

The destination check requires EndpointV2 `PacketDelivered`, rejects receipts containing `LzReceiveAlert`, and can optionally require the recipient's TestOFT balance to be at least `MIN_RECIPIENT_BALANCE`. `SOURCE_TX_HASH` checks are intended for the source-chain RPC; `DESTINATION_TX_HASH` checks are intended for the destination-chain RPC.

The migration evidence record must capture each direction's source OpenExecutor/OpenDVN contracts, destination OpenDVN contract, canary `AMOUNT_LD`, sender account, recipient account, `MIN_RECIPIENT_BALANCE`, source receipt, destination receipt, and recipient balance check. The `priceConfigCheck` object must capture the `DST_EID`, `MAX_PRICE_AGE_SECONDS`, `EXPECTED_STALE_AFTER`, `checkedAt`, and each worker's `updatedAt`, `staleAfter`, and non-zero `dstGasPriceInSrcToken` values from `check:price-config`. The `dvnVerificationReceipt` object must also capture the `EXPECTED_PAYLOAD_HASH`, `EXPECTED_SRC_EID`, `EXPECTED_DST_EID`, `EXPECTED_NONCE`, `EXPECTED_SENDER`, and `EXPECTED_RECEIVER` values used by `check:dvn-verification`. This keeps the approval artifact tied to the exact transfer size, accounts, worker contracts, price freshness, packet, and direction used in the rehearsal.

After DVN join, check a destination-chain verification receipt for both required DVNs:

```bash
RPC_URL=... \
CHAIN_ID=84532 \
TX_HASH=... \
RECEIVE_ULN=... \
OPEN_DVN=... \
LAYERZERO_LABS_DVN=... \
CONFIRMATIONS=12 \
ENDPOINT=... \
npm run check:dvn-verification
```

`check:dvn-verification` requires ReceiveUln302 `PayloadVerified` logs for both OpenDVN and the LayerZero Labs DVN with at least `CONFIRMATIONS`. For the active OpenDVN path, the worker transaction calls `OpenDVN.submitVerification`, and OpenDVN calls `ReceiveUln302.verify` so the `PayloadVerified.dvn` field is the OpenDVN address. When `ENDPOINT` is set, it also requires EndpointV2 `PacketVerified` in the same receipt. `EXPECTED_PAYLOAD_HASH` is optional and filters the checked `PayloadVerified` logs to one payload hash.
Set `EXPECTED_SRC_EID`, `EXPECTED_DST_EID`, `EXPECTED_NONCE`, `EXPECTED_SENDER`, and `EXPECTED_RECEIVER` to also require the `PayloadVerified` PacketV1 header to match the exact migration direction and packet identity.

Run the LayerZero config scripts on both chains with the local endpoint, local OApp, local message libraries, and local DVN addresses for each direction. `configure:lz-dvn` explicitly sets `optionalDVNCount` to LayerZero's NIL value so default optional DVNs are not inherited during the first-phase required-DVN migration.
