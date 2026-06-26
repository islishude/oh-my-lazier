# 04 - Database and Config Plan

Status: [x]

## Database

使用 Postgres。

Migrations 目录：

```text
go/migrations/
```

## Core Tables

### chains

```sql
CREATE TABLE chains (
  eid INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  chain_id BIGINT NOT NULL,
  endpoint_address BYTEA NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  paused BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### pathways

```sql
CREATE TABLE pathways (
  id BIGSERIAL PRIMARY KEY,
  src_eid INTEGER NOT NULL,
  dst_eid INTEGER NOT NULL,
  src_oapp BYTEA NOT NULL,
  dst_oapp BYTEA NOT NULL,
  send_lib BYTEA NOT NULL,
  receive_lib BYTEA NOT NULL,
  max_message_size INTEGER NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  paused BOOLEAN NOT NULL DEFAULT false,
  UNIQUE(src_eid, dst_eid, src_oapp, dst_oapp)
);
```

### packets

```sql
CREATE TABLE packets (
  guid BYTEA PRIMARY KEY,
  src_eid INTEGER NOT NULL,
  dst_eid INTEGER NOT NULL,
  nonce NUMERIC NOT NULL,
  sender BYTEA NOT NULL,
  receiver BYTEA NOT NULL,
  send_lib BYTEA NOT NULL,
  src_tx_hash BYTEA NOT NULL,
  src_block_number BIGINT NOT NULL,
  src_log_index INTEGER NOT NULL,
  encoded_packet BYTEA NOT NULL,
  packet_header BYTEA NOT NULL,
  message BYTEA NOT NULL,
  payload_hash BYTEA NOT NULL,
  options BYTEA NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(src_eid, dst_eid, sender, receiver, nonce)
);
```

### executor_jobs

```sql
CREATE TABLE executor_jobs (
  guid BYTEA PRIMARY KEY REFERENCES packets(guid),
  assigned BOOLEAN NOT NULL DEFAULT false,
  assigned_fee NUMERIC,
  status TEXT NOT NULL,
  commit_tx_hash BYTEA,
  receive_tx_hash BYTEA,
  last_error TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### dvn_jobs

```sql
CREATE TABLE dvn_jobs (
  guid BYTEA PRIMARY KEY REFERENCES packets(guid),
  assigned BOOLEAN NOT NULL DEFAULT false,
  confirmations_required BIGINT NOT NULL,
  status TEXT NOT NULL,
  verify_tx_hash BYTEA,
  quorum_result JSONB,
  last_error TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### tx_outbox

```sql
CREATE TABLE tx_outbox (
  id BIGSERIAL PRIMARY KEY,
  chain_eid INTEGER NOT NULL REFERENCES chains(eid),
  purpose TEXT NOT NULL,
  guid BYTEA,
  to_address BYTEA NOT NULL,
  calldata BYTEA NOT NULL,
  value NUMERIC NOT NULL DEFAULT 0,
  gas_limit NUMERIC,
  max_fee_per_gas NUMERIC,
  max_priority_fee_per_gas NUMERIC,
  nonce BIGINT,
  signer_id TEXT NOT NULL,
  status TEXT NOT NULL,
  tx_hash BYTEA,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## Config Files

配置文件目录：

```text
go/configs/
```

必需文件：

```text
chains.sepolia.yaml
pathways.sepolia.yaml
workers.sepolia.yaml
pricing.sepolia.yaml
```

## Config Reload

第一阶段不支持 hot reload。

配置修改后重启 worker。

## Docker Compose

第一阶段 Compose 只包含：

- postgres
- worker

示例：

```yaml
services:
  postgres:
    image: postgres:18-alpine
    environment:
      POSTGRES_DB: lz_workers
      POSTGRES_USER: lz
      POSTGRES_PASSWORD: lz
    ports:
      - "5432:5432"

  worker:
    build:
      context: ./go
      dockerfile: Dockerfile
    depends_on:
      - postgres
    environment:
      DATABASE_URL: postgres://lz:lz@postgres:5432/lz_workers?sslmode=disable
      CONFIG_DIR: /app/configs
    ports:
      - "9090:9090"
```

## 当前实现证据

- `go/migrations/001_initial_schema.sql` 实现计划中的核心表、索引与 `schema_migrations` 跟踪表。
- `go/internal/db.Store.Migrate` 在 worker 启动时应用 embedded migrations，并拒绝已应用 migration 的 checksum drift。
- `go/internal/db.Store.SyncConfig` 将启动配置中的 chain 与 pathway 元数据 upsert 到 Postgres。
- `go/internal/config.Config.Validate` 校验 executor signer、chain endpoint、OpenExecutor/OpenDVN、pathway OApp、SendLib/ReceiveLib、confirmations 与 message size。
- `go/internal/chain.Registry` 同时提供 chain registry、pathway registry 与 worker contract registry。
- `config/example.yaml` 覆盖 executor signer 与 Ethereum Sepolia <-> Base Sepolia 双向 pathway 的必需配置字段。
