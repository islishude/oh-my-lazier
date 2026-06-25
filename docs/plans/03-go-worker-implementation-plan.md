# 03 - Go Worker Implementation Plan

## 目标

实现单个 Go worker 进程，同时处理 Ethereum Sepolia 与 Base Sepolia。

进程包含：

- source chain indexing
- target chain indexing
- Executor Committer
- Executor Deliverer
- DVN shadow verifier
- tx manager
- price bot
- metrics endpoint

## Layout

```text
go/
  cmd/
    worker/
      main.go

  internal/
    app/
    config/
    chain/
    db/
    indexer/
    lz/
    lzabi/
    packets/
    rpcquorum/
    executor/
    dvn/
    txmgr/
    signer/
      kms/
      keystore/
    pricing/
    metrics/
    logging/
```

## Main Process

`cmd/worker/main.go` 启动：

- config loader
- db connection
- chain registry
- RPC quorum clients
- metrics HTTP server
- source indexers
- target indexers
- tx manager loop
- executor committer loop
- executor deliverer loop
- dvn shadow verifier loop
- price bot loop

配置启动时加载，变更后重启生效。

## Executor Flow

### Source Events

监听 source chain：

- `PacketSent`
- `ExecutorFeePaid`
- `OpenExecutor.ExecutorJobAssigned`

### Target Events

监听 target chain：

- `PayloadVerified`
- packet committed / verified events
- packet received events
- `LzReceiveAlert`

### State Machine

```text
NEW
ASSIGNED
WAITING_DVN_VERIFICATION
VERIFIABLE
COMMIT_TX_ENQUEUED
COMMITTED
EXECUTABLE
LZ_RECEIVE_TX_ENQUEUED
DELIVERED
LZ_RECEIVE_FAILED
MANUAL_REVIEW
```

### Committer

职责：

- 找到 assigned jobs
- 检查 `ULN.verifiable(packetHeader, payloadHash)`
- 如果状态是 `Verifiable`，enqueue `commitVerification`
- 如果状态已经是 `Verified`，标记 committed

### Deliverer

职责：

- 检查 `endpoint.executable(packetHeader, payloadHash)`
- 如果状态是 `Executable`，构造 `endpoint.lzReceive(...)`
- 通过 `tx_outbox` 提交交易
- receipt 成功后标记 delivered
- 监听 `LzReceiveAlert` 并标记 failed

## DVN Flow

### Source Events

监听 source chain：

- `PacketSent`
- `DVNFeePaid`
- `OpenDVN.DVNJobAssigned`

DVN worker 必须使用 `DVNFeePaid` 确认 OpenDVN 被分配，再将 packet 作为 DVN job 处理。

### State Machine

```text
NEW
ASSIGNED
WAITING_CONFIRMATIONS
QUORUM_CHECKING
READY_TO_VERIFY
WOULD_VERIFY
VERIFY_TX_ENQUEUED
VERIFIED
QUORUM_CONFLICT
REORG_DETECTED
MANUAL_REVIEW
```

### Phase 1 Mode

默认 DVN mode：

```text
shadow
```

Shadow mode：

- parse packets
- confirm assignment
- wait confirmations
- run RPC quorum
- compute payload hash
- check destination receive config
- generate would-verify report
- do not submit verify transaction

Active mode：

- 与 shadow 相同
- 额外 enqueue verify transaction

## RPC Quorum

Quorum 按链配置。

检查项：

- latest block height
- block hash
- tx receipt
- log presence
- log index
- encoded packet bytes
- source tx success

策略：

- lagging provider：degrade provider
- block hash conflict：pause chain and alert
- receipt conflict：pause pathway and alert
- reorg：job 回滚到 `WAITING_CONFIRMATIONS`

## txmgr

使用 Postgres `tx_outbox` 与 per-chain advisory locks。

Nonce assignment：

1. acquire advisory lock for `(chain_eid, signer_address)`
2. read RPC pending nonce
3. read DB max pending nonce
4. assign `max(rpcPendingNonce, dbMaxNonce + 1)`
5. sign transaction
6. broadcast
7. update `tx_outbox`
8. wait receipt
9. mark complete or retry

## Signer Interface

```go
type Signer interface {
    Address() common.Address
    SignHash(ctx context.Context, digest common.Hash) ([]byte, error)
    SignTx(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
    Type() string
}
```

## AWS KMS Signer

第一阶段只支持：

```text
ECC_SECG_P256K1
```

实现要求：

- request ECDSA signature from AWS KMS
- parse DER r/s
- normalize low-S
- recover Ethereum `v`
- validate recovered address
- integration test with rustack mock AWS-compatible API

## Geth Keystore Signer

使用 geth keystore JSON 格式。

要求：

- load encrypted JSON
- decrypt with configured password or password file
- sign EIP-1559 transactions
- never log private key material

## Worker Acceptance Criteria

- [x] worker starts from config with two configured chains
- [~] worker indexes source events: package and loop boundary exist; ABI-specific log decoding remains pending.
- [ ] worker writes packets into Postgres
- [ ] Executor active flow delivers basic OFT send
- [~] DVN shadow flow produces would-verify records: mode and loop boundary exist; persistence/report generation remains pending.
- [ ] tx_outbox prevents nonce conflicts
- [~] RPC quorum conflict pauses chain/pathway: quorum package boundary exists; provider comparison and pause actions remain pending.
- [x] unsupported options move packet to `MANUAL_REVIEW` state is represented in the state model.
