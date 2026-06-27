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

- `npm audit` now reports 0 critical, 6 high, and 4 moderate vulnerabilities after retaining `@nomicfoundation/hardhat-toolbox-viem` and applying `overrides` for independent transitive packages (`axios`, `elliptic`, `undici`, and `ws`). The remaining advisories are still open and block final M9 approval.

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
- Original metadata summary: 38 total vulnerabilities, including 3 critical and 19 high.
- Current metadata summary after remediation: 31 total vulnerabilities, including 0 critical, 6 high, and 4 moderate.
- The cleared critical paths included transitive `elliptic` and `ethers` advisories through `@ethersproject/*` / `zksync-ethers` dependency chains.
- The remaining high paths are reported through pinned LayerZero 3.0.168 transitive dependencies and retained Hardhat toolbox transitive dependencies, including `@chainlink/contracts-ccip`, old OpenZeppelin contract packages, `@layerzerolabs/lz-evm-messagelib-v2`, `@layerzerolabs/lz-evm-oapp-v2`, and `lodash-es` via Hardhat Ignition.

Assessment:

- These dependencies are currently used by the contract build/test/deployment toolchain, not by the Go worker runtime.
- They still matter for deployment and migration safety because deployment scripts consume private keys and LayerZero config scripts run against live networks.
- The audit-suggested fixes still include semver-major package changes or a LayerZero package downgrade to `2.0.6`; that is not safe to apply automatically because the project plan pins current LayerZero packages and requires interface compatibility with those pinned packages.
- Local remediation retained only changes that preserved current compile/test behavior and kept the Hardhat toolbox dependency: `viem` remains a direct dependency for scripts, and `axios`, `elliptic`, `undici`, and `ws` are overridden to fixed versions.

Required closure before M9 completion:

- Decide whether to update the Hardhat/LayerZero toolchain, isolate deployment scripts into a locked ephemeral environment, or accept documented testnet-only exposure.
- Re-run `npm audit`.
- Record a reviewed disposition for every remaining high and moderate advisory before mainnet readiness approval.

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
