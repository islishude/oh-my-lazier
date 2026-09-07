# Worker operation

The example config is [config/example.yaml](../../config/example.yaml). The `worker` and
`txretry` commands default to `config.yaml` in the current working directory;
use `-config <worker.yaml>` to select another file. Start the default local stack with:

```bash
docker compose up
```

Run the same on-chain config gate used at worker startup:

```bash
go run ./go/cmd/configcheck -config config/example.yaml
```

`DATABASE_URL` may override the configured database URL at runtime. Other config is loaded once at startup; runtime config changes require a process restart.

Useful operator commands:

```bash
go run ./go/cmd/worker -config <worker.yaml> -log-level debug -indexer-progress-log-interval 1m
go run ./go/cmd/configdiff -from <approved.yaml> -to <proposed.yaml>
go run ./go/cmd/readinesscheck -config <worker.yaml> -format json
go run ./go/cmd/pricebot-once -config <worker.yaml> -log-level debug
go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst> -format json
go run ./go/cmd/txretry -config <worker.yaml> -action inspect|retry-failed|replace|cancel-nonce|resolve-external-nonce [-resolution retry|abandon] -id <tx_outbox_id>
go run ./go/cmd/txretry -config <worker.yaml> -action rebroadcast -id <tx_outbox_id> -rpc-url <rpc_url>
```

Worker binaries default to `-log-level info`. Use `-log-level debug` when investigating normal skip/defer reasons such as indexer caught-up windows, disabled pathways, not-yet-confirmed DVN jobs, or deferred tx manager work.
The long-running worker also defaults to `-indexer-progress-log-interval 1m`, which limits indexer progress `Info` logs to one line per chain and stream per interval; set it to `0` to disable periodic progress `Info` logs and rely on `/metrics` plus debug logs.
Use `go run ./go/cmd/worker -config <worker.yaml> -skip-onchain-check` only as a long-running worker startup bypass for the on-chain config check. It does not skip local YAML/schema validation, and it does not affect `configcheck` or `pricebot-once`. Pricing chains configured with on-chain sources (Chainlink, Uniswap) still establish their RPC head quorum before source validation, and every remaining configured chain establishes it before the durable loops start, so the bypass never lets a single provider serve unverified startup reads.

## Transaction recovery

`txretry -action rebroadcast -rpc-url <rpc_url>` immediately replays the current persisted signed transaction through an explicitly selected RPC. It validates the RPC and transaction chain IDs, signature, sender, nonce and hash; it does not load a signer or change fees. Each invocation authorizes one manual send beyond automatic replay limits without resetting the cumulative count. An accepted result acknowledges RPC acceptance only; the worker still establishes canonical receipt confirmation. See the recovery runbook for eligible states and concurrency guards.

Accepted-but-missing transactions use bounded same-raw recovery before replacement; independent recovery clocks, signer-level alerts and `txretry inspect` expose silent fee/budget waits. See [durable transaction recovery](monitoring.md#durable-transaction-recovery) for timing, operator actions and alert receiver setup. `make check-alerts` runs pinned Prometheus rule tests using Docker and is part of `make check`.

See [runtime behavior](../runtime.md) for quorum, indexing, pricing, and transaction
state guarantees.
