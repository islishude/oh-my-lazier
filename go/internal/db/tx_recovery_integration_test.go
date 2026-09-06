package db

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/islishude/oh-my-lazier/go/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecoveryReplayIsFencedDurableAndHeadOnly(t *testing.T) {
	h := newAttemptHarness(t, "0x9090909090909090909090909090909090909090", 38)
	id := h.enqueue()
	a := h.signAttempt(id, 38, common.HexToHash("0x9038"))
	h.broadcastResult(a.ID, SendErrorAccepted)
	next := h.enqueue()
	b := h.signAttempt(next, 39, common.HexToHash("0x9039"))
	h.broadcastResult(b.ID, SendErrorAccepted)
	now := time.Now().UTC()
	task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now, 8)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != id {
		t.Fatalf("selected higher nonce: %d", task.ID)
	}
	if _, err = h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now, 8); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate claim: %v", err)
	}
	if err = h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: now, Absent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now.Add(30*time.Second), 8); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("skipped delayed head: %v", err)
	}
	task, err = h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now.Add(time.Minute), 8)
	if err != nil {
		t.Fatal(err)
	}
	if task.AbsentSince == nil || task.Broadcasts != 1 {
		t.Fatalf("lost evidence: %+v", task)
	}
	if err = h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: now.Add(time.Minute), Absent: true, Replay: true}); err != nil {
		t.Fatal(err)
	}
	claim, err := h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claim.AttemptID != a.ID || !bytes.Equal(claim.RawTx, a.RawTx) {
		t.Fatal("replay changed raw or attempt")
	}
	if _, err = h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrNoBroadcastCandidate) {
		t.Fatalf("duplicate replay: %v", err)
	}
	inspection, err := h.store.InspectTx(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(inspection, []byte("raw_tx")) || !bytes.Contains(inspection, []byte(`"broadcast_count": 2`)) {
		t.Fatalf("unsafe or incorrect inspection: %s", inspection)
	}
	if _, err = h.store.recoveryStats(h.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryWindowAndExistingExcess(t *testing.T) {
	h := newAttemptHarness(t, "0x9191919191919191919191919191919191919191", 10)
	h.store.SetMaxInflight(2)
	for i := range 2 {
		id := h.enqueue()
		a := h.signAttempt(id, uint64(10+i), common.BigToHash(big.NewInt(int64(100+i))))
		h.broadcastResult(a.ID, SendErrorAccepted)
	}
	id := h.enqueue()
	if _, err := h.store.ClaimOutboxForSigning(h.ctx, id, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrSignerLaneBlocked) {
		t.Fatalf("window exceeded: %v", err)
	}
	h.store.SetMaxInflight(1)
	if _, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, time.Now(), 1); err != nil {
		t.Fatalf("existing excess cannot recover: %v", err)
	}
}

func TestRecoveryReasonClockAndLogPacing(t *testing.T) {
	h := newAttemptHarness(t, "0x9292929292929292929292929292929292929292", 10)
	id := h.enqueue()
	a := h.signAttempt(id, 10, common.HexToHash("0x92"))
	h.broadcastResult(a.ID, SendErrorAccepted)
	now := time.Now()
	for _, tc := range []struct {
		after  time.Duration
		reason string
		due    bool
	}{{0, "fee_cap", true}, {time.Minute, "fee_cap", false}, {5 * time.Minute, "fee_cap", true}, {6 * time.Minute, "", true}} {
		due, _, err := h.store.SetRecoveryReason(h.ctx, id, a.ID, tc.reason, nil, now.Add(tc.after))
		if err != nil || due != tc.due {
			t.Fatalf("reason %s: %v %v", tc.reason, due, err)
		}
		if tc.reason != "" {
			var since time.Time
			if err = h.store.pool.QueryRow(h.ctx, `SELECT recovery_since FROM tx_outbox WHERE id=$1`, id).Scan(&since); err != nil {
				t.Fatal(err)
			}
			if since.Sub(now).Abs() > time.Millisecond {
				t.Fatal("deferral refreshed age")
			}
		}
	}
}

func TestRecoveryUpgradePreservesOldAttempts(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}
	admin, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "recovery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{"001_initial_schema.sql", "002_txmgr_attempts.sql", "003_txmgr_cancel_pacing.sql"} {
		body, e := migrations.Files.ReadFile(name)
		if e != nil {
			t.Fatal(e)
		}
		tx, e := pool.Begin(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		if name == "001_initial_schema.sql" {
			if _, e = tx.Exec(t.Context(), `CREATE TABLE schema_migrations(version text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); e != nil {
				t.Fatal(e)
			}
		}
		if e = applyMigration(t.Context(), tx, name, migrationChecksum(body), string(body)); e != nil {
			_ = tx.Rollback(t.Context())
			t.Fatal(e)
		}
		if e = tx.Commit(t.Context()); e != nil {
			t.Fatal(e)
		}
	}
	_, err = pool.Exec(t.Context(), `INSERT INTO chains(eid,name,chain_id,endpoint_address) VALUES(1,'test',1,decode(repeat('11',20),'hex'));
 INSERT INTO tx_outbox(id,chain_eid,purpose,to_address,calldata,nonce,signer_id,status) VALUES(1,1,'dvn_verify',decode(repeat('11',20),'hex'),'\x01',38,'worker','broadcast');
 INSERT INTO tx_attempts(id,outbox_id,kind,nonce,tx_type,tx_hash,raw_tx,gas_limit,max_fee_per_gas,max_priority_fee_per_gas,state,signing_token,broadcast_count,created_at,last_broadcast_at)
 VALUES(1,1,'replacement',38,2,decode(repeat('22',32),'hex'),'\x010203',21000,100,10,'submitted','00000000-0000-0000-0000-000000000001',4,now()-interval '2 days',now()-interval '1 day');
 UPDATE tx_outbox SET active_attempt_id=1 WHERE id=1;`)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{pool: pool, maxInflight: 8}
	if err = store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	var first *time.Time
	var raw []byte
	if err = pool.QueryRow(t.Context(), `SELECT a.broadcast_count,o.first_broadcast_at,a.raw_tx FROM tx_outbox o JOIN tx_attempts a ON a.id=o.active_attempt_id`).Scan(&count, &first, &raw); err != nil {
		t.Fatal(err)
	}
	if count != 4 || first != nil || !bytes.Equal(raw, []byte{1, 2, 3}) {
		t.Fatal("migration backfilled or changed history")
	}
	task, err := store.ClaimRecovery(t.Context(), 1, "worker", time.Now(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if task.Broadcasts != 4 || task.Replacements != 1 || task.FirstBroadcastAt == nil || time.Since(*task.FirstBroadcastAt) < 47*time.Hour {
		t.Fatalf("lost old evidence: %+v", task)
	}
	if err = store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
}
