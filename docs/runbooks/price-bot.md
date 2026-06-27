# Price Bot Runbook

This runbook covers the phase-1 price config update path for OpenExecutor and OpenDVN.

## Preconditions

- `pricing.enabled: true` in the validated worker config.
- The pricing signer is present in `signers` and has funds on every configured source chain.
- Every configured chain has a Binance symbol, Uniswap V3 sanity route, and at least one healthy RPC URL.
- `base_fee_wei`, `buffer_bps`, `stale_after_seconds`, EIP-1559 fee caps, and `tx_gas_limit` are approved for the target environment.
- Worker contract addresses and pathway EIDs match the latest deployment record.

## One-Shot Update

Run one price calculation and enqueue the resulting worker transactions:

```bash
go run ./go/cmd/pricebot-once -config <worker.yaml>
```

The command runs DB migrations, syncs the validated chain/pathway config, reads Binance and Uniswap prices, reads destination gas prices from RPC, then enqueues `setPriceConfig` transactions for both OpenExecutor and OpenDVN per configured pathway. It does not bypass the normal transaction manager or signer boundary; the tx manager still signs, broadcasts, replaces, and records receipts from the Postgres outbox.

## Expected Outbox Effects

For each unique `src_eid -> dst_eid` pathway, the command should enqueue:

- one `pricing_set_executor_price_config` transaction to the source-chain OpenExecutor
- one `pricing_set_dvn_price_config` transaction to the source-chain OpenDVN

If Binance and Uniswap deviate beyond `max_deviation_bps`, no price update should be enqueued and the command should exit non-zero.

## Verification

After the tx manager broadcasts and confirms the queued transactions:

1. Confirm the tx outbox rows for the pricing signer reached a terminal confirmed status.
2. Read `priceConfig(dstEid)` on source-chain OpenExecutor and OpenDVN.
3. Confirm `updatedAt` is recent and `staleAfter` matches the approved config.
4. Confirm `dstGasPriceInSrcToken` is non-zero and consistent with the recorded gas/price inputs.
5. Confirm `getFee`/`getFeeOnSend` succeeds before the stale window expires.
6. Confirm stale configs still cause worker quote/assignment reverts in tests before enabling mainnet use.

## Rollback

If the newly submitted price config is wrong:

1. Pause sends for affected pathways when pricing could undercharge execution.
2. Restore the previous approved config values with `contracts/scripts/configure-workers.ts` or a manually reviewed owner transaction.
3. Restart the worker after updating config files; phase 1 does not support hot reload.
4. Retry failed pricing outbox rows only after the signer, fee caps, and calldata have been reviewed.
