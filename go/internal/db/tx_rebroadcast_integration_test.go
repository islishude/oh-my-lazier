package db

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
)

func TestManualRebroadcastEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, outbox, attempt string
		changed               bool
		ok                    bool
	}{
		{name: "signed", ok: true},
		{name: "paused", ok: true},
		{name: "broadcast", outbox: "status='broadcast'", attempt: "state='submitted'", ok: true},
		{name: "exhausted", outbox: "status='held',held_reason='broadcast_exhausted'", attempt: "state='ambiguous',broadcast_count=5", ok: true},
		{name: "manual hold", outbox: "status='held',held_reason='manual'"},
		{name: "reprice hold", outbox: "status='held',held_reason='reprice_required'"},
		{name: "nonce hold", outbox: "status='held',held_reason='nonce_reconcile_required'"},
		{name: "external nonce", outbox: "status='held',held_reason='nonce_consumed_externally'"},
		{name: "confirmed", outbox: "status='confirmed'"},
		{name: "failed", outbox: "status='failed'"},
		{name: "missing attempt", outbox: "active_attempt_id=NULL"},
		{name: "receipt", outbox: "receipt_outcome='confirmed',receipt_attempt_id=active_attempt_id"},
		{name: "pending cancel", outbox: "cancel_requested_at=now()"},
		{name: "cancel attempt", outbox: "cancel_requested_at=now()", attempt: "kind='cancel'", ok: true},
		{name: "signing lease", outbox: "lease_token='11111111-1111-1111-1111-111111111111',lease_until=now()+interval '1 minute'"},
		{name: "broadcast lease", attempt: "broadcast_lease_token='11111111-1111-1111-1111-111111111111',broadcast_lease_until=now()+interval '1 minute'"},
		{name: "changed attempt", changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newAttemptHarness(t, "0x8383838383838383838383838383838383838383", 7)
			id := h.enqueue()
			h.signAttempt(id, 7, common.HexToHash("0x8383"))
			snapshot, err := h.store.GetRebroadcastAttempt(h.ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "paused" {
				restoreScopeFlags(t, h.store)
				setChainFlags(t, h, 40161, true, true)
			}
			if tc.outbox != "" {
				if _, err = h.store.pool.Exec(h.ctx, "UPDATE tx_outbox SET "+tc.outbox+" WHERE id=$1", id); err != nil {
					t.Fatal(err)
				}
			}
			if tc.attempt != "" {
				if _, err = h.store.pool.Exec(h.ctx, "UPDATE tx_attempts SET "+tc.attempt+" WHERE id=$1", snapshot.AttemptID); err != nil {
					t.Fatal(err)
				}
			}
			if tc.changed {
				snapshot.AttemptID++
			}
			err = h.store.ClaimManualRebroadcast(h.ctx, snapshot, uuid.New(), time.Minute)
			if (err == nil) != tc.ok {
				t.Fatalf("claim error=%v; want success=%v", err, tc.ok)
			}
		})
	}
}

