package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/readiness"
)

func TestHandlerHealthDoesNotRequireStats(t *testing.T) {
	handler := Handler(fakeProvider{err: errors.New("database down")})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "ok\n" {
		t.Fatalf("body = %q, want ok", recorder.Body.String())
	}
}

func TestHandlerReadyReportsStatsFailure(t *testing.T) {
	handler := Handler(fakeProvider{err: errors.New("database down")})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestHandlerReadyReportsReadinessFailure(t *testing.T) {
	handler := Handler(fakeProvider{snapshot: cleanSnapshotWith(func(snapshot *db.StatsSnapshot) {
		snapshot.DVNJobs = []db.StatusStat{{Status: string(packets.DVNQuorumConflict), Count: 1}}
	})})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestHandlerReadyAcceptsCleanSnapshot(t *testing.T) {
	handler := Handler(fakeProvider{snapshot: cleanSnapshotWith(nil)})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "ready\n" {
		t.Fatalf("body = %q, want ready", recorder.Body.String())
	}
}

func TestHandlerReadyUsesRoleAwareReadiness(t *testing.T) {
	snapshot := cleanSnapshotWith(func(snapshot *db.StatsSnapshot) {
		snapshot.IndexerCursors = []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: "executor_source", LastBlock: 100},
			{ChainEID: 40449, Stream: "executor_destination", LastBlock: 100},
		}
		snapshot.DVNJobs = []db.StatusStat{{Status: string(packets.DVNQuorumConflict), Count: 1}}
	})
	handler := HandlerWithReadiness(fakeProvider{snapshot: snapshot}, readiness.Services{ExecutorEnabled: true})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestHandlerMetricsRendersPrometheusSnapshot(t *testing.T) {
	handler := Handler(fakeProvider{snapshot: db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40449, Name: "hoodi", Enabled: true, Paused: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40449, Enabled: true, Paused: true},
		},
		Packets: []db.PacketStat{
			{SrcEID: 40161, DstEID: 40449, Status: "MANUAL_REVIEW", Count: 2},
		},
		ExecutorJobs: []db.StatusStat{
			{Status: "LZ_RECEIVE_FAILED", Count: 1},
		},
		DVNJobs: []db.StatusStat{
			{Status: "QUORUM_CONFLICT", Count: 1},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40449, Status: "failed", RetryState: db.TxOutboxRetryStateExhausted, Count: 3},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: "source", LastBlock: 123456},
		},
	}})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`laz_worker_info 1`,
		`laz_metrics_db_snapshot_available 1`,
		`laz_chain_paused{eid="40449",name="hoodi"} 1`,
		`laz_pathway_paused{src_eid="40161",dst_eid="40449"} 1`,
		`laz_packets_total{src_eid="40161",dst_eid="40449",status="MANUAL_REVIEW"} 2`,
		`laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} 1`,
		`laz_dvn_jobs_total{status="QUORUM_CONFLICT"} 1`,
		`laz_tx_outbox_total{chain_eid="40449",status="failed",retry_state="exhausted"} 3`,
		`laz_indexer_cursor_last_block{chain_eid="40161",stream="source"} 123456`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

func TestHandlerMetricsRendersRuntimeMetricsWhenStatsUnavailable(t *testing.T) {
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", 200, 188, 2, 1, 3, 1500*time.Millisecond, nil)
	registry.now = func() time.Time { return time.Unix(1_700_000_030, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", 0, 0, 0, 0, 0, 250*time.Millisecond, errors.New("rpc unavailable"))
	registry.now = func() time.Time { return time.Unix(1_700_000_040, 0) }
	registry.RecordLoopRetry("txmgr")
	registry.now = func() time.Time { return time.Unix(1_700_000_050, 0) }
	registry.RecordLoopRetry("txmgr")
	registry.now = func() time.Time { return time.Unix(1_700_000_060, 0) }
	registry.RecordLoopRetry("pricing")
	handler := Handler(fakeProvider{err: errors.New("database down")}, registry)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`laz_worker_info 1`,
		`laz_metrics_db_snapshot_available 0`,
		`laz_worker_loop_retries_total{name="pricing"} 1`,
		`laz_worker_loop_retries_total{name="txmgr"} 2`,
		`laz_worker_loop_last_retry_timestamp_seconds{name="pricing"} 1700000060`,
		`laz_worker_loop_last_retry_timestamp_seconds{name="txmgr"} 1700000050`,
		`laz_indexer_poll_success{chain_eid="40161",name="ethereum-sepolia"} 0`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",result="success"} 1`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",result="error"} 1`,
		`laz_indexer_last_success_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia"} 1700000000`,
		`laz_indexer_last_error_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia"} 1700000030`,
		`laz_indexer_last_poll_duration_seconds{chain_eid="40161",name="ethereum-sepolia"} 0.250000`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",kind="source_transactions"} 2`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",kind="dvn_transactions"} 1`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",kind="destination_logs"} 3`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "database down") || strings.Contains(body, "rpc unavailable") {
		t.Fatalf("metrics body exposes raw error text:\n%s", body)
	}
}

type fakeProvider struct {
	snapshot db.StatsSnapshot
	err      error
}

func (p fakeProvider) Stats(context.Context) (db.StatsSnapshot, error) {
	return p.snapshot, p.err
}

func cleanSnapshotWith(mutator func(*db.StatsSnapshot)) db.StatsSnapshot {
	snapshot := db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40449, Name: "hoodi", Enabled: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40449, Enabled: true},
		},
		IndexerCursors: []db.IndexerCursorStat{
			{ChainEID: 40161, Stream: "executor_source", LastBlock: 100},
			{ChainEID: 40449, Stream: "executor_destination", LastBlock: 100},
			{ChainEID: 40161, Stream: "dvn_source", LastBlock: 100},
			{ChainEID: 40449, Stream: "dvn_destination", LastBlock: 100},
		},
	}
	if mutator != nil {
		mutator(&snapshot)
	}
	return snapshot
}
