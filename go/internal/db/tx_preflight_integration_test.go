package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPreflightObservationOrderingRecoveryAndScope(t *testing.T) {
	h := newAttemptHarness(t, "preflight-observations", 8)
	restoreScopeFlags(t, h.store)
	id := h.enqueue()
	token := func() int64 {
		t.Helper()
		n, err := h.store.NextPreflightObservation(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	record := func(n int64, reason string, wantApplied, wantRecovery bool) {
		t.Helper()
		a, r, err := h.store.RecordPreflight(h.ctx, id, n, reason, reason)
		if err != nil || a != wantApplied || r != wantRecovery {
			t.Fatalf("record %d %s = %t %t %v", n, reason, a, r, err)
		}
	}
	first, old, newer := token(), token(), token()
	record(first, "invalid_priority_fee", true, false)
	if _, err := h.store.pool.Exec(h.ctx, `UPDATE tx_outbox SET preflight_failure_since=now()-interval '5 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	record(newer, "fee_cap", true, false)
	record(old, "", false, false)
	stats, err := h.store.txPreflightStats(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range stats {
		if s.SignerID == h.signerID {
			found = true
			if s.Reason != "fee_cap" || s.OldestAgeSeconds < 300 || s.Count != 1 {
				t.Fatalf("stats=%+v", s)
			}
		}
	}
	if !found {
		t.Fatal("no preflight stats")
	}
	// A new process/connection sees the same failure clock and newer token order.
	reopened, err := Connect(h.ctx, os.Getenv("TEST_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	inspection, err := reopened.InspectTx(h.ctx, id)
	if err != nil || !strings.Contains(string(inspection), `"preflight_reason": "fee_cap"`) {
		t.Fatalf("inspection=%s %v", inspection, err)
	}
	for _, tc := range []struct {
		name                     string
		enabled, paused, visible bool
	}{
		{"paused", true, true, true}, {"disabled", false, false, false}, {"active", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.store.pool.Exec(h.ctx, `UPDATE chains SET enabled=$1,paused=$2 WHERE eid=40161`, tc.enabled, tc.paused); err != nil {
				t.Fatal(err)
			}
			stats, err := h.store.txPreflightStats(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			seen := false
			for _, s := range stats {
				if s.SignerID == h.signerID {
					seen = true
				}
			}
			if seen != tc.visible {
				t.Fatalf("visible=%t", seen)
			}
		})
	}
	stale, success := token(), token()
	record(success, "", true, true)
	record(stale, "invalid_priority_fee", false, false)
	record(token(), "estimate_gas_error", true, false)
	// Once another instance claims the nonce, late RPC failures cannot resurrect queued diagnostics.
	late := token()
	if _, err := h.store.ClaimOutboxForSigning(h.ctx, id, 40161, h.signerID, uuid.New(), time.Minute); err != nil {
		t.Fatal(err)
	}
	record(late, "fee_cap", false, false)
	stats, err = h.store.txPreflightStats(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		if s.SignerID == h.signerID {
			t.Fatalf("claimed row still visible: %+v", s)
		}
	}
}

func TestPreflightConcurrentObservations(t *testing.T) {
	h := newAttemptHarness(t, "preflight-concurrent", 1)
	for range 12 {
		id := h.enqueue()
		older, err := h.store.NextPreflightObservation(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		newer, err := h.store.NextPreflightObservation(h.ctx)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 2)
		go func() { _, _, e := h.store.RecordPreflight(h.ctx, id, older, "fee_cap", "cap"); done <- e }()
		go func() { _, _, e := h.store.RecordPreflight(h.ctx, id, newer, "", ""); done <- e }()
		for range 2 {
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		}
		var reason string
		if err := h.store.pool.QueryRow(h.ctx, `SELECT preflight_reason FROM tx_outbox WHERE id=$1`, id).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if reason != "" {
			t.Fatalf("stale failure won: %s", reason)
		}
	}
}

func TestStatsDeadlineCancelsPoolWaitAndQuery(t *testing.T) {
	h := newAttemptHarness(t, "preflight-stats-deadline", 1)
	for _, mode := range []string{"query lock", "pool exhausted"} {
		t.Run(mode, func(t *testing.T) {
			var release func()
			if mode == "query lock" {
				tx, err := h.store.pool.Begin(h.ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(h.ctx, `LOCK TABLE chains IN ACCESS EXCLUSIVE MODE`); err != nil {
					t.Fatal(err)
				}
				release = func() { _ = tx.Rollback(h.ctx) }
			} else {
				conns := h.store.pool.AcquireAllIdle(h.ctx)
				for len(conns) < int(h.store.pool.Config().MaxConns) {
					c, err := h.store.pool.Acquire(h.ctx)
					if err != nil {
						t.Fatal(err)
					}
					conns = append(conns, c)
				}
				release = func() {
					for _, c := range conns {
						c.Release()
					}
				}
			}
			defer release()
			ctx, cancel := context.WithTimeout(h.ctx, 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := h.store.Stats(ctx)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
				t.Fatalf("stats cancellation=%v elapsed=%s", err, time.Since(start))
			}
		})
	}
	if _, err := h.store.Stats(h.ctx); err != nil {
		t.Fatalf("stats did not recover: %v", err)
	}
}

func TestPreflightPacketScopeFiltering(t *testing.T) {
	h := newAttemptHarness(t, "preflight-packet-scope", 1)
	restoreScopeFlags(t, h.store)
	packet, id := seedScopedPacketRow(t, h, 1)
	n, err := h.store.NextPreflightObservation(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = h.store.RecordPreflight(h.ctx, id, n, "fee_cap", "cap"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		sql     string
		visible bool
	}{
		{"paused pathway", `UPDATE pathways SET paused=true WHERE src_eid=40161 AND dst_eid=40449`, true},
		{"disabled pathway", `UPDATE pathways SET enabled=false WHERE src_eid=40161 AND dst_eid=40449`, false},
		{"restore pathway", `UPDATE pathways SET enabled=true,paused=false WHERE src_eid=40161 AND dst_eid=40449`, true},
		{"disabled source", `UPDATE chains SET enabled=false WHERE eid=40161`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.store.pool.Exec(h.ctx, tc.sql); err != nil {
				t.Fatal(err)
			}
			stats, err := h.store.txPreflightStats(h.ctx)
			if err != nil {
				t.Fatal(err)
			}
			seen := false
			for _, s := range stats {
				if s.SignerID == h.signerID && s.ChainEID == packet.DstEID {
					seen = true
				}
			}
			if seen != tc.visible {
				t.Fatalf("visible=%t", seen)
			}
		})
	}
}
