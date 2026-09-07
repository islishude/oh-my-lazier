# Phase 1 scope

Phase 1 is EVM-only.

DVN independence is organizational, relative to the OApp and its configuration
owner: a separate legal and control entity with its own keys, infrastructure, RPC
providers, and build/deploy pipeline. A reciprocal counterparty-operated DVN
qualifies. Maintained deployment scope docs must record the concrete operator
topology and verification evidence.

Update maintained scope docs before adding non-EVM support, `composeMsg`,
`lzCompose`, native drop, ordered execution, self-only DVN, hot config reload,
or live testnet/mainnet execution.

- Worker chain configs must declare `family: evm`.
- Required DVNs are the configured `OpenDVN` plus at least one independent
  external DVN. LayerZero Labs DVN is an optional external DVN choice, not a
  required provider; deployment profiles can opt into the repo-known
  Sepolia/Hoodi address with `chains[].includeLayerZeroLabsDVN`.
- Basic OFT send is supported.
- `composeMsg`, `lzCompose`, native drop, ordered execution, self-only DVN, and non-EVM chains are out of scope.
- Executor options must contain exactly one zero-value executor `lzReceive` option.
- `OpenDVN` rejects non-empty DVN options.
- Shared price snapshots must be fresh.

`OpenExecutor` remains compatible with the pinned nonpayable `ILayerZeroExecutor.assignJob` interface. `OpenExecutor` and `OpenDVN` quote and emit assignment price information while pinned `SendUln302` records returned worker fees in its own ledger. Worker owners withdraw those recorded fees through the worker `withdrawFee(sendLib, recipient, amount)` passthrough for allowed send libraries.

See the [documentation index](README.md#deployment-and-release) for deployment
policies and release gates.
