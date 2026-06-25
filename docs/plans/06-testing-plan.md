# 06 - Testing Plan

## Contract Unit Tests

### TestOFT

- deploy succeeds
- owner can configure peers
- send pause works
- receive pause works
- outbound rate limit works
- outbound rate limit refills
- rate limit zero blocks send
- unauthorized config reverts

### OpenExecutor

- `getFee` returns expected value
- stale price reverts
- unsupported sendLib reverts
- unsupported sender OApp reverts
- calldata too large reverts
- lzReceive gas below min reverts
- lzReceive gas above max reverts
- lzReceive value nonzero reverts
- lzCompose option reverts
- nativeDrop option reverts
- orderedExecution option reverts
- assignJob requires sufficient msg.value
- pause blocks assignJob
- withdraw only allowed role

### OpenDVN

- `getFee` returns expected value
- stale price reverts
- unsupported sendLib reverts
- unsupported sender OApp reverts
- message size too large reverts
- non-empty DVN options revert
- assignJob requires sufficient msg.value
- pause blocks assignJob
- withdraw only allowed role

## Go Unit Tests

### rpcquorum

- 2-of-3 success
- provider lag detection
- lagging provider degraded
- block hash conflict pauses chain
- receipt/log conflict pauses pathway

### signer

- AWS KMS DER parse
- low-S normalization
- v recovery
- recovered address validation
- geth keystore decrypt
- geth keystore sign tx

### txmgr

- advisory lock
- nonce assignment
- DB max nonce greater than RPC nonce
- RPC nonce greater than DB max nonce
- replacement tx
- receipt confirmation
- retry after failure

### pricing

- Binance primary price
- Uniswap sanity check
- deviation threshold
- stale source handling
- setPriceConfig transaction enqueue

### executor

- PacketSent decode
- ExecutorFeePaid assignment
- commit verification tx build
- lzReceive tx build
- unsupported options cause `MANUAL_REVIEW`

### dvn

- PacketSent decode
- DVNFeePaid assignment
- confirmation wait
- quorum verification
- payload hash computation
- shadow would-verify report

## Testnet Integration Tests

目标：

- Ethereum Sepolia <-> Base Sepolia

流程：

1. Deploy TestOFT on both chains.
2. Deploy OpenExecutor on both chains.
3. Deploy OpenDVN on both chains.
4. Configure OFT peers.
5. Configure worker allowlist.
6. Configure price config.
7. Configure LayerZero ExecutorConfig to OpenExecutor.
8. Keep third-party DVN unchanged.
9. Send small OFT amount.
10. Confirm source `PacketSent`.
11. Confirm `ExecutorFeePaid` points to OpenExecutor.
12. Confirm worker commits verification.
13. Confirm worker calls `lzReceive`.
14. Confirm destination token balance increased.
15. Enable DVN shadow mode.
16. Confirm DVN would-verify report matches third-party verification.
