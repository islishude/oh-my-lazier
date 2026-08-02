# Rate-Limit and Pause Review

This review covers `TestOFT` pause controls and outbound token-bucket limits used during testnet migration, DVN join, rollback, and future mainnet readiness.

## Contract Controls

`OFTPauseAndRateLimit` exposes:

- `pauseSend(uint32 dstEid, bool paused)`: blocks outbound burns for one destination endpoint.
- `pauseReceive(uint32 srcEid, bool paused)`: blocks inbound mints from one source endpoint.
- `setOutboundRateLimit(uint32 dstEid, RateLimitConfig config)`: configures outbound token-bucket capacity and refill rate.
- `clearOutboundRateLimit(uint32 dstEid)`: removes the configured outbound token-bucket limit and returns the pathway to unrestricted send capacity.

Operational properties:

- unset outbound rate limits are unrestricted
- setting `capacity = 0` and `refillPerSecond = 0` is explicit drain mode
- clearing a rate limit deletes both config and bucket state
- setting a rate limit resets current bucket tokens to `capacity`
- refill is capped at `capacity`, so idle periods cannot accumulate more than one bucket
- send pause is checked before token debit
- receive pause is checked before mint

Implementation evidence:

- `contracts/contracts/oft/OFTPauseAndRateLimit.sol`
- `contracts/test/OpenWorkers.t.sol` pause and rate-limit tests
- `npm run configure:workers` with the optional `input.rateLimit` object
- `npm run oft:pathway` with strict `input.action` and optional
  `input.rateLimit` for inspecting and changing TestOFT pathway
  pause/drain/rate-limit/clear state
- `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>` for confirming worker DB state has no in-flight packet, job, or outbox work before a config switch

Every `oft:pathway` invocation uses `OML_SCRIPT_PARAMS` and an explicit
single-chain Hardhat `--network`. `inspect` is read-only. The write actions
`pause-send`, `unpause-send`, `pause-receive`, `unpause-receive`, `drain`,
`clear-rate-limit`, and `set-rate-limit` require a top-level `apply` flag. The
`set-rate-limit` action alone requires
`input.rateLimit.{capacity,refillPerSecond}`, both as decimal strings.

Example reviewed drain plan:

```json
{
  "input": {
    "action": "drain",
    "testOFT": "0x1111111111111111111111111111111111111111",
    "remoteEid": "40449",
    "expectedSigner": "0x2222222222222222222222222222222222222222"
  },
  "apply": false,
  "confirmation": "interactive"
}
```

```bash
OML_SCRIPT_PARAMS=tmp/oft-pathway-drain.json \
  npm run oft:pathway -- --network sepolia
```

## Migration Usage

Before switching Executor or DVN config:

1. Set a conservative outbound rate limit for the pathway.
2. Run canary transfers and confirm `ExecutorFeePaid`, commit, delivery, and destination balance.
3. For drain, archive an `oft:pathway` envelope with `input.action: "drain"` or `input.action: "pause-send"` and `apply: false`; after approval, rerun the same explicit network with `apply: true`.
4. Run `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst>` until it reports `ready: true` before changing DVN required sets.
5. After validation, restore the approved rate limit with `input.action: "set-rate-limit"`, or remove a temporary limiter with `input.action: "clear-rate-limit"`, then unpause sends with `input.action: "unpause-send"`. Each state change requires its own reviewed envelope and explicit `apply` value.

`pauseSend` is preferred when the operator wants an immediate hard stop. Zero-capacity rate limit is preferred when documenting an explicit drain configuration alongside other pathway setup.

## Review Checklist

- Confirm every pathway has a documented steady-state capacity and refill rate.
- Confirm the canary amount is below steady-state capacity.
- Confirm drain mode uses both capacity and refill set to zero.
- Confirm temporary rate limits are cleared only when unrestricted send capacity is the approved steady state.
- Confirm no migration relies on inbound `pauseReceive` unless rollback explicitly requires blocking destination mint.
- Confirm `configdiff` captures any worker-side pathway changes before the on-chain rate-limit operation.
- Confirm monitoring includes `laz_packets_total`, `laz_executor_jobs_total`, and `laz_pathway_paused` during drain.
- Confirm the owner account for `TestOFT` is available before starting a pause/drain operation.

## Rollback

1. Pause outbound sends for the affected destination endpoint.
2. Inspect in-flight packets and tx outbox state with `go run ./go/cmd/draincheck -config <worker.yaml> -src-eid <src> -dst-eid <dst> -format json`.
3. Restore the previous LayerZero Executor or DVN config.
4. Send a canary transfer.
5. Unpause sends only after delivery and balance checks pass.

## Rejection Criteria

Do not approve mainnet readiness if:

- steady-state limits are not documented per pathway
- canary size exceeds capacity
- owner access for pause/unpause is not verified
- drain and rollback steps are missing from the migration ticket
- rate-limit changes are not paired with monitoring and config-diff artifacts
