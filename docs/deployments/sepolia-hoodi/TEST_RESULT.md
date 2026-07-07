```sh
MIGRATION_EVIDENCE=docs/deployments/sepolia-hoodi/migration-evidence.json npm run check:migration-evidence
```

```
> oh-my-lazier@1.0.0 check:migration-evidence
> tsx contracts/scripts/migration-evidence.ts

{
  "ok": true,
  "evidenceType": "deployment",
  "environment": "testnet",
  "directions": 2
}
```
