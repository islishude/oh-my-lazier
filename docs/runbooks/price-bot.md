# Price Bot Runbook

This runbook covers the phase-1 shared price snapshot update path for source-chain OpenPriceFeed contracts used by OpenExecutor and OpenDVN.

## Preconditions

- `pricing.enabled: true` in the validated worker config.
- The pricing signer is present in `signers`, every `pricing.chains[].tx_policy.min_native_balance_wei` is configured, and `laz_signer_native_balance_wei` is above `laz_signer_min_native_balance_wei` on every configured source chain.
- The pricing signer is authorized as an `OpenPriceFeed` submitter on every configured source chain. The PriceFeed owner manages submitters, but owner status alone does not submit snapshots. Profile-driven deployments temporarily authorize the owner only for initial snapshot configuration and revoke it before handoff.
- Every configured pricing chain declares `native_asset_id`, `data_fee_per_byte_wei`, and at least one healthy RPC URL. Use an explicit `"0"` data fee for routes that do not charge an L2-style per-byte data fee.
- Pathways whose source and destination pricing chains share the same `native_asset_id` use 1:1 native-token conversion and do not need any market source. Cross-asset pathways require exactly one `primary_source` selected from `coinmarketcap`, `coingecko`, or `chainlink`.
- `sanity_sources` is optional and may reference any configured non-primary source, including optional Chainlink and Uniswap V3 TWAP readers. Sanity sources validate the primary but never replace it. When sanity sources are declared, at least one must be healthy and every healthy sanity observation must remain within `max_deviation_bps` of the primary.
- Every referenced source declares an explicit freshness limit. The bot rejects missing, non-positive, stale, or materially future-dated observations. An unavailable primary always stops the update; unavailable sanity sources are tolerated only while at least one other declared sanity source remains healthy.
- CoinMarketCap uses a numeric asset `id`, the V3 quotes endpoint, and `coinmarketcap_api_key_env`; never put the key itself in YAML. CoinGecko uses its coin `id`, requests `last_updated_at`, and optionally reads a Pro key through `coingecko_api_key_env`. When that key reference is set and `coingecko_base_url` is omitted, the worker selects the CoinGecko Pro endpoint automatically; an explicit BaseURL still overrides the default.
- Chainlink is optional. When referenced, `feed_address` must be an AggregatorV3 proxy on the pricing chain, `expected_description` must match the approved USD/native feed, and `max_age_seconds` must account for that feed's approved heartbeat.
- Uniswap is optional and may only be a sanity source. When referenced, `pool_address`, `token_in`, and `token_out` must identify the approved V3 native/stablecoin pool. The reader uses `observe()` over `twap_window_seconds`, checks latest-block age and `min_harmonic_mean_liquidity`, and treats `token_out` as the approved USD reference token. It does not use a QuoterV2 spot quote.
- The Sepolia/Hoodi same-native ETH testnet profile does not require Chainlink, CoinMarketCap, CoinGecko, or Uniswap.
- `pathways[].pricing.executor_fee`, `pathways[].pricing.dvn_fee`, `pricing.stale_after_seconds`, `pricing.gas_spike_bps`, and every chain's `pricing.chains[].tx_policy` are approved for the target environment. `stale_after_seconds` must not exceed the OpenPriceFeed one-day maximum. The fee model fields are worker quote inputs, not EIP-1559 block header base fees; the tx manager derives legacy versus dynamic-fee signing and estimates outer transaction gas from RPC before broadcast.
- Each pathway fee model has `fixed_fee_wei`, `dst_gas_overhead`, `data_size_overhead_bytes`, and `margin_bps`; the bot validates those fee models for shared-worker conflicts, but only writes high-frequency market price snapshot batches to `pathways[].source_workers.price_feed`.
- `pathways[].source_workers`, including `source_workers.price_feed`, `pathways[].destination_workers.open_dvn`, and pathway EIDs match the latest deployment record.
- Pathways that share the same source-chain worker contract and destination EID must use the same fee model for that worker role; configcheck verifies `feeModel(dstEid)` on each worker when pricing is enabled.

## One-Shot Update

Run one price calculation and enqueue the resulting worker transactions:

```bash
go run ./go/cmd/pricebot-once -config <worker.yaml> -log-level debug
```

The command checks the loaded chain/pathway config against on-chain Endpoint, OApp, SendLib, ReceiveLib, source and destination ULN required DVNs, OpenPriceFeed, OpenExecutor, source OpenDVN, destination OpenDVN code, worker `priceFeed()` bindings, worker `feeModel(dstEid)` values when pricing is enabled, pricing signer `submitters(address)` authorization, and active destination OpenDVN verifier authorization before database sync. It then runs DB migrations, syncs the validated chain/pathway config, reads destination gas prices from RPC, uses 1:1 conversion for same-`native_asset_id` pathways, and concurrently reads each selected primary with only its declared sanity sources for cross-asset pathways. Every unique EID is priced once per cycle. All required price and gas inputs must be healthy before the command enqueues any snapshot, preventing a partially updated cycle. It converts the configured destination `data_fee_per_byte_wei` into source-token units, then enqueues one batched `setPriceSnapshot(PriceSnapshotUpdate[])` transaction for each unique `(src_eid, source_workers.price_feed)` key. It does not bypass the normal transaction manager or signer boundary; the tx manager still signs, broadcasts, replaces, and records receipts from the Postgres outbox.

Before database initialization, long-running worker startup performs one bounded, concurrent source-identity preflight. It verifies that configured market IDs resolve, Chainlink feed descriptions match, and Uniswap pools expose the configured token pair; confirmed semantic mismatches fail fast alongside malformed market-data BaseURLs and missing API-key environment values. The preflight does not require current price freshness or gas-price health. Timeouts, transport/RPC errors, rate limits, and upstream server failures are logged and deferred to the supervised pricing and fee-accounting loops. Those transient failures remain fail-closed for the affected cycle, enqueue no partial snapshot, and restart only the affected loop instead of terminating the worker process.

## Expected Outbox Effects

For each unique source/source price-feed key, the command should enqueue:

- one `pricing_set_price_snapshot` transaction to the source-chain OpenPriceFeed using a `PriceSnapshotUpdate[]` batch for every destination EID sharing that source/feed

For cross-asset pathways, an unavailable, stale, future-dated, or non-positive primary prevents every update in that cycle. If sanity sources are declared, all being unavailable or any healthy sanity observation exceeding `max_deviation_bps` also prevents every update. A healthy sanity source never replaces the primary.

During the long-running worker loop, the bot still tracks the last destination gas price used for each unique source/destination/source price-feed key. If later destination gas reads increase by at least `gas_spike_bps`, it groups triggered destinations by source/feed and enqueues fresh snapshot update batches before the next scheduled interval.

## Verification

After the tx manager broadcasts and confirms the queued transactions:

1. Confirm the tx outbox rows for the pricing signer reached a terminal confirmed status.
2. Run `npm run check:price-config` on the source chain for the target `DST_EID`.
3. Confirm `updatedAt` is recent, not in the future, and `staleAfter` matches the approved config without exceeding the contract's one-day maximum.
4. Confirm shared `dstGasPriceInSrcToken`, `dstDataFeePerByteInSrcToken`, and stale window match the recorded gas/data/price inputs, and each worker's `dstGasOverhead`, `dataSizeOverheadBytes`, `marginBps`, and ABI-facing `baseFee` match the approved executor/DVN fee models derived from `fixed_fee_wei`.
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
