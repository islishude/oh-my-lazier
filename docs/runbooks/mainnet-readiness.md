# Mainnet Readiness Runbook

This runbook is the final review index before any mainnet deployment proposal. Phase 1 remains EVM-only and must not use self-only DVN.

## Required Inputs

- Validated worker config for the target environment.
- `configdiff` output from the last approved config to the proposed config.
- LayerZero address refresh output from `npm run check:lz-addresses`.
- DB-backed readiness output from `go run ./go/cmd/readinesscheck -config <worker.yaml> -format json`.
- `inspect:lz-config` output for every configured direction.
- Signer inventory and key-management review.
- Price bot runbook evidence for the latest OpenExecutor/OpenDVN price config update.
- Rate-limit and pause review for every OFT pathway.
- Monitoring dashboard and alert proof.
- Security review report with no open critical findings.

## Non-Negotiable Phase 1 Scope

- Ethereum Sepolia <-> Base Sepolia rehearsal must complete before mainnet.
- No `composeMsg`.
- No `lzCompose`.
- No native drop.
- No ordered execution.
- No non-EVM chain support.
- No self-only DVN.
- Required DVNs must include both OpenDVN and an independent LayerZero Labs DVN.
- Confirmations must be 12 unless the top-level plan is explicitly updated.

## Review Sequence

1. Run `make check`.
2. Run `go run ./go/cmd/configdiff -from <approved.yaml> -to <proposed.yaml>`.
3. Run `npm run inspect:lz-config` for each configured direction and archive output.
4. Complete `docs/runbooks/key-management.md`.
5. Complete `docs/runbooks/price-bot.md`.
6. Complete `docs/runbooks/rate-limit.md`.
7. Confirm `docs/runbooks/monitoring.md` dashboard and alerts are active.
8. Run `go run ./go/cmd/readinesscheck -config <worker.yaml> -format json` and archive output.
9. Complete security review and resolve all critical findings.
10. Confirm rollback steps for Executor and DVN config are documented with previous config values.
11. Confirm canary transfer amount, sender account, recipient account, minimum recipient balance, signer, owner, and operator contacts.
12. Run `MIGRATION_EVIDENCE=<record.json> npm run check:migration-evidence`.
13. Approve the migration ticket only after every artifact above is attached.

## Go / Worker Checks

Required commands:

```bash
go test ./...
go test ./go/internal/signer/keystore ./go/internal/signer/kms -count=1
go test ./go/internal/config ./go/internal/configdiff ./go/cmd/configdiff -count=1
go test ./go/internal/metrics ./go/internal/db ./go/internal/app -count=1
go test ./go/internal/readiness ./go/cmd/readinesscheck -count=1
make security-check
```

Required runtime checks:

- `/healthz` returns `200`.
- `/readyz` returns `200`.
- `/metrics` exposes chain pause, pathway pause, packet, executor, DVN, tx outbox, and indexer cursor metrics.
- No chain or pathway is paused before the migration begins.
- No tx outbox row is stuck in `failed` for active chains.
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
- `ExecutorConfig.executor` points to OpenExecutor only after the executor migration step.
- ULN config has required DVNs `[OpenDVN, LayerZero Labs DVN]` only during the DVN join step.
- Optional DVNs are explicitly disabled for the first-phase required-DVN migration.
- OpenExecutor and OpenDVN `priceConfig(dstEid)` values are fresh and match the approved price bot evidence.

## Rollback Approval

The rollback section of the migration ticket must include:

- previous Executor config
- previous send ULN config
- previous receive ULN config
- restored Executor/ULN config check after rollback
- canary transfer evidence after rollback
- owner account able to pause/unpause TestOFT
- signer account able to submit worker transactions
- `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst> -format json` output for the affected pathway
- manual retry plan for verified but undelivered packets when `verified_but_undelivered_count` is non-zero, using `go run ./go/cmd/txretry -config <worker.yaml> -action retry-failed|replace -id <tx_outbox_id>` for the selected outbox row

## Rejection Criteria

Reject mainnet readiness if:

- any critical security finding remains open
- self-only DVN is proposed
- required DVNs do not include an independent LayerZero Labs DVN
- confirmations are not 12 without an explicit plan update
- signer inventory or rollback signer is missing
- price config evidence is missing, stale, or not produced through the approved pricing path
- rate-limit capacity/refill is not documented per pathway
- monitoring alerts are not active
- `make security-check` fails
- config diff output is missing or unreviewed
- canary amount, sender, recipient, receipt, or recipient balance evidence is missing
- `npm run check:migration-evidence` fails for the migration ticket record
- `go run ./go/cmd/readinesscheck -config <worker.yaml>` reports any issue
