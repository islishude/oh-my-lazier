package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/islishude/oh-my-lazier/go/internal/config"
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
	store := &Store{pool: pool, maxInflight: config.DefaultMaxInflightPerSigner}
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

func TestRecoveryAcceptedReplayRetryableFailure(t *testing.T) {
	for _, class := range []string{SendErrorRetryableEnv, SendErrorNonceTooHigh} {
		t.Run(class, func(t *testing.T) {
			h := newAttemptHarness(t, "0x9898989898989898989898989898989898989898", 38)
			id := h.enqueue()
			a := h.signAttempt(id, 38, common.HexToHash("0x9898"))
			h.broadcastResult(a.ID, SendErrorAccepted)
			now := time.Now().UTC()
			observe := func(at time.Time, replay bool) {
				t.Helper()
				task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, at, 8)
				if err != nil {
					t.Fatal(err)
				}
				if err = h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: at, Absent: true, Replay: replay}); err != nil {
					t.Fatal(err)
				}
			}
			observe(now, false)
			observe(now.Add(time.Minute), true)
			h.broadcastResult(a.ID, class)
			row, err := h.store.GetOutboxTx(h.ctx, id)
			if err != nil || row.Status != TxStatusBroadcast {
				t.Fatalf("accepted replay must remain broadcast: status=%s err=%v", row.Status, err)
			}
			var state string
			var deadline time.Time
			if err = h.store.pool.QueryRow(h.ctx, `SELECT a.state,o.next_recovery_at FROM tx_outbox o JOIN tx_attempts a ON a.id=o.active_attempt_id WHERE o.id=$1`, id).Scan(&state, &deadline); err != nil {
				t.Fatal(err)
			}
			if state != TxAttemptSubmitted {
				t.Fatalf("acceptance lost: %s", state)
			}
			if _, err = h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrNoBroadcastCandidate) {
				t.Fatalf("replayed without new authorization: %v", err)
			}
			// A fresh observation before the action deadline must not spend budget.
			observe(deadline.Add(-time.Millisecond), true)
			if _, err = h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrNoBroadcastCandidate) {
				t.Fatalf("replayed before recovery deadline: %v", err)
			}
			observe(deadline.Add(time.Minute), true)
			claim, err := h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute)
			if err != nil {
				t.Fatalf("retryable replay cannot recover: %v", err)
			}
			if claim.AttemptID != a.ID || !bytes.Equal(claim.RawTx, a.RawTx) || claim.Nonce != 38 {
				t.Fatalf("recovery changed the transaction: %+v", claim)
			}
		})
	}
}

// environmentalHold builds a never-accepted exhausted attempt, then collects
// two fresh quorum-absence observations without sleeping in integration tests.
func (h *attemptHarness) environmentalHold() (int64, TxAttempt) {
	h.t.Helper()
	id := h.enqueue()
	a := h.signAttempt(id, 7, common.BigToHash(big.NewInt(0x70710000+id)))
	h.broadcastResult(a.ID, SendErrorRetryableEnv)
	h.exhaustAttempt(a.ID)
	if _, err := h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrBroadcastLaneHeld) {
		h.t.Fatalf("park: %v", err)
	}
	h.environmentalEvidence(id, a.ID)
	return id, a
}

func (h *attemptHarness) environmentalEvidence(id, attemptID int64) {
	h.t.Helper()
	if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_attempts SET last_broadcast_at=now()-interval '2 minutes' WHERE id=$1`, attemptID); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_recovery_at=now()-interval '1 minute',next_visibility_at=NULL WHERE id=$1`, id); err != nil {
		h.t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, at := range []time.Time{now.Add(-61 * time.Second), now} {
		task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, at, 8)
		if err != nil {
			h.t.Fatal(err)
		}
		nonce := task.Nonce
		if err := h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: at, Absent: true, Nonce: &nonce}); err != nil {
			h.t.Fatal(err)
		}
	}
}

