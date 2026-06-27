# Key Management Review

This review covers the first-phase worker signer boundary:

- AWS KMS `ECC_SECG_P256K1`
- local geth keystore JSON

## Required Signer Inventory

For each deployment environment, record:

- signer address
- signer backend: `kms` or `keystore`
- allowed chains and purposes: Executor, DVN, price bot
- key owner and emergency contact
- rotation procedure and rollback signer
- funding policy for native gas

The configured signer address must match the expected address in worker config and migration tickets. Never infer approval from a successful transaction alone.

Worker config keeps a top-level `signers` inventory. `executor.signer` and `pricing.signer` must reference one of those signer IDs, and signer IDs are the Ethereum addresses used as `tx_outbox.signer_id`. Keystore entries may reference only a password environment variable or password file. KMS entries may reference only credential environment variable names, never raw access keys.

## AWS KMS Requirements

The KMS key must be asymmetric `ECC_SECG_P256K1`.

Required controls:

- key policy limits signing access to the worker role and break-glass operators
- CloudTrail or equivalent audit logging is enabled for `kms:Sign`
- deletion is disabled or has a long pending window
- key rotation plan uses a staged worker config change and `configdiff`
- signer address recovery is validated before first use

Implementation evidence:

- `go/internal/config.Config.Validate` rejects unknown executor/pricing signer references.
- `go/internal/app.App.txTargets` loads configured signers and creates one tx manager target per required signer per chain.
- `go/internal/signer/kms.Signer.ValidateKey` rejects non-`ECC_SECG_P256K1` keys.
- `go/internal/signer/kms.Signer.SignHash` parses DER signatures, normalizes low-S, and recovers the configured Ethereum address.
- `go/internal/signer/kms.Signer.SignTx` signs EIP-1559 transactions through the same address-recovery boundary.

## Local Keystore Requirements

Local geth keystore signers are acceptable for testnet and emergency dry runs only unless mainnet approval explicitly allows them.

Required controls:

- keystore JSON is stored outside the repository
- password comes from an environment variable or password file, not config YAML
- password file permissions are restricted to the worker user
- encrypted keystore content and passwords are never logged
- Never log private key material, decrypted keystore content, KMS signatures, or raw secrets.
- keystore signer address is recorded in the signer inventory

Implementation evidence:

- `go/internal/signer/keystore.LoadWithPasswordSource` supports password value, environment variable, or file.
- `go/internal/signer/keystore.ResolvePassword` rejects missing or empty password sources.
- `go/internal/signer/keystore.Signer.SignTx` signs with geth's chain-aware latest signer.

## Pre-Migration Checklist

- Run `go test ./go/internal/signer/keystore ./go/internal/signer/kms -count=1`.
- Run `make test-integration` when Docker is available; it starts dedicated Postgres and Rustack containers, runs the DB/tx manager integration tests and the Rustack KMS signing test, and removes the temporary database files on exit.
- When an AWS-compatible KMS mock is available, run `RUSTACK_KMS_ENDPOINT=<endpoint> make test-kms-rustack`. The target requires `RUSTACK_KMS_ENDPOINT` and runs only the Rustack KMS transaction signing integration test.
- Run `go run ./go/cmd/configdiff -from <current.yaml> -to <proposed.yaml>` and confirm signer changes are expected.
- Confirm worker logs do not include private key material, decrypted keystore JSON, KMS signatures, or raw secrets.
- Confirm each configured signer has native gas on its assigned chains.
- Confirm break-glass operators can pause OFT sends and worker assignments without needing private key material from the worker host.

## Rotation Procedure

1. Add the new signer to the inventory.
2. Validate KMS key spec or keystore decrypt/sign behavior in a non-production environment.
3. Prepare proposed config with the new signer address.
4. Run `configdiff` and attach output to the change ticket.
5. Fund the new signer for gas.
6. Stop the worker, deploy the config, and restart.
7. Confirm `/readyz`, `/metrics`, and tx outbox progress.
8. Keep the old signer funded until queued/broadcast transactions are confirmed or explicitly abandoned.

## Rejection Criteria

Do not approve mainnet readiness if:

- the signer address is unknown or not in inventory
- a KMS key is not `ECC_SECG_P256K1`
- signer changes are not present in a reviewed `configdiff`
- a local keystore password is committed, printed, or stored in worker YAML
- the rollback signer is unfunded or inaccessible
