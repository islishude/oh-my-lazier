# Monitoring Checklist

The first-phase worker exposes HTTP health and Prometheus text metrics from `metrics.listen_address`.

Endpoints:

- `/healthz`: process liveness only. It returns `200` without touching Postgres.
- `/readyz`: readiness check. It returns `200` only when the worker can read the DB stats snapshot and the DB-backed readiness rules pass.
- `/metrics`: Prometheus text metrics derived from durable Postgres state.

Required scrape target:

```text
http://<worker-host>:9090/metrics
```

Required alerts:

- `laz_chain_paused == 1` for any chain: page immediately. This means chain-wide quorum safety logic paused the worker path.
- `laz_pathway_paused == 1` for any pathway: page immediately. This means packet-level receipt/log conflict safety logic paused a pathway.
- `laz_dvn_jobs_total{status="QUORUM_CONFLICT"} > 0`: page immediately and inspect source RPC providers before unpausing.
- `laz_dvn_jobs_total{status="REORG_DETECTED"} > 0`: page if it persists past the next confirmation loop; inspect source RPC providers and source transaction receipts.
- `laz_packets_total{status="MANUAL_REVIEW"} > 0`: ticket within one business day; page if count increases during migration.
- `laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} > 0`: ticket and inspect destination `LzReceiveAlert` logs.
- `laz_executor_jobs_total{status="MANUAL_REVIEW"} > 0` or `laz_dvn_jobs_total{status="MANUAL_REVIEW"} > 0`: ticket and block migration approval until reviewed.
- `laz_tx_outbox_total{status="failed"} > 0`: ticket; page if failure count increases for active migration chains.
- Missing `laz_indexer_cursor_last_block` movement for an enabled chain over the expected polling window: page if the chain is actively used.
- `/readyz` returns non-200 for more than two scrape intervals: page.

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

Operational assumptions:

- Metrics are derived from committed DB state, not in-memory counters, so a worker restart should not reset packet/job/outbox visibility.
- `/healthz` is only a liveness probe. Use `/readyz` and `/metrics` for operational readiness and alerting.
- Do not unpause a chain or pathway until the conflict source is identified and the latest `inspect:lz-config` output still matches the intended migration config.
