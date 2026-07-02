# Price Bot Runbook

This runbook covers the phase-1 price config update path for OpenExecutor and OpenDVN.

## Preconditions

- `pricing.enabled: true` in the validated worker config.
- The pricing signer is present in `signers` and has funds on every configured source chain.
- Every configured chain has the configured primary source identifier, Uniswap V3 sanity route, and at least one healthy RPC URL.
- `primary_source` defaults to `binance`; supported values are `binance`, `coinmarketcap`, and `coingecko`.
- CoinMarketCap API keys must be referenced through `coinmarketcap_api_key_env` whenever `coinmarketcap_symbol` is configured; do not put API keys in worker YAML.
- `base_fee_wei`, `buffer_bps`, `stale_after_seconds`, `gas_spike_bps`, and configured gas price/fee caps are approved for the target environment. `base_fee_wei` is the worker quote base fee, not an EIP-1559 block header base fee; the tx manager derives legacy versus dynamic-fee signing and estimates outer transaction gas from RPC before broadcast.
- `pathways[].source_workers` addresses and pathway EIDs match the latest deployment record.

## One-Shot Update

Run one price calculation and enqueue the resulting worker transactions:

```bash
go run ./go/cmd/pricebot-once -config <worker.yaml>
```

The command checks the loaded chain/pathway config against on-chain Endpoint, OApp, SendLib, ReceiveLib, ULN, OpenExecutor, and OpenDVN state before database sync. It then runs DB migrations, syncs the validated chain/pathway config, reads the configured primary source, CoinMarketCap/CoinGecko sanity sources when configured, Uniswap sanity prices, and destination gas prices from RPC, then enqueues `setPriceConfig` transactions for each unique `(src_eid, dst_eid, source_workers.open_executor, source_workers.open_dvn)` pair. It does not bypass the normal transaction manager or signer boundary; the tx manager still signs, broadcasts, replaces, and records receipts from the Postgres outbox.

## Expected Outbox Effects

For each unique source/destination/source-worker pair, the command should enqueue:

- one `pricing_set_executor_price_config` transaction to the source-chain OpenExecutor
- one `pricing_set_dvn_price_config` transaction to the source-chain OpenDVN

If the primary source and any configured sanity source deviate beyond `max_deviation_bps`, no price update should be enqueued and the command should exit non-zero.

During the long-running worker loop, the bot also tracks the last destination gas price used for each unique source/destination/source-worker pair. If a later destination gas read increases by at least `gas_spike_bps`, it enqueues a fresh OpenExecutor/OpenDVN price update before the next scheduled interval.

## Verification

After the tx manager broadcasts and confirms the queued transactions:

1. Confirm the tx outbox rows for the pricing signer reached a terminal confirmed status.
2. Run `npm run check:price-config` on the source chain for the target `DST_EID`.
3. Confirm `updatedAt` is recent and `staleAfter` matches the approved config.
4. Confirm `dstGasPriceInSrcToken` is non-zero and consistent with the recorded gas/price inputs.
5. Confirm `gas_spike_bps` matches the approved config and is included in config-diff review evidence.
6. Confirm `getFee`/`getFeeOnSend` succeeds before the stale window expires.
7. Confirm stale configs still cause worker quote/assignment reverts in tests before enabling mainnet use.

Example:

```bash
RPC_URL=... \
CHAIN_ID=11155111 \
OPEN_EXECUTOR=... \
OPEN_DVN=... \
DST_EID=40245 \
MAX_PRICE_AGE_SECONDS=300 \
EXPECTED_STALE_AFTER=1800 \
npm run check:price-config
```

## Rollback

If the newly submitted price config is wrong:

1. Pause sends for affected pathways when pricing could undercharge execution.
2. Restore the previous approved config values with `contracts/scripts/configure-workers.ts` or a manually reviewed owner transaction.
3. Restart the worker after updating config files; phase 1 does not support hot reload.
4. Let txmgr automatic retry handle classified pricing outbox failures. Use `txretry` only after automatic retry is exhausted or after the signer, fee caps, and calldata have been reviewed for an operator override.
