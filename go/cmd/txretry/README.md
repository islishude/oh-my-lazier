# txretry

Inspect and recover durable worker transactions in Postgres. Run the examples
from the repository root. `-id` is a positive **`tx_outbox.id`**, not a transaction
hash or packet GUID.

Start with inspection and diagnose the lowest outstanding nonce for the signer:

```bash
go run ./go/cmd/txretry -config <worker.yaml> -action inspect -id <tx_outbox_id>
```

## Arguments

| Argument | Meaning |
| --- | --- |
| `-config` | Worker YAML configuration; defaults to `config.yaml` in the current working directory. |
| `-action` | Required: one of the six actions below. |
| `-id` | Required: positive outbox row ID. |
| `-rpc-url` | Required only for `rebroadcast`; rejected by other actions. Supports HTTP(S), WS(S), and absolute IPC paths. |
| `-resolution` | Required only for `resolve-external-nonce`: `retry` or `abandon`; rejected by other actions. |

All actions load the worker configuration and connect to its database. The
command does not run migrations or start worker loops. Only `rebroadcast` sends
to RPC directly; the other recovery actions update durable database state for
the worker to process. Keep the worker running to sign, broadcast, and confirm
requested recovery transactions. These actions execute when invoked; there is
no dry-run or confirmation flag.

## Actions

| Action | Effect | When to use |
| --- | --- | --- |
| `inspect` | Read-only JSON diagnostics: signer lane head, current and historical attempts, budgets, recovery evidence, and next action times. | Before choosing a recovery action. |
| `retry-failed` | Requeues a failed row in place if it has no mined failure, preserving any assigned nonce. Receipt-failed rows are cloned for a fresh nonce, preserving the original evidence. | Retry the business operation after its failure cause is resolved. |
| `replace` | Registers one same-nonce replacement of the current business transaction, with increased fees. | A broadcast transaction is stuck, underpriced, or has exhausted its broadcast budget. |
| `rebroadcast` | Immediately sends the current persisted signed bytes once through the selected RPC. Fees, nonce, and hash stay unchanged. | Recover RPC propagation; this cannot fix an underpriced transaction. |
| `cancel-nonce` | Requests a same-nonce zero-value self transfer to abandon the business attempt. | Clear an unconsumed nonce that is blocking the signer. |
| `resolve-external-nonce` | Terminates a row whose nonce was consumed externally, then either clones the business task or abandons it. | Resolve `held(nonce_consumed_externally)` after reviewing the external nonce use. |

### retry-failed

```bash
go run ./go/cmd/txretry -config <worker.yaml> -action retry-failed -id <tx_outbox_id>
```

Requires `status = failed`. Canceled rows, externally consumed rows, and rows
with a pinned receipt that cannot follow the receipt-failed clone path are
rejected. Workflow state and retry limits still apply: an `lzReceive` retry
cannot reactivate a workflow that has already advanced or exhausted its budget.
Retries do not bypass paused or disabled send scopes.

Failed pricing observations normally must be rebuilt by the price bot, so
nonce-less and receipt-failed pricing rows are rejected. A historical failed
pricing row still holding an unconsumed nonce can be requeued in place to fill
that gap, subject to the per-feed in-flight guard.

### replace

```bash
go run ./go/cmd/txretry -config <worker.yaml> -action replace -id <tx_outbox_id>
```

Requires an active attempt and either `broadcast`, `held(reprice_required)`, or
`held(broadcast_exhausted)`. Pending cancels and other hold reasons are rejected.
The request resets the signing-failure budget and authorizes one additional
replacement beyond the automatic cap and cooldowns. The worker uses the same
nonce and business content, with fees at least 10% above the previous attempt
and no lower than the fresh RPC suggestion. Configured fee caps still apply;
when a cap prevents the bump, the worker defers the request. Fee configuration
changes require a worker restart.

### rebroadcast

```bash
go run ./go/cmd/txretry -config <worker.yaml> -action rebroadcast -id <tx_outbox_id> -rpc-url <rpc_url>
```

Requires the lowest outstanding nonce, a current signed/ambiguous/submitted
attempt, and `signed`, `broadcast`, or `held(broadcast_exhausted)` state. Other
holds, terminal rows, pinned receipts, missing attempts, and active signing or
broadcast leases are rejected. A pending cancel blocks replay of the original
attempt; an active cancel attempt can itself be replayed.

The command validates the RPC and signed transaction chain IDs, signature,
sender, nonce, hash, and canonical bytes without loading signer keys. Each
invocation authorizes one send beyond automatic replay caps and cooldowns, while
preserving the cumulative broadcast count. It grants no extra automatic replay
budget. Keep RPC credentials out of retained shell history and evidence.

### cancel-nonce

```bash
go run ./go/cmd/txretry -config <worker.yaml> -action cancel-nonce -id <tx_outbox_id>
```

Requires an assigned nonce in `queued`, `nonce_assigned`, `signed`, `broadcast`,
or `held` state, without a pinned receipt or an externally consumed nonce hold.
A nonce-holding row with no active attempt is also cancelable.

The first request clears any pending business replacement request. If the
active attempt is already a cancel, invoking this action again authorizes one
cancel fee bump. The cancel remains subject to fee caps. Once the cancel is
canonically confirmed, the row ends as `failed/canceled` and the owning job is
parked for manual review where applicable. A cancel can race an already
authorized original broadcast; it cannot undo a mined transaction.

### resolve-external-nonce

```bash
# Re-execute the business task with a fresh nonce.
go run ./go/cmd/txretry -config <worker.yaml> -action resolve-external-nonce -resolution retry -id <tx_outbox_id>

# Abandon the task and park its owning job for manual review.
go run ./go/cmd/txretry -config <worker.yaml> -action resolve-external-nonce -resolution abandon -id <tx_outbox_id>
```

Both resolutions require `held(nonce_consumed_externally)` with no pinned
receipt. The old row becomes terminal with the external-consumption evidence
retained. `retry` atomically creates a queued clone; its send scope must be
active and its owning workflow must still be waiting for the transaction.
Pricing rows reject `retry`: use `abandon` and let the price bot rebuild from a
fresh observation. `abandon` creates no clone and leaves an already-advanced
workflow untouched.

External nonce consumption violates the single-broadcaster assumption. Review
the signer's access and external activity before deciding whether to re-execute
the task; see [key management](../../../docs/runbooks/key-management.md).

## Results and confirmation

`inspect` prints diagnostics without raw signed transactions, signatures, or RPC
credentials. Successful database recovery actions print `action`,
`request_status: registered`, and `before`/`after` inspections. When an action
creates a clone, `after` describes the new outbox ID. Registration does not mean
the worker has sent or confirmed a transaction.

`rebroadcast` prints `outbox_id`, `attempt_id`, `tx_hash`, `send_class`, `detail`,
and `recorded` alongside the action. Only RPC acceptance (including
`already known`) exits successfully. **RPC acceptance is not receipt
confirmation.** Timeouts and unknown send errors are ambiguous; a writeback
failure with `recorded: false` can also mean the RPC accepted the transaction.
Inspect and allow worker receipt tracking to reconcile before retrying.

Existing nonce holders can converge while paused, including replacement,
rebroadcast, and cancellation. New business retries remain subject to scope
gates. Final receipt handling requires the configured confirmation depth and
canonical block verification.

See [durable transaction recovery](../../../docs/runbooks/monitoring.md#durable-transaction-recovery)
for automatic recovery timing, alerts, concurrency guards, and operational
procedures.
