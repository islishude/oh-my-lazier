package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/db"
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

func TestHandlerMetricsRendersPrometheusSnapshot(t *testing.T) {
	handler := Handler(fakeProvider{snapshot: db.StatsSnapshot{
		Chains: []db.ChainStat{
			{EID: 40161, Name: "ethereum-sepolia", Enabled: true},
			{EID: 40245, Name: "base-sepolia", Enabled: true, Paused: true},
		},
		Pathways: []db.PathwayStat{
			{SrcEID: 40161, DstEID: 40245, Enabled: true, Paused: true},
		},
		Packets: []db.PacketStat{
			{SrcEID: 40161, DstEID: 40245, Status: "MANUAL_REVIEW", Count: 2},
		},
		ExecutorJobs: []db.StatusStat{
			{Status: "LZ_RECEIVE_FAILED", Count: 1},
		},
		DVNJobs: []db.StatusStat{
			{Status: "QUORUM_CONFLICT", Count: 1},
		},
		TxOutbox: []db.TxOutboxStat{
			{ChainEID: 40245, Status: "failed", Count: 3},
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
		`laz_chain_paused{eid="40245",name="base-sepolia"} 1`,
		`laz_pathway_paused{src_eid="40161",dst_eid="40245"} 1`,
		`laz_packets_total{src_eid="40161",dst_eid="40245",status="MANUAL_REVIEW"} 2`,
		`laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} 1`,
		`laz_dvn_jobs_total{status="QUORUM_CONFLICT"} 1`,
		`laz_tx_outbox_total{chain_eid="40245",status="failed"} 3`,
		`laz_indexer_cursor_last_block{chain_eid="40161",stream="source"} 123456`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

type fakeProvider struct {
	snapshot db.StatsSnapshot
	err      error
}

func (p fakeProvider) Stats(context.Context) (db.StatsSnapshot, error) {
	return p.snapshot, p.err
}
