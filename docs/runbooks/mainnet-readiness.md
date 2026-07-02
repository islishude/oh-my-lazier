# Mainnet Readiness Runbook

This runbook is the final review index before any mainnet deployment proposal. Phase 1 remains EVM-only and must not use self-only DVN.

## Required Inputs

- Validated worker config for the target environment.
- `configdiff` output from the last approved config to the proposed config.
- `configcheck` output proving the proposed worker config matches live chain state.
- LayerZero address refresh output from `npm run check:lz-addresses`.
- DB-backed readiness output from `go run ./go/cmd/readinesscheck -config <worker.yaml> -format json`.
- `inspect:lz-config` output for every configured direction.
- Signer inventory and key-management review.
- Price bot runbook evidence for the latest OpenExecutor/OpenDVN price config update.
- Rate-limit and pause review for every OFT pathway.
- Monitoring dashboard and alert proof.
- Runbook review output from `npm run check:runbooks`.
- Security review report with no open critical findings.

## Non-Negotiable Phase 1 Scope

- Ethereum Sepolia <-> Base Sepolia rehearsal must complete before mainnet.
- No `composeMsg`.
- No `lzCompose`.
- No native drop.
- No ordered execution.
- No non-EVM chain support.
- Worker chain configs must declare `family: evm`.
- No self-only DVN.
- Required DVNs must include both OpenDVN and an independent LayerZero Labs DVN.
- Confirmations must be 12 unless the maintained scope documentation is explicitly updated.

## Review Sequence

1. Run `make check`.
2. Run `go run ./go/cmd/configdiff -from <approved.yaml> -to <proposed.yaml>`.
3. Run `go run ./go/cmd/configcheck -config <proposed.yaml> -format json`.
4. Run `npm run inspect:lz-config` for each configured direction and archive output.
5. Complete `docs/runbooks/key-management.md`.
6. Complete `docs/runbooks/price-bot.md`.
7. Complete `docs/runbooks/rate-limit.md`.
8. Confirm `docs/runbooks/monitoring.md` dashboard and alerts are active.
9. Run `npm run check:runbooks` and archive output.
10. Run `go run ./go/cmd/readinesscheck -config <worker.yaml> -format json` and archive output.
11. Complete security review and resolve all critical findings.
12. Confirm rollback steps for Executor and DVN config are documented with previous config values.
13. Confirm canary transfer amount, sender account, recipient account, minimum recipient balance, signer, owner, and operator contacts.
14. Run `MIGRATION_EVIDENCE=<record.json> npm run check:migration-evidence`.
15. Approve the migration ticket only after every artifact above is attached.

## Go / Worker Checks

Required commands:

```bash
go test ./...
go test ./go/internal/signer/keystore ./go/internal/signer/kms -count=1
go test ./go/internal/config ./go/internal/configdiff ./go/cmd/configdiff -count=1
go test ./go/internal/configcheck ./go/cmd/configcheck -count=1
go test ./go/internal/metrics ./go/internal/db ./go/internal/app -count=1
go test ./go/internal/readiness ./go/cmd/readinesscheck -count=1
npm run check:runbooks
make security-check
```

Required runtime checks:

- `/healthz` returns `200`.
- `/readyz` returns `200`.
- `/metrics` exposes chain pause, pathway pause, packet, executor, DVN, tx outbox, indexer cursor, and indexer polling metrics.
- No chain or pathway is paused before the migration begins.
- No active-chain tx outbox row is stuck in `failed` with `retry_state="exhausted"`.
- No DVN job is stuck in `READY_TO_VERIFY` or `VERIFY_TX_ENQUEUED` beyond the expected tx manager polling and confirmation window.
- No executor job is stuck in `WAITING_DVN_VERIFICATION`, `VERIFIABLE`, `COMMIT_TX_ENQUEUED`, `COMMITTED`, `EXECUTABLE`, or `LZ_RECEIVE_TX_ENQUEUED` beyond the expected source/destination confirmation window.
- Every enabled pathway has advanced `executor_source` and `executor_destination` indexer cursors on the relevant active chains.
- `go run ./go/cmd/readinesscheck -config <worker.yaml>` exits successfully.

## Contract / LayerZero Checks

Required commands:

```bash
npm run typecheck
npx hardhat test solidity
npm run inspect:lz-config
```

Required state:

- OFT peers are configured in both directions.
- OpenExecutor and OpenDVN allow only intended SendLib addresses.
- Worker pathway config is enabled only for approved OApps.
- Endpoint executor config points to each pathway's configured `source_workers.open_executor` only after the executor migration step.
- ULN config includes each pathway's configured `source_workers.open_dvn` plus an independent LayerZero Labs DVN only during the DVN join step.
- Optional DVNs are explicitly disabled for the first-phase required-DVN migration.
- OpenExecutor and OpenDVN `priceConfig(dstEid)` values are fresh and match the approved price bot evidence.

## Rollback Approval

The rollback section of the migration ticket must include:

- previous Executor config
- previous send ULN config
- previous receive ULN config
- `DRY_RUN=1 npm run configure:lz-rollback` output showing the exact rollback `setConfig` batches
- restored Executor/ULN config check after rollback
- canary transfer evidence after rollback
- owner account able to pause/unpause TestOFT
- signer account able to submit worker transactions
- `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst> -format json` output for the affected pathway
- manual retry plan for verified but undelivered packets when `verified_but_undelivered_count` is non-zero, using txmgr automatic retry first and `go run ./go/cmd/txretry -config <worker.yaml> -action retry-failed|replace -id <tx_outbox_id>` only after automatic retry is exhausted or an operator override is approved. `replace` keeps the nonce, re-reads the latest RPC header/gas suggestions, and signs only when the current fee is at least 10% above the previous signed fee without exceeding the configured cap. `retry-failed` preserves any failed row that already consumed a nonce and returns a cloned queued outbox row in the command JSON; use the returned `after.id` for tracking the fresh retry.

## Rejection Criteria

Reject mainnet readiness if:

- any critical security finding remains open
- self-only DVN is proposed
- required DVNs do not include an independent LayerZero Labs DVN
- confirmations are not 12 without an explicit scope documentation update
- signer inventory or rollback signer is missing
- price config evidence is missing, stale, or not produced through the approved pricing path
- rate-limit capacity/refill is not documented per pathway
- monitoring alerts are not active
- `make security-check` fails
- config diff output is missing or unreviewed
- canary amount, sender, recipient, receipt, or recipient balance evidence is missing
- `npm run check:migration-evidence` fails for the migration ticket record
- `go run ./go/cmd/readinesscheck -config <worker.yaml>` reports any issue