func TestManualRebroadcastBudgetAndLeases(t *testing.T) {
	h := newAttemptHarness(t, "0x8484848484848484848484848484848484848484", 7)
	id := h.enqueue()
	a := h.signAttempt(id, 7, common.HexToHash("0x8484"))
	h.exhaustAttempt(a.ID)
	if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET status='held',held_reason='broadcast_exhausted',first_broadcast_at=now()-interval '1 day' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	snapshot, err := h.store.GetRebroadcastAttempt(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	token := uuid.New()
	if err = h.store.ClaimManualRebroadcast(h.ctx, snapshot, token, time.Minute); err != nil {
		t.Fatal(err)
	}
	var count int64
	var state string
	if err = h.store.pool.QueryRow(h.ctx, `SELECT broadcast_count,state FROM tx_attempts WHERE id=$1`, a.ID).Scan(&count, &state); err != nil {
		t.Fatal(err)
	}
	if count != 6 || state != TxAttemptAmbiguous {
		t.Fatalf("count=%d state=%s", count, state)
	}
	if _, err = h.store.ClaimOutboxForReplacementSigning(h.ctx, id, a.ID, uuid.New(), time.Minute); !errors.Is(err, ErrOutboxLeaseLost) {
		t.Fatalf("replacement raced send: %v", err)
	}
	if err = h.store.ClaimManualRebroadcast(h.ctx, snapshot, uuid.New(), time.Minute); err == nil {
		t.Fatal("duplicate manual claim")
	}
	if _, err = h.store.ClaimAttemptForBroadcast(h.ctx, 40161, h.signerID, uuid.New(), time.Minute); err == nil {
		t.Fatal("worker raced send")
	}
	if err = h.store.MarkAttemptSendResult(h.ctx, a.ID, token, SendErrorAccepted, ""); err != nil {
		t.Fatal(err)
	}
	var status string
	var released, old bool
	if err = h.store.pool.QueryRow(h.ctx, `SELECT status,lease_token IS NULL,first_broadcast_at<now()-interval '23 hours' FROM tx_outbox WHERE id=$1`, id).Scan(&status, &released, &old); err != nil {
		t.Fatal(err)
	}
	if status != TxStatusBroadcast || !released || !old {
		t.Fatal("lost state, lease release or original broadcast age")
	}
	// A second explicit command authorizes one further replay, even during cooldown.
	token = uuid.New()
	if err = h.store.ClaimManualRebroadcast(h.ctx, snapshot, token, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = h.store.MarkAttemptSendResult(h.ctx, a.ID, token, SendErrorAmbiguous, "unrecognized broadcast error"); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err = h.store.pool.QueryRow(h.ctx, `SELECT broadcast_count,raw_tx FROM tx_attempts WHERE id=$1`, a.ID).Scan(&count, &raw); err != nil {
		t.Fatal(err)
	}
	if count != 7 || !bytes.Equal(raw, a.RawTx) {
		t.Fatal("budget reset or raw changed")
	}
}

func TestManualRebroadcastConcurrentClaimsAndWritebackLoss(t *testing.T) {
	h := newAttemptHarness(t, "0x8585858585858585858585858585858585858585", 7)
	id := h.enqueue()
	a := h.signAttempt(id, 7, common.HexToHash("0x8585"))
	snapshot, err := h.store.GetRebroadcastAttempt(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	tokens := []uuid.UUID{uuid.New(), uuid.New()}
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() { results[i] = h.store.ClaimManualRebroadcast(h.ctx, snapshot, tokens[i], time.Minute) })
	}
	wg.Wait()
	winner := -1
	for i, err := range results {
		if err == nil {
			if winner != -1 {
				t.Fatal("multiple winners")
			}
			winner = i
		}
	}
	if winner == -1 {
		t.Fatalf("no winner: %v", results)
	}
	if err = h.store.RequestTxCancel(h.ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.ClaimOutboxForCancelSigning(h.ctx, id, a.ID, uuid.New(), time.Minute); !errors.Is(err, ErrOutboxLeaseLost) {
		t.Fatalf("cancel signing raced broadcast: %v", err)
	}
	// Simulate an acknowledgement arriving after the sender's lease expires.
	if _, err = h.store.pool.Exec(h.ctx, `UPDATE tx_attempts SET broadcast_lease_until=now()-interval '1 second' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = h.store.MarkAttemptSendResult(h.ctx, a.ID, tokens[winner], SendErrorAccepted, ""); !errors.Is(err, ErrOutboxLeaseLost) {
		t.Fatalf("stale writeback=%v", err)
	}
	tasks, err := h.store.ListReceiptPollTasks(h.ctx, 40161, h.signerID, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range tasks {
		for _, attempt := range task.Attempts {
			if attempt.ID == a.ID {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("possibly accepted attempt lost from receipt tracking")
	}
}

func TestManualRebroadcastHeadOnlyAndSigningRace(t *testing.T) {
	h := newAttemptHarness(t, "0x8686868686868686868686868686868686868686", 7)
	first := h.enqueue()
	a := h.signAttempt(first, 7, common.HexToHash("0x8686"))
	h.broadcastResult(a.ID, SendErrorAccepted)
	second := h.enqueue()
	h.signAttempt(second, 8, common.HexToHash("0x8687"))
	higher, err := h.store.GetRebroadcastAttempt(h.ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.store.ClaimManualRebroadcast(h.ctx, higher, uuid.New(), time.Minute); err == nil {
		t.Fatal("higher nonce replayed")
	}
	snapshot, err := h.store.GetRebroadcastAttempt(h.ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.ClaimOutboxForReplacementSigning(h.ctx, first, a.ID, uuid.New(), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = h.store.ClaimManualRebroadcast(h.ctx, snapshot, uuid.New(), time.Minute); err == nil {
		t.Fatal("replayed during replacement signing")
	}
}
