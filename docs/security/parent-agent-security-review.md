# Parent-Agent Security Review

Date: 2026-06-26

Scope:

- Solidity contracts under `contracts/contracts`
- Go worker code under `go`
- TypeScript deployment/config scripts under `contracts/scripts`
- package and module dependency metadata
- runbooks and config examples where they affect operational security

This is a parent-agent local review, not a completed exhaustive Codex Security multi-agent scan. Codex Security preflight returned `incomplete` because the active multi-agent runtime mode and usable worker slot count could not be confirmed. Treat this document as M9 security-review progress, not final mainnet approval.

## Commands

```bash
python3 /Users/sudoless/.codex/plugins/cache/openai-curated/codex-security/3fdeeb49/scripts/config_preflight.py --profile security_scan --cwd /Users/sudoless/codespace/coding/oh-my-lazier --runtime-check delegation_available=true --runtime-check goal_tools_available=true --available-plugin-skill security-scan --available-plugin-skill threat-model --available-plugin-skill finding-discovery --available-plugin-skill validation --available-plugin-skill attack-path-analysis
rg "(?i)(private[_-]?key|secret|password|signature|keystore|kms|api[_-]?key)" -n go contracts/scripts config docs --glob '!contracts/artifacts/**' --glob '!node_modules/**'
rg "(?i)(logger\\.|slog\\.|fmt\\.Print|log\\.|console\\.log)" -n go contracts/scripts --glob '!contracts/artifacts/**' --glob '!node_modules/**'
rg "(?i)(delegatecall|selfdestruct|tx\\.origin|assembly|unchecked|\\.call\\{|withdraw|onlyOwner|setAllowedSendLib|setPaused)" contracts/contracts contracts/test -n --glob '!contracts/artifacts/**'
npm audit --audit-level=moderate --json
tmpbin=$(mktemp -d) && GOBIN="$tmpbin" go install golang.org/x/vuln/cmd/govulncheck@latest && "$tmpbin/govulncheck" ./...
```

## Summary

Open critical issue:

- `npm audit` reports 3 critical and 19 high vulnerabilities in transitive JavaScript toolchain dependencies, including `elliptic`, `ethers`, and `ws` paths pulled through Hardhat/LayerZero-related packages. This blocks claiming M9 `no open critical issues`.

No critical code-level issue was confirmed in this local review:

- No committed private key or raw secret was found in repository source/config examples.
- Worker logs reviewed in `go` do not print private key material, decrypted keystore JSON, KMS signatures, or raw secret values.
- Solidity contract review did not find `delegatecall`, `selfdestruct`, `tx.origin`, inline assembly, or unchecked arithmetic in project contracts.
- Native-token withdrawal uses an external call but is restricted by `onlyOwner` in `WorkerAccess`.
- SQL query construction is static except `db.statusStats`, which allowlists table names before `fmt.Sprintf`.
- `govulncheck ./...` found 0 vulnerabilities affecting called Go code.

## Finding S-001: Open npm audit critical/high advisories in toolchain dependencies

Severity: High for release readiness; open blocker for M9 acceptance.

Evidence:

- `npm audit --audit-level=moderate --json` returned non-zero.
- Metadata summary: 38 total vulnerabilities, including 3 critical and 19 high.
- Critical paths include transitive `elliptic` and `ethers` advisories through `@ethersproject/*` / `zksync-ethers` dependency chains.
- High paths include `ws`, `viem`, Hardhat toolbox related packages, and LayerZero packages as reported by npm audit.

Assessment:

- These dependencies are currently used by the contract build/test/deployment toolchain, not by the Go worker runtime.
- They still matter for deployment and migration safety because deployment scripts consume private keys and LayerZero config scripts run against live networks.
- The audit-suggested fixes include semver-major or package downgrades for LayerZero packages and are not safe to apply automatically because the project plan pins LayerZero packages and requires interface compatibility with current pinned packages.
- A local remediation attempt with npm `overrides` for independent transitive packages such as `ws`, `undici`, `lodash-es`, and `axios` did not close the critical advisory chain. The critical findings remained rooted in pinned LayerZero-package transitive `ethers@5` / `elliptic` paths, and one override combination increased the critical count, so those changes were not retained.

Required closure before M9 completion:

- Decide whether to update the Hardhat/LayerZero toolchain, isolate deployment scripts into a locked ephemeral environment, or accept documented testnet-only exposure.
- Re-run `npm audit`.
- Record a reviewed disposition for every remaining critical advisory before mainnet readiness approval.

## Reviewed Controls

Signer/key boundary:

- KMS signer validates `ECC_SECG_P256K1`.
- KMS signatures are parsed from DER, low-S normalized, and recovered against the configured Ethereum address.
- Keystore passwords can come from env or file and missing/empty sources are rejected.
- No signer implementation logs raw key material.

Contract authority:

- `OpenExecutor`, `OpenDVN`, `WorkerAccess`, and OFT controls use owner-gated configuration.
- SendLib allowlists are explicit.
- Worker assignment can be paused.
- OFT sends/receives can be paused per endpoint direction.

RPC quorum and safety:

- Head hash conflicts pause chains.
- Receipt/log conflicts pause pathways.
- Pause state is exposed through `/metrics`.

Operational readiness:

- Monitoring, config diff, key management, rate-limit, and mainnet readiness runbooks exist.
- Mainnet readiness still requires a full security review and explicit closure of S-001.

## Non-Exhaustive Review Limitations

- No multi-agent exhaustive file/shard coverage ledger was produced.
- No per-candidate discovery/validation/attack-path ledgers were produced.
- No formal final Codex Security report was sealed.
- No testnet live deployment or canary transfer evidence was available.

## Required Next Step

Complete an exhaustive Codex Security scan or equivalent human security review, and close or formally accept S-001 before marking M9 security review complete.