func TestEnvironmentalReplacementEligibilityAndPublication(t *testing.T) {
	for _, phase := range []string{"claim", "publish"} {
		for _, tc := range []struct {
			name, outbox, attempt, lane string
			ok                          bool
		}{
			{name: "eligible", ok: true},
			{name: "unknown class", attempt: "send_error_class=NULL"},
			{name: "ambiguous", attempt: "send_error_class='ambiguous'"},
			{name: "definitive", attempt: "send_error_class='definitive'"},
			{name: "nonce too low", attempt: "send_error_class='nonce_too_low'"},
			{name: "nonce too high", attempt: "send_error_class='nonce_too_high'"},
			{name: "cancel", attempt: "kind='cancel'"},
			{name: "cancel intent", outbox: "cancel_requested_at=now()"},
			{name: "manual hold", outbox: "held_reason='manual'"},
			{name: "accepted meanwhile", outbox: "status='broadcast',held_reason=NULL", attempt: "state='submitted',send_error_class='accepted'"},
			{name: "sign budget", outbox: "pre_sign_failure_count=5"},
			{name: "replacement cap"},
			{name: "broadcast budget not exhausted", attempt: "broadcast_count=4"},
			{name: "missing evidence", outbox: "absent_since=NULL"},
			{name: "short absence", outbox: "absent_since=now()-interval '10 seconds'"},
			{name: "stale evidence", outbox: "visibility_checked_at=now()-interval '3 minutes'"},
			{name: "missing nonce", lane: "observed_nonce=NULL"},
			{name: "stale nonce", lane: "nonce_observed_at=now()-interval '3 minutes'"},
			{name: "nonce advanced", lane: "observed_nonce=8"},
			{name: "cooldown", outbox: "next_recovery_at=now()+interval '1 minute'"},
			{name: "broadcast lease", attempt: "broadcast_lease_token='11111111-1111-1111-1111-111111111111',broadcast_lease_until=now()+interval '1 minute'"},
			{name: "recovery lease", outbox: "recovery_lease_token='11111111-1111-1111-1111-111111111111',recovery_lease_until=now()+interval '1 minute'"},
			{name: "receipt", outbox: "receipt_outcome='confirmed',receipt_attempt_id=active_attempt_id"},
		} {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				h := newAttemptHarness(t, "0x7171717171717171717171717171717171717171", 7)
				id, a := h.environmentalHold()
				candidate, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute)
				if err != nil || !candidate.EnvironmentalRecovery {
					t.Fatalf("candidate=%+v err=%v", candidate, err)
				}
				token := uuid.New()
				if phase == "publish" {
					if _, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, token, time.Minute, true); err != nil {
						t.Fatal(err)
					}
				}
				if tc.outbox != "" {
					if _, err := h.store.pool.Exec(h.ctx, "UPDATE tx_outbox SET "+tc.outbox+" WHERE id=$1", id); err != nil {
						t.Fatal(err)
					}
				}
				if tc.attempt != "" {
					if _, err := h.store.pool.Exec(h.ctx, "UPDATE tx_attempts SET "+tc.attempt+" WHERE id=$1", a.ID); err != nil {
						t.Fatal(err)
					}
				}
				if tc.lane != "" {
					if _, err := h.store.pool.Exec(h.ctx, "UPDATE tx_recovery_lanes SET "+tc.lane+" WHERE chain_eid=40161 AND signer_id=$1", h.signerID); err != nil {
						t.Fatal(err)
					}
				}
				if tc.name == "replacement cap" {
					_, err := h.store.pool.Exec(h.ctx, `INSERT INTO tx_attempts
                    (outbox_id,kind,nonce,tx_type,tx_hash,raw_tx,gas_limit,max_fee_per_gas,max_priority_fee_per_gas,state,signing_token)
                    SELECT a.outbox_id,'replacement',a.nonce,a.tx_type,
                     decode(md5(a.id::text||n::text)||md5('environmental-cap'||a.id::text||n::text),'hex'),
                     a.raw_tx,a.gas_limit,a.max_fee_per_gas,a.max_priority_fee_per_gas,'ambiguous',gen_random_uuid()
                    FROM tx_attempts a CROSS JOIN generate_series(1,5) n WHERE a.id=$1`, a.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				if phase == "claim" {
					_, err = h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, token, time.Minute, true)
				} else {
					_, err = h.store.InsertReplacementAttempt(h.ctx, id, a.ID, token, SignedAttempt{EnvironmentalRecovery: true, Kind: TxAttemptReplacement, Nonce: 7, TxHash: common.BigToHash(big.NewInt(0x70720000 + id)), RawTx: []byte{1}, GasLimit: 21000, MaxFeePerGas: big.NewInt(3_000_000_000), SigningToken: uuid.New()})
				}
				if (err == nil) != tc.ok {
					t.Fatalf("err=%v want success=%v", err, tc.ok)
				}
			})
		}
	}
}

