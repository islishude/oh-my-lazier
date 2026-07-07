# Price Bot Runbook

This runbook covers the phase-1 shared price snapshot update path for source-chain OpenPriceFeed contracts used by OpenExecutor and OpenDVN.

## Preconditions

- `pricing.enabled: true` in the validated worker config.
- The pricing signer is present in `signers`, has `pricing.min_native_balance_wei` configured, and `laz_signer_native_balance_wei` is above `laz_signer_min_native_balance_wei` on every configured source chain.
- The pricing signer is authorized as an `OpenPriceFeed` submitter on every configured source chain. The PriceFeed owner manages submitters, but owner status alone does not submit snapshots.
- Every configured pricing chain declares `primary_source`, `sanity_sources`, `data_fee_per_byte_wei`, a Uniswap V3 sanity route, and at least one healthy RPC URL. Use an explicit `"0"` data fee for routes that do not charge an L2-style per-byte data fee.
- Supported primary sources are `binance`, `coinmarketcap`, and `coingecko`. `sanity_sources` may use those sources plus `uniswap`, must include `uniswap`, and must not duplicate the primary source.
- CoinMarketCap API keys must be referenced through `coinmarketcap_api_key_env` whenever `coinmarketcap` is used as a primary or sanity source; do not put API keys in worker YAML.
- `pathways[].pricing.executor_fee`, `pathways[].pricing.dvn_fee`, `pricing.stale_after_seconds`, `pricing.gas_spike_bps`, and configured gas price/fee caps are approved for the target environment. The fee model fields are worker quote inputs, not EIP-1559 block header base fees; the tx manager derives legacy versus dynamic-fee signing and estimates outer transaction gas from RPC before broadcast.
- Each pathway fee model has `fixed_fee_wei`, `dst_gas_overhead`, `data_size_overhead_bytes`, and `margin_bps`; the bot validates those fee models for shared-worker conflicts, but only writes high-frequency market price snapshot batches to `pathways[].source_workers.price_feed`.
- `pathways[].source_workers`, including `source_workers.price_feed`, `pathways[].destination_workers.open_dvn`, and pathway EIDs match the latest deployment record.
- Pathways that share the same source-chain worker contract and destination EID must use the same fee model for that worker role; configcheck verifies `feeModel(dstEid)` on each worker when pricing is enabled.

## One-Shot Update

Run one price calculation and enqueue the resulting worker transactions:

```bash
go run ./go/cmd/pricebot-once -config <worker.yaml> -log-level debug
```

The command checks the loaded chain/pathway config against on-chain Endpoint, OApp, SendLib, ReceiveLib, source and destination ULN required DVNs, OpenPriceFeed, OpenExecutor, source OpenDVN, destination OpenDVN code, worker `priceFeed()` bindings, worker `feeModel(dstEid)` values when pricing is enabled, pricing signer `submitters(address)` authorization, and active destination OpenDVN verifier authorization before database sync. It then runs DB migrations, syncs the validated chain/pathway config, reads each chain's configured primary feed, configured sanity feeds, Uniswap sanity prices, and destination gas prices from RPC, converts the configured destination `data_fee_per_byte_wei` into source-token units, then enqueues one batched `setPriceSnapshot(PriceSnapshotUpdate[])` transaction for each unique `(src_eid, source_workers.price_feed)` key. It does not bypass the normal transaction manager or signer boundary; the tx manager still signs, broadcasts, replaces, and records receipts from the Postgres outbox.

## Expected Outbox Effects

For each unique source/source price-feed key, the command should enqueue:

- one `pricing_set_price_snapshot` transaction to the source-chain OpenPriceFeed using a `PriceSnapshotUpdate[]` batch for every destination EID sharing that source/feed

If the primary source and any configured sanity source deviate beyond `max_deviation_bps`, no price update should be enqueued and the command should exit non-zero.

During the long-running worker loop, the bot still tracks the last destination gas price used for each unique source/destination/source price-feed key. If later destination gas reads increase by at least `gas_spike_bps`, it groups triggered destinations by source/feed and enqueues fresh snapshot update batches before the next scheduled interval.

## Verification

After the tx manager broadcasts and confirms the queued transactions:

1. Confirm the tx outbox rows for the pricing signer reached a terminal confirmed status.
2. Run `npm run check:price-config` on the source chain for the target `DST_EID`.
3. Confirm `updatedAt` is recent and `staleAfter` matches the approved config.
4. Confirm shared `dstGasPriceInSrcToken`, `dstDataFeePerByteInSrcToken`, and stale window match the recorded gas/data/price inputs, and each worker's `dstGasOverhead`, `dataSizeOverheadBytes`, `marginBps`, and `baseFee` match the approved executor/DVN fee models.
5. Confirm `gas_spike_bps` matches the approved config and is included in config-diff review evidence.
6. Confirm `getFee`/`getFeeOnSend` succeeds before the stale window expires.
7. Confirm stale configs still cause worker quote/assignment reverts in tests before enabling mainnet use.

Example:

```bash
npm run check:price-config -- \
  --rpc-url ... \
  --chain-id 11155111 \
  --price-feed ... \
  --open-executor ... \
  --open-dvn ... \
  --dst-eid 40449 \
  --max-price-age-seconds 300 \
  --expected-stale-after 1800
```

## Rollback

If the newly submitted price snapshot is wrong:

1. Pause sends for affected pathways when pricing could undercharge execution.
2. Restore the previous approved snapshot with `contracts/scripts/configure-workers.ts` or a manually reviewed submitter transaction. If the configured `source_workers.price_feed` changed, rotate OpenExecutor/OpenDVN back with `setPriceFeed`; only use worker fee-model updates when the low-frequency model itself was wrong.
3. Restart the worker after updating config files; phase 1 does not support hot reload.
4. Let txmgr automatic retry and pending replacement handle classified pricing outbox failures or stale broadcasts. Use `txretry` only after automatic retry is exhausted or after the signer balance, fee caps, and calldata have been reviewed for an operator override.
