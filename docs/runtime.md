# Worker runtime behavior

For commands and startup options, see [worker operation](runbooks/worker.md).

## Startup and configuration

- Startup fails before durable loops if local config is invalid or live chain state does not match the loaded YAML.

- Unsigned worker YAML fields accept only non-negative YAML integer scalars, not decimals or quoted numeric strings. Second-based values used as runtime durations must also fit the worker's duration range.

- Every configured RPC URL must report the configured EVM chain ID.

- Address fields are parsed as EVM 20-byte hex addresses during config load.

- Worker contract addresses remain required in every pathway config, even when this process runs only one role.

- `services.executor.enabled` and `services.dvn.enabled` default to true when omitted; pricing remains independently controlled.

## RPC quorum

- Chain head, safe-block, log, and state reads go through a fixed strict-majority quorum over all configured RPC providers (`q = floor(N/2) + 1` of the configured count, not of the reachable subset).

- State reads (`eth_call`, `eth_getCode`, pending nonces) vote on comparable results — successes by payload, deterministic reverts by identity — with nil-block calls anchored to the verified canonical block hash. A provider error counts as a deterministic revert only when the provider itself claims one (RPC code 3, a standalone `revert`/`reverted` word, or a `VM execution error.` carrying validated ABI revert bytes), so an opaque diagnostic payload never votes; a revert that wins its vote is marked as the quorum's verdict and terminal transaction handling consumes that marker instead of re-reading provider text.

- Gas estimates aggregate a bounded set around the upper median on each provider's latest state and reject values past the canonical block gas limit; state-read disagreement is tracked as a separate sticky per-provider conflict dimension and never re-promotes a lagging or forked provider's head status.

- The canonical head is the highest height a majority has reached with a majority-identical block hash. The indexer stops at the highest `safe` height and hash supported by a configured majority, never falls back when the tag is unavailable, and accepts log windows only when a majority returns the same sequence.

- Each concurrent stream holds an immutable safe snapshot whose log reads remain bound to that snapshot's exact voters even after another stream completes a newer safe round. Accepted safe snapshots are monotonic: a same-height hash change or height regression is rejected, and a voter set advancing the safe height must itself contain a configured quorum that still agrees with the previous accepted height/hash anchor.

- A disagreeing minority provider is flagged through head status or sticky safe/log/state conflict metrics without stopping progress; losing the majority stops the affected reads fail-closed.

- Startup establishes the head quorum before the first on-chain config read.

- Configured RPC endpoints must come from independent failure domains — duplicating one backend across URLs satisfies the count but silently voids the majority-safety assumption.

## Indexing and confirmations

- Executor and DVN source/destination cursors run as independent per-chain stream loops. `indexer_backfill_block_range` defaults to 10000 blocks per safe snapshot, while `indexer_query_block_range` defaults to 500 blocks per `eth_getLogs` request and durable cursor checkpoint. A lagging stream immediately starts its next backfill pass without waiting for `indexer_poll_interval_seconds`; caught-up, failed, and pending-destination streams wait for that interval. A failed query replays only its current query subwindow, and a pending destination event stops later subwindows without discarding earlier cursor checkpoints.

- `chains[].confirmations` is the local depth for receipt and destination-state terminalization; it does not bound indexer cursors or supply DVN assignment values. Each pathway pins the approved LayerZero values through `send_uln_confirmations` and `receive_uln_confirmations`; an indexed `DVNJobAssigned.confirmations` remains the per-packet source used by the DVN worker.

- Indexers poll through the quorum `safe` block at each chain's `indexer_poll_interval_seconds` cadence (default 5 seconds) and persist role-specific cursors in Postgres. If a configured majority cannot serve and agree on `safe`, the poll fails without advancing a cursor.

## Pricing

- Pricing transaction fee caps and minimum signer balances are configured per EID under `pricing.chains[].tx_policy`; `pricing.stale_after_seconds` cannot exceed the OpenPriceFeed one-day maximum.

- Scheduled snapshots are written only after `pricing.min_update_deviation_bps` movement or `pricing.heartbeat_seconds` elapsed. `pricing.stale_after_seconds` must exceed `pricing.heartbeat_seconds` plus `pricing.interval_seconds` by at least 600 seconds, so a scheduled heartbeat refresh always retains an enqueue/signing/confirmation margin and the pending-write readiness escalation fires while the snapshot still has headroom.

- Fee caps intentionally have no repository-wide absolute ceiling because they are chain-specific trusted operator inputs and must be approved during config review.

- Before durable loops and database initialization, worker startup performs bounded, concurrent pricing-source identity checks: it probes configured market IDs, requires Chainlink descriptions to match, requires Uniswap pools to expose the configured token pair, and confirms that each configured Uniswap TWAP window has enough observation history.

- A successful CoinMarketCap or CoinGecko response that omits the requested ID is retried up to three attempts within the source-request timeout; only repeated omission becomes a deterministic configuration mismatch.