func TestEnvironmentalRecoveryBudgetAndAge(t *testing.T) {
	h := newAttemptHarness(t, "0x7272727272727272727272727272727272727272", 7)
	id, a := h.environmentalHold()
	for round := 0; round <= TxMaxReplacements; round++ {
		if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_attempts SET created_at=now()-interval '20 minutes' WHERE id=$1`, a.ID); err != nil {
			t.Fatal(err)
		}
		if err := h.store.DeferReplacement(h.ctx, id); err != nil {
			t.Fatal(err)
		}
		stats, err := h.store.Stats(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range stats.TxOutboxHeld {
			if s.SignerID == h.signerID {
				want := HeldBroadcastRetrying
				if round == TxMaxReplacements {
					want = HeldBroadcastExhausted
				}
				if s.HeldReason != want || s.OldestAgeSeconds < 1200 {
					t.Fatalf("stats=%+v want %s with stable age", s, want)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("missing held stats")
		}
		if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_recovery_at=now()-interval '1 second' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		c, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute)
		if round == TxMaxReplacements {
			if !errors.Is(err, ErrNoStaleBroadcastReplacement) {
				t.Fatalf("cap: %v", err)
			}
			if _, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, uuid.New(), time.Minute, true); err == nil {
				t.Fatal("claim bypassed cap")
			}
			break
		}
		if err != nil || !c.EnvironmentalRecovery {
			t.Fatalf("candidate: %+v %v", c, err)
		}
		token := uuid.New()
		if _, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, token, time.Minute, true); err != nil {
			t.Fatal(err)
		}
		a, err = h.store.InsertReplacementAttempt(h.ctx, id, a.ID, token, SignedAttempt{EnvironmentalRecovery: true, Kind: TxAttemptReplacement, Nonce: 7, TxHash: common.BigToHash(big.NewInt(0x72000000 + id*10 + int64(round))), RawTx: []byte{byte(round + 1)}, GasLimit: 21000, MaxFeePerGas: big.NewInt(3_000_000_000), SigningToken: uuid.New()})
		if err != nil {
			t.Fatal(err)
		}
		h.broadcastResult(a.ID, SendErrorRetryableEnv)
		h.exhaustAttempt(a.ID)
		if _, err := h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); !errors.Is(err, ErrBroadcastLaneHeld) {
			t.Fatal(err)
		}
		if round+1 < TxMaxReplacements {
			h.environmentalEvidence(id, a.ID)
		}
	}
}

func TestEnvironmentalRecoveryLeasesAndHead(t *testing.T) {
	for _, scenario := range []string{"legacy hold", "lower nonce", "manual replay fences probe", "replacement fences probe", "concurrent replay and replacement"} {
		t.Run(scenario, func(t *testing.T) {
			h := newAttemptHarness(t, "0x7373737373737373737373737373737373737373", 7)
			id, a := h.environmentalHold()
			switch scenario {
			case "legacy hold":
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET first_broadcast_at=NULL,absent_since=NULL,visibility_checked_at=NULL,next_visibility_at=NULL,next_recovery_at=now()+interval '8 minutes' WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, time.Now(), 8)
				if err != nil {
					t.Fatal(err)
				}
				if task.FirstBroadcastAt == nil || task.Broadcasts != TxMaxBroadcasts {
					t.Fatalf("legacy evidence lost: %+v", task)
				}
				var bounded bool
				if err := h.store.pool.QueryRow(h.ctx, `SELECT next_recovery_at<=now()+interval '61 seconds' AND next_recovery_at>now() FROM tx_outbox WHERE id=$1`, id).Scan(&bounded); err != nil || !bounded {
					t.Fatalf("legacy cooldown: %v %v", bounded, err)
				}
				if err := h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: time.Now(), Absent: true, Nonce: &task.Nonce}); err != nil {
					t.Fatal(err)
				}
				if _, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute); !errors.Is(err, ErrNoStaleBroadcastReplacement) {
					t.Fatalf("legacy hold bypassed evidence: %v", err)
				}
			case "lower nonce":
				lower := h.enqueue()
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET status='nonce_assigned',nonce=6 WHERE id=$1`, lower); err != nil {
					t.Fatal(err)
				}
				if _, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute); !errors.Is(err, ErrNoStaleBroadcastReplacement) {
					t.Fatalf("skipped lower nonce: %v", err)
				}
				if _, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, uuid.New(), time.Minute, true); err == nil {
					t.Fatal("claim skipped lower nonce")
				}
				if _, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, time.Now(), 8); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatalf("probe skipped lower nonce: %v", err)
				}
			case "manual replay fences probe":
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_visibility_at=NULL WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, time.Now(), 8)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := h.store.GetRebroadcastAttempt(h.ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				token := uuid.New()
				if err := h.store.ClaimManualRebroadcast(h.ctx, raw, token, time.Minute); err != nil {
					t.Fatal(err)
				}
				if err := h.store.MarkAttemptSendResult(h.ctx, a.ID, token, SendErrorRetryableEnv, ""); err != nil {
					t.Fatal(err)
				}
				if err := h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, RecoveryObservation{Now: time.Now(), Absent: true, Nonce: &task.Nonce}); !errors.Is(err, ErrOutboxLeaseLost) {
					t.Fatalf("stale probe survived replay: %v", err)
				}
				if _, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute); !errors.Is(err, ErrNoStaleBroadcastReplacement) {
					t.Fatalf("reused pre-send evidence: %v", err)
				}
			case "replacement fences probe":
				if _, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, uuid.New(), time.Minute, true); err != nil {
					t.Fatal(err)
				}
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_visibility_at=NULL WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				if _, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, time.Now(), 8); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatalf("probe raced signer: %v", err)
				}
			case "concurrent replay and replacement":
				raw, err := h.store.GetRebroadcastAttempt(h.ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				start := make(chan struct{})
				results := make(chan error, 2)
				go func() { <-start; results <- h.store.ClaimManualRebroadcast(h.ctx, raw, uuid.New(), time.Minute) }()
				go func() {
					<-start
					_, err := h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, uuid.New(), time.Minute, true)
					results <- err
				}()
				close(start)
				wins := 0
				for range 2 {
					if <-results == nil {
						wins++
					}
				}
				if wins != 1 {
					t.Fatalf("concurrent claims=%d want 1", wins)
				}
			}
		})
	}
}

