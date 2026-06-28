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
- Confirm chain `eid`, `chain_id`, endpoint, worker contract, and RPC changes are intentional.
- Confirm pathway `src_eid`, `dst_eid`, OApp, SendLib, ReceiveLib, enablement, and max message size changes are intentional.
- Confirm signer changes are expected and do not point to unapproved keys.
- Confirm pricing source, stale threshold, gas spike threshold, gas limit, and fee cap changes are expected.
- For DVN migration, confirm the proposed config still uses `dvn.mode: shadow` until the explicit active-mode change is approved.
- Keep the text output in the migration ticket and the JSON output as an immutable review artifact.

The command compares chains by `eid`, pricing chains by `eid`, and pathways by `(src_eid, dst_eid, src_oapp, dst_oapp)` so list reordering does not create review noise.