- Stable request/identity mismatches, contract-call reverts during identity checks, an Uniswap `OLD` observation-history revert, empty or malformed ABI responses from otherwise successful identity calls, malformed or non-HTTPS market-data BaseURLs, and missing or malformed secret references fail fast.

- Market-data HTTP 403/404 responses, non-`OLD` Uniswap observation failures, timeouts, transport/RPC errors, rate limits, and upstream server failures are deferred to the supervised runtime loops, so transient pricing outages do not terminate unrelated worker loops.

- Every market-data endpoint must use HTTPS, and its client does not follow redirects to another origin.

- Runtime Chainlink reads pin `description`, `decimals`, and `latestRoundData` to one latest block number. Uniswap remains sanity-only and requires a TWAP window of at least 1800 seconds.

## Transactions and pause behavior

- Tx fees are selected at send time by `txmgr`, which estimates gas, reads current RPC fee suggestions, applies configured caps, and signs. The latest block header determines the transaction type: a base fee selects EIP-1559 dynamic fees and requires a positive priority-fee cap; without a base fee, the worker uses type-0 (legacy) fees and the priority-fee cap is optional.

- Every signed transaction is persisted as an immutable `tx_attempts` row before any node sees it: broadcasts, bounded same-raw replays, and same-nonce replacements after `tx_manager.stale_broadcast_replacement_after_seconds` all send previously persisted raw bytes, so an ambiguous send result or a crash mid-broadcast can never lose receipt tracking for a possibly accepted transaction.

- An underpriced raw is automatically repriced (10% bump under the configured caps) after a one-minute cooldown; a node's deterministic rejection or an exhausted signing or replay budget parks the signer lane as `held` for operator review instead of releasing the nonce, while an accepted broadcast that reaches the automatic replacement cap stays under receipt polling and emits an actionable recovery alert.

- A signer assigns a fresh nonce only when no earlier row is still short of broadcast (nonce-assigned, signed, or held), and the per-signer inflight window (default 8) has capacity, so already-broadcast nonces converge without allowing unbounded new nonce allocation.

- A `nonce too low` hold is reconciled automatically against the chain's confirmed account nonce: a still-unspent nonce resumes broadcasting, while an externally consumed nonce parks with evidence (fast-forwarding the local cursor) until the operator resolves it with `txretry resolve-external-nonce`.

- Any nonce-holding row can be abandoned with `txretry cancel-nonce`, which replaces it with a same-nonce noop self transfer and parks the owning job for manual review.

- A mined receipt's terminal workflow state is applied only once the receipt is buried under the chain's configured local confirmation depth and its block hash is the majority canonical hash at that height, so a short reorg cannot leave a terminal database state for a transaction the chain rolled back.

- A `signed` or `broadcast` outbox row with no active attempt is a broken invariant (the signing path writes both in one statement), so it is detected rather than tolerated: `laz_tx_outbox_orphaned_total` reports it, readiness fails with `orphaned_outbox_row` for that chain, and the operator clears the row with `txretry -action cancel-nonce`. Such a row is invisible to receipt polling and, when `signed`, blocks every higher nonce for its signer. Applying the `002_txmgr_attempts.sql` upgrade while rows are in flight is the known producer; the pre-upgrade drain requirement is in the [mainnet-readiness runbook](runbooks/mainnet-readiness.md).

- Pausing a chain or pathway (safety logic or operator) is enforced across the whole send side: after the pause commits, no new nonce is added for that scope — work selection, executor/DVN/pricing transaction enqueues, outbox signing, and automatic failure retries all hold back until the scope is active again, and the enqueue/signing decisions take share locks on the pause carriers so a racing pause cannot be missed. Transactions that already held a nonce before the pause converge to a terminal state (broadcast, replacement, reprice, reconciliation, and receipt polling continue), since freezing them would wedge the shared signer lane; `txretry cancel-nonce` remains available under pause to abandon them.

## Accounting, metrics, and supervision

- Worker fee accounting records mined receipt gas usage, converts destination-chain gas cost back into source-chain native wei with the configured pricing sources, and exposes revenue, actual cost, gross margin, negative-margin jobs, and pending reconciliation through `/metrics`.

- Worker metrics expose each active transaction signer's native balance against its configured `min_native_balance_wei` threshold.

- Retryable loop errors use exponential restart delays of 5, 10, 20, 40, then 60 seconds. Pricing instead caps the delay at its configured `interval_seconds` (starting at the smaller of five seconds and that interval). An uninterrupted run of at least the greater of five minutes and twice the cap resets the delay; unexpected successful returns also back off. Cancellation interrupts the wait and non-retryable loop errors stop `App.Run` immediately.

- Runtime price-source rejections stay inside the pricing loop. A failed EID cools down for one `interval_seconds` after the read completes, shared by periodic and gas-spike evaluation; independent feeds continue. A timer evaluates again at cooldown expiry, so recovery can wait one full interval after the preceding failure, plus read/enqueue/confirmation time. No stale observation is reused. Cooldowns survive supervisor re-entry of the same bot but not process restart. Single-shot pricing still reports total failure as an error.