func TestEnvironmentalRecoveryRequiresContinuousAbsence(t *testing.T) {
	for _, gap := range []bool{false, true} {
		t.Run(fmt.Sprintf("gap=%v", gap), func(t *testing.T) {
			h := newAttemptHarness(t, "0x7474747474747474747474747474747474747474", 7)
			id, _ := h.environmentalHold()
			if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_visibility_at=NULL WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			if gap {
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET visibility_checked_at=now()-interval '3 minutes',absent_since=now()-interval '4 minutes' WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC()
			task, err := h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now, 8)
			if err != nil {
				t.Fatal(err)
			}
			obs := RecoveryObservation{Now: now, Reason: "rpc_unavailable"}
			if gap {
				obs = RecoveryObservation{Now: now, Absent: true, Nonce: &task.Nonce}
			}
			if err := h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, obs); err != nil {
				t.Fatal(err)
			}
			var absent *time.Time
			if err := h.store.pool.QueryRow(h.ctx, `SELECT absent_since FROM tx_outbox WHERE id=$1`, id).Scan(&absent); err != nil {
				t.Fatal(err)
			}
			if gap {
				if absent == nil || absent.Before(now.Add(-time.Second)) {
					t.Fatalf("gap retained old absence: %v", absent)
				}
			} else if absent != nil {
				t.Fatal("RPC failure counted as absence")
			}
			if _, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute); !errors.Is(err, ErrNoStaleBroadcastReplacement) {
				t.Fatalf("replaced without continuous evidence: %v", err)
			}
		})
	}
}

