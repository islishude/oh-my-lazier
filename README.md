# oh-my-lazier

Self-hosted LayerZero V2 Executor and DVN worker stack, with Solidity contracts,
TypeScript deployment and validation scripts, and Go worker services backed by
Postgres.

Phase 1 supports EVM chains and basic OFT sends. Required DVNs are `OpenDVN` plus
at least one independently operated DVN. See [scope](docs/scope.md) for supported
options and operator independence requirements.

## Quick start

Requires Node.js 26+, Go 1.26+, Docker, Foundry `forge`, and `golangci-lint`.

```bash
npm install
make check
```

Start the default local stack with `docker compose up`. See the
[example config](config/example.yaml) and [worker guide](docs/runbooks/worker.md)
for configuration, startup checks, and operator commands.

## Repository gates

```bash
make check            # compile, typecheck, ABI drift, tests, docs, alerts, lint, formatting
make test-integration # Postgres + Rustack KMS integration and recovery race tests
make security-check   # security review, npm audit disposition, Go vulnerabilities
make docker-smoke     # worker image and entrypoint
make e2e-local        # local dual-Anvil end-to-end flow
make e2e-ci           # CI E2E with prestarted services and a prebuilt worker image
```

See [development](docs/development.md) for focused commands, shared-database test
rules, existing integration dependencies, and ABI generation.

## Documentation

- [Documentation index](docs/README.md): all development, operations, deployment, and security guides.
- [Runtime behavior](docs/runtime.md): quorum, indexing, pricing, and durable transactions.
- [Contract scripts](contracts/scripts/README.md): deployment, configuration, canary, and rollback.
- [Mainnet readiness](docs/runbooks/mainnet-readiness.md): release review sequence.
- [Agent instructions](AGENTS.md): repository-wide constraints and required reading.
