# Monitoring Checklist

The first-phase worker exposes HTTP health and Prometheus text metrics from `metrics.listen_address`.

Endpoints:

- `/healthz`: process liveness only. It returns `200` without touching Postgres.
- `/readyz`: readiness check. It returns `200` only when the worker can read the DB stats snapshot and the DB-backed readiness rules pass.
- `/metrics`: Prometheus text metrics derived from durable Postgres state plus process-local indexer polling and worker-loop retry status.

Required scrape target:

```text
http://<worker-host>:9090/metrics
```

Required alert rules are tracked in `docs/monitoring/prometheus-alerts.yml`.
`npm run check:runbooks` verifies that the documented high-signal alerts remain
present and linked back to this runbook.

Required alerts:

- `LazWorkerReadinessFailed`: `/readyz` returns non-200 for more than two scrape intervals; page.
- `LazChainPaused`: `laz_chain_paused == 1` for any chain; page immediately. This means chain-wide quorum safety logic paused the worker path.
- `LazPathwayPaused`: `laz_pathway_paused == 1` for any pathway; page immediately. This means packet-level receipt/log conflict safety logic paused a pathway.
- `LazDVNQuorumConflict`: `laz_dvn_jobs_total{status="QUORUM_CONFLICT"} > 0`; page immediately and inspect source RPC providers before unpausing.
- `LazDVNReorgDetected`: `laz_dvn_jobs_total{status="REORG_DETECTED"} > 0`; page if it persists past the next confirmation loop; inspect source RPC providers and source transaction receipts.
- `LazPacketManualReview`: `laz_packets_total{status="MANUAL_REVIEW"} > 0`; ticket within one business day; page if count increases during migration.
- `LazExecutorReceiveFailed`: `laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} > 0`; ticket and inspect destination `LzReceiveAlert` logs.
- `LazWorkerManualReview`: `laz_executor_jobs_total{status="MANUAL_REVIEW"} > 0` or `laz_dvn_jobs_total{status="MANUAL_REVIEW"} > 0`; ticket and block migration approval until reviewed.
- `LazTxOutboxFailed`: `laz_tx_outbox_total{status="failed"} > 0`; ticket; page if failure count increases for active migration chains.
- `LazIndexerPollFailing`: `laz_indexer_poll_success == 0` for a configured indexer; page after the failure persists across polling intervals.
- `LazIndexerCursorStalled`: missing `laz_indexer_cursor_last_block` movement for an enabled chain over the expected polling window; page if the chain is actively used.

Active worker status paths:

- DVN active verification: `ASSIGNED -> WAITING_CONFIRMATIONS -> QUORUM_CHECKING -> READY_TO_VERIFY -> VERIFY_TX_ENQUEUED -> VERIFIED`.
- Executor delivery: `ASSIGNED -> WAITING_DVN_VERIFICATION -> VERIFIABLE -> COMMIT_TX_ENQUEUED -> COMMITTED -> EXECUTABLE -> LZ_RECEIVE_TX_ENQUEUED -> DELIVERED`.
- Shadow DVN verification stops at `WOULD_VERIFY`; it must not enqueue `dvn_verify` transactions.
- `QUORUM_CONFLICT`, `REORG_DETECTED`, `MANUAL_REVIEW`, `LZ_RECEIVE_FAILED`, and `tx_outbox.status="failed"` are operator-action states, not healthy terminal states.

Before any migration approval, run the DB-backed readiness gate and attach the JSON output:

```bash
go run ./go/cmd/readinesscheck -config <worker.yaml> -format json
```

The readiness gate fails if an enabled chain is paused, an enabled pathway between enabled chains is paused, an active chain has failed `tx_outbox` rows, a packet/job is in a manual-review or failed/conflict/reorg state, or an enabled pathway's required source/destination indexer cursor is missing or has not advanced past block `0`.

Migration dashboard panels:

- Chain enabled/paused status by `eid` and `name`.
- Pathway paused status by `src_eid` and `dst_eid`.
- Packet count by pathway and status.
- Executor job count by status.
- DVN job count by status.
- Tx outbox count by chain and status.
- Indexer cursor last block by chain and stream.
- Indexer poll success, last poll duration, observed head, confirmed block upper bound, and last error timestamp by chain.
- Worker loop retry count and last retry timestamp by loop name.

Operational assumptions:

- Packet, job, outbox, pause, and cursor metrics are derived from committed DB state, so a worker restart should not reset that visibility. Indexer poll status and worker loop retry metrics are process-local and reset on restart.
- If Postgres-backed stats are temporarily unavailable, `/metrics` still exposes process-local indexer and worker loop retry metrics and sets `laz_metrics_db_snapshot_available 0`; `/readyz` remains unavailable until the DB-backed readiness snapshot succeeds.
- `/healthz` is only a liveness probe. Use `/readyz` and `/metrics` for operational readiness and alerting.
- Do not unpause a chain or pathway until the conflict source is identified and the latest `inspect:lz-config` output still matches the intended migration config.