func TestEnvironmentalRecoveryYieldsToOperatorReplacement(t *testing.T) {
	for _, timing := range []string{"before probe", "during probe", "deferred request"} {
		for _, outcome := range []string{"rpc_unavailable", "visible", "absent", "nonce consumed"} {
			t.Run(timing+"/"+outcome, func(t *testing.T) {
				h := newAttemptHarness(t, "0x7575757575757575757575757575757575757575", 7)
				id, a := h.environmentalHold()
				if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET next_visibility_at=NULL WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
				now := time.Now().UTC()
				var task RecoveryTask
				var err error
				if timing == "during probe" {
					task, err = h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now, 8)
					if err != nil {
						t.Fatal(err)
					}
				}
				if err = h.store.RequestTxReplacement(h.ctx, id); err != nil {
					t.Fatal(err)
				}
				if timing == "deferred request" {
					if err = h.store.DeferReplacement(h.ctx, id); err != nil {
						t.Fatal(err)
					}
				}
				if timing == "during probe" {
					obs := RecoveryObservation{Now: now}
					switch outcome {
					case "rpc_unavailable":
						obs.Reason = outcome
					case "visible":
						obs.Visible = true
					case "absent":
						obs.Absent = true
						obs.Nonce = &task.Nonce
					case "nonce consumed":
						nonce := task.Nonce + 1
						obs.Nonce = &nonce
					}
					if err = h.store.FinishRecovery(h.ctx, task, 40161, h.signerID, obs); !errors.Is(err, ErrOutboxLeaseLost) {
						t.Fatalf("probe overwrote operator request: %v", err)
					}
				}
				// Repeated due passes must not re-arm the automatic cooldown,
				// including while an operator request awaits its own preflight delay.
				for range 3 {
					if _, err = h.store.ClaimRecovery(h.ctx, 40161, h.signerID, now, 8); !errors.Is(err, pgx.ErrNoRows) {
						t.Fatalf("operator-owned row was probed: %v", err)
					}
				}
				if timing == "deferred request" {
					if _, err = h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute); !errors.Is(err, ErrNoStaleBroadcastReplacement) {
						t.Fatalf("operator preflight delay lost: %v", err)
					}
					if _, err = h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET replace_requested_at=now()-interval '1 second',next_recovery_at=now()-interval '1 second' WHERE id=$1`, id); err != nil {
						t.Fatal(err)
					}
				}
				candidate, err := h.store.NextReplacementCandidate(h.ctx, 40161, h.signerID, time.Minute)
				if err != nil || candidate.EnvironmentalRecovery {
					t.Fatalf("operator replacement suppressed: %+v %v", candidate, err)
				}
				token := uuid.New()
				if _, err = h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, token, time.Minute, candidate.EnvironmentalRecovery); err != nil {
					t.Fatal(err)
				}
				if _, err = h.store.InsertReplacementAttempt(h.ctx, id, a.ID, token, SignedAttempt{Kind: TxAttemptReplacement, Nonce: 7, TxHash: common.BigToHash(big.NewInt(0x75000000 + id)), RawTx: []byte{1}, GasLimit: 21000, MaxFeePerGas: big.NewInt(3_000_000_000), SigningToken: uuid.New()}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
