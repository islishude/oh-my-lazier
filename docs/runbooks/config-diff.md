# Config Diff Runbook

Use `go run ./go/cmd/configdiff` before testnet migration changes, DVN joins, rollback rehearsals, and any mainnet config proposal.

The command loads both YAML files with validation and defaults, but without environment overrides. This prevents `DATABASE_URL` or deployment environment variables from hiding file-level drift during review.

Text review:

```bash
go run ./go/cmd/configdiff \
  -from config/current.yaml \
  -to config/proposed.yaml
```

JSON artifact:

```bash
go run ./go/cmd/configdiff \
  -from config/current.yaml \
  -to config/proposed.yaml \
  -format json > config-diff.json
```

CI guard:

```bash
go run ./go/cmd/configdiff \
  -from config/current.yaml \
  -to config/proposed.yaml \
  -fail-on-diff
```

Review checklist:

- Confirm both configs validate successfully.
- Confirm chain `eid`, `chain_id`, endpoint, transaction roles, and RPC changes are intentional.
- Confirm pathway `src_eid`, `dst_eid`, OApp, SendLib, ReceiveLib, source worker contracts, DVN mode, enablement, and max message size changes are intentional.
- Confirm pathway `min_lz_receive_gas` and `max_lz_receive_gas` changes match the OpenExecutor/OpenDVN on-chain pathway settings.
- Confirm signer changes are expected and do not point to unapproved keys.
- Confirm pricing source, stale threshold, gas spike threshold, gas limit, and fee cap changes are expected.
- For DVN migration, confirm each proposed pathway still uses `pathways[].dvn.mode: shadow` until the explicit active-mode change is approved.
- Keep the text output in the migration ticket and the JSON output as an immutable review artifact.

After diff review, run the on-chain config check before worker restart or price
updates:

```bash
go run ./go/cmd/configcheck -config config/proposed.yaml
```

The check compares the YAML with live chain state, including chain ID, Endpoint
EID, deployed code, OApp peers, send/receive libraries, ULN required DVNs, and
the configured `pathways[].source_workers` pathway configuration. Every
configured RPC URL must return the configured `chain_id` from `eth_chainId`.
Worker startup and `price-once` run the same check before database sync.

The command compares chains by `eid`, pricing chains by `eid`, and pathways by `(src_eid, dst_eid, src_oapp, dst_oapp)` so list reordering does not create review noise.
