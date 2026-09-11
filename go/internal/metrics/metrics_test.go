package metrics

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/readiness"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
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
			// Removed from configuration while paused: the retained safety
			// state must not keep paging.
			{EID: 49999, Name: "retired", Enabled: false, Paused: true},
		},
		Pathways: []db.PathwayStat{
			{
				SrcEID: 40161, DstEID: 40449,
				SrcOApp: common.HexToAddress("0x1111111111111111111111111111111111111111"),
				DstOApp: common.HexToAddress("0x2222222222222222222222222222222222222222"),
				Enabled: true, Paused: true,
			},
			{
				SrcEID: 40161, DstEID: 40449,
				SrcOApp: common.HexToAddress("0x3333333333333333333333333333333333333333"),
				DstOApp: common.HexToAddress("0x4444444444444444444444444444444444444444"),
				Enabled: true,
			},
			{
				SrcEID: 40161, DstEID: 49999,
				SrcOApp: common.HexToAddress("0x5555555555555555555555555555555555555555"),
				DstOApp: common.HexToAddress("0x6666666666666666666666666666666666666666"),
				Enabled: false, Paused: true,
			},
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
		TxReceiptGasCosts: []db.TxReceiptGasCostStat{
			{ChainEID: 40449, Purpose: "executor_lz_receive", GasCostDstWei: "42000000000000"},
		},
		WorkerFees: []db.WorkerFeeStat{
			{
				Role:                "executor",
				SrcEID:              40161,
				DstEID:              40449,
				RevenueSrcWei:       "100",
				ActualGasCostSrcWei: "120",
				GrossMarginSrcWei:   "-20",
				NegativeMarginJobs:  1,
				UnpricedReceipts:    2,
			},
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
		`laz_chain_paused{eid="49999",name="retired"} 0`,
		`laz_pathway_paused{src_eid="40161",dst_eid="40449",src_oapp="0x1111111111111111111111111111111111111111",dst_oapp="0x2222222222222222222222222222222222222222"} 1`,
		`laz_pathway_paused{src_eid="40161",dst_eid="40449",src_oapp="0x3333333333333333333333333333333333333333",dst_oapp="0x4444444444444444444444444444444444444444"} 0`,
		`laz_pathway_paused{src_eid="40161",dst_eid="49999",src_oapp="0x5555555555555555555555555555555555555555",dst_oapp="0x6666666666666666666666666666666666666666"} 0`,
		`laz_packets_total{src_eid="40161",dst_eid="40449",status="MANUAL_REVIEW"} 2`,
		`laz_executor_jobs_total{status="LZ_RECEIVE_FAILED"} 1`,
		`laz_dvn_jobs_total{status="QUORUM_CONFLICT"} 1`,
		`laz_tx_outbox_total{chain_eid="40449",status="failed",retry_state="exhausted"} 3`,
		`laz_tx_receipt_gas_cost_dst_wei{chain_eid="40449",purpose="executor_lz_receive"} 42000000000000`,
		`laz_worker_fee_revenue_src_wei{role="executor",src_eid="40161",dst_eid="40449"} 100`,
		`laz_worker_fee_actual_gas_cost_src_wei{role="executor",src_eid="40161",dst_eid="40449"} 120`,
		`laz_worker_fee_gross_margin_src_wei{role="executor",src_eid="40161",dst_eid="40449"} -20`,
		`laz_worker_fee_negative_margin_jobs{role="executor",src_eid="40161",dst_eid="40449"} 1`,
		`laz_worker_fee_unpriced_receipts{role="executor",src_eid="40161",dst_eid="40449"} 2`,
		`laz_indexer_cursor_last_block{chain_eid="40161",stream="source"} 123456`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	assertUniquePrometheusSeries(t, body)
}

func assertUniquePrometheusSeries(t *testing.T, body string) {
	t.Helper()
	seen := make(map[string]struct{})
	for line := range strings.Lines(body) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.LastIndexByte(line, ' ')
		if separator < 0 {
			t.Fatalf("invalid Prometheus sample line %q", line)
		}
		identity := line[:separator]
		if _, exists := seen[identity]; exists {
			t.Fatalf("duplicate Prometheus series %q in body:\n%s", identity, body)
		}
		seen[identity] = struct{}{}
	}
}

func TestHandlerMetricsRendersRegisteredIndexerBeforeFirstPoll(t *testing.T) {
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	registry.RegisterIndexer(40161, "ethereum-sepolia", "executor_source", 30*time.Minute)
	handler := Handler(fakeProvider{err: errors.New("database down")}, registry)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := recorder.Body.String()
	for _, want := range []string{
		`laz_indexer_poll_interval_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1800.000000`,
		`laz_indexer_start_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1700000000`,
		`laz_indexer_last_poll_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 0`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",result="success"} 0`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",result="error"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q before first poll:\n%s", want, body)
		}
	}
}

func TestHandlerMetricsRendersRuntimeMetricsWhenStatsUnavailable(t *testing.T) {
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Unix(1_699_999_990, 0) }
	registry.RegisterIndexer(40161, "ethereum-sepolia", "executor_source", 5*time.Second)
	registry.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 5*time.Second, 200, 188, 2, 1, 3, 1500*time.Millisecond, nil)
	registry.now = func() time.Time { return time.Unix(1_700_000_030, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 5*time.Second, 0, 0, 0, 0, 0, 250*time.Millisecond, errors.New("rpc unavailable"))
	registry.now = func() time.Time { return time.Unix(1_700_000_040, 0) }
	registry.RecordLoopRetry("txmgr")
	registry.now = func() time.Time { return time.Unix(1_700_000_050, 0) }
	registry.RecordLoopRetry("txmgr")
	registry.now = func() time.Time { return time.Unix(1_700_000_060, 0) }
	registry.RecordLoopRetry("pricing")
	registry.now = func() time.Time { return time.Unix(1_700_000_070, 0) }
	registry.RecordSignerBalance(40161, "0x9999999999999999999999999999999999999999", big.NewInt(900_000_000_000_000_000), big.NewInt(1_000_000_000_000_000_000), 75*time.Millisecond, nil)
	registry.now = func() time.Time { return time.Unix(1_700_000_080, 0) }
	registry.RecordSignerBalance(40449, "0x8888888888888888888888888888888888888888", nil, big.NewInt(1_000_000_000_000_000_000), 125*time.Millisecond, errors.New("balance rpc unavailable"))
	registry.RecordRPCProviders(40161, "ethereum-sepolia", []rpcquorum.Provider{
		{ID: "provider-0", Status: rpcquorum.ProviderHealthy},
		{ID: "provider-1", Status: rpcquorum.ProviderConflict, LogConflict: true, SafeConflict: true},
	})
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
		`laz_indexer_poll_success{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 0`,
		`laz_indexer_poll_interval_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 5.000000`,
		`laz_indexer_start_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1699999990`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",result="success"} 1`,
		`laz_indexer_polls_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",result="error"} 1`,
		`laz_indexer_last_poll_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1700000030`,
		`laz_indexer_last_success_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1700000000`,
		`laz_indexer_failure_since_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1700000030`,
		`laz_indexer_last_error_timestamp_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 1700000030`,
		`laz_indexer_last_poll_duration_seconds{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 0.250000`,
		`laz_indexer_safe_to_block{chain_eid="40161",name="ethereum-sepolia",stream="executor_source"} 0`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",kind="source_transactions"} 2`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",kind="dvn_transactions"} 1`,
		`laz_indexer_processed_total{chain_eid="40161",name="ethereum-sepolia",stream="executor_source",kind="destination_logs"} 3`,
		`laz_signer_native_balance_wei{chain_eid="40161",signer="0x9999999999999999999999999999999999999999"} 900000000000000000`,
		`laz_signer_min_native_balance_wei{chain_eid="40161",signer="0x9999999999999999999999999999999999999999"} 1000000000000000000`,
		`laz_signer_min_native_balance_wei{chain_eid="40449",signer="0x8888888888888888888888888888888888888888"} 1000000000000000000`,
		`laz_signer_balance_poll_success{chain_eid="40161",signer="0x9999999999999999999999999999999999999999"} 1`,
		`laz_signer_balance_poll_success{chain_eid="40449",signer="0x8888888888888888888888888888888888888888"} 0`,
		`laz_signer_balance_last_success_timestamp_seconds{chain_eid="40161",signer="0x9999999999999999999999999999999999999999"} 1700000070`,
		`laz_signer_balance_last_error_timestamp_seconds{chain_eid="40449",signer="0x8888888888888888888888888888888888888888"} 1700000080`,
		`laz_signer_balance_last_poll_duration_seconds{chain_eid="40449",signer="0x8888888888888888888888888888888888888888"} 0.125000`,
		`laz_rpc_provider_status{chain_eid="40161",provider="provider-0",status="healthy"} 1`,
		`laz_rpc_provider_status{chain_eid="40161",provider="provider-1",status="conflict"} 1`,
		`laz_rpc_provider_log_conflict{chain_eid="40161",provider="provider-0"} 0`,
		`laz_rpc_provider_log_conflict{chain_eid="40161",provider="provider-1"} 1`,
		`laz_rpc_provider_safe_conflict{chain_eid="40161",provider="provider-0"} 0`,
		`laz_rpc_provider_safe_conflict{chain_eid="40161",provider="provider-1"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "database down") || strings.Contains(body, "rpc unavailable") || strings.Contains(body, "balance rpc unavailable") {
		t.Fatalf("metrics body exposes raw error text:\n%s", body)
	}
	if strings.Contains(body, "laz_indexer_confirmed_to_block") {
		t.Fatalf("metrics body still exposes retired confirmed-to metric:\n%s", body)
	}
}

func TestRegistryTracksIndexerRegistrationAndFailureWindow(t *testing.T) {
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Unix(100, 0) }
	registry.RegisterIndexer(40161, "ethereum-sepolia", "executor_source", 30*time.Minute)

	stat := onlyIndexerRuntimeStat(t, registry.RuntimeSnapshot())
	if stat.StartedUnix != 100 || stat.PollIntervalSeconds != 1800 {
		t.Fatalf("registered indexer stat = %#v", stat)
	}
	if stat.LastPollUnix != 0 || stat.FailureSinceUnix != 0 {
		t.Fatalf("registered indexer has poll outcome state = %#v", stat)
	}

	registry.now = func() time.Time { return time.Unix(110, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 30*time.Minute, 0, 0, 0, 0, 0, time.Second, errors.New("first failure"))
	registry.now = func() time.Time { return time.Unix(1_910, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 30*time.Minute, 0, 0, 0, 0, 0, time.Second, errors.New("second failure"))

	stat = onlyIndexerRuntimeStat(t, registry.RuntimeSnapshot())
	if stat.LastPollUnix != 1_910 || stat.FailureSinceUnix != 110 || stat.ErrorPolls != 2 {
		t.Fatalf("failed indexer stat = %#v", stat)
	}

	registry.now = func() time.Time { return time.Unix(3_710, 0) }
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 30*time.Minute, 200, 188, 0, 0, 0, time.Second, nil)
	registry.now = func() time.Time { return time.Unix(3_720, 0) }
	registry.RegisterIndexer(40161, "ethereum-sepolia", "executor_source", 30*time.Minute)

	stat = onlyIndexerRuntimeStat(t, registry.RuntimeSnapshot())
	if stat.StartedUnix != 100 || stat.LastPollUnix != 3_710 || stat.LastSuccessUnix != 3_710 {
		t.Fatalf("recovered indexer stat = %#v", stat)
	}
	if stat.FailureSinceUnix != 0 || !stat.PollSuccess {
		t.Fatalf("recovered indexer retains failure state = %#v", stat)
	}
}

func TestRegistryKeepsIndexerStreamsIndependent(t *testing.T) {
	registry := NewRegistry()
	registry.now = func() time.Time { return time.Unix(100, 0) }
	registry.RegisterIndexer(40161, "ethereum-sepolia", "executor_source", 5*time.Second)
	registry.RegisterIndexer(40161, "ethereum-sepolia", "dvn_source", 5*time.Second)
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "executor_source", 5*time.Second, 200, 188, 2, 0, 0, time.Second, nil)
	registry.RecordIndexerPoll(40161, "ethereum-sepolia", "dvn_source", 5*time.Second, 200, 188, 0, 1, 0, time.Second, errors.New("log conflict after a completed chunk"))

	snapshot := registry.RuntimeSnapshot()
	if len(snapshot.Indexers) != 2 {
		t.Fatalf("indexer stats = %#v, want two streams", snapshot.Indexers)
	}
	byStream := make(map[string]IndexerRuntimeStat, len(snapshot.Indexers))
	for _, stat := range snapshot.Indexers {
		byStream[stat.Stream] = stat
	}
	if !byStream["executor_source"].PollSuccess || byStream["executor_source"].SourceTransactions != 2 {
		t.Fatalf("executor source stat = %#v, want independent success", byStream["executor_source"])
	}
	if byStream["dvn_source"].PollSuccess || byStream["dvn_source"].ErrorPolls != 1 || byStream["dvn_source"].DVNTransactions != 1 {
		t.Fatalf("dvn source stat = %#v, want independent failure with durable partial count", byStream["dvn_source"])
	}
}

func onlyIndexerRuntimeStat(t *testing.T, snapshot RuntimeSnapshot) IndexerRuntimeStat {
	t.Helper()
	if len(snapshot.Indexers) != 1 {
		t.Fatalf("indexer stats = %#v, want one", snapshot.Indexers)
	}
	return snapshot.Indexers[0]
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
			{
				SrcEID: 40161, DstEID: 40449,
				SrcOApp: common.HexToAddress("0x7777777777777777777777777777777777777777"),
				DstOApp: common.HexToAddress("0x8888888888888888888888888888888888888888"),
				Enabled: true,
			},
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

func TestRecoveryMetricIdentity(t *testing.T) {
	var output strings.Builder
	renderRecoveryMetrics(&output, []db.RecoveryStat{{ChainEID: 1, SignerID: "a", Inflight: 8, Window: 8, Reason: "fee_cap"}, {ChainEID: 1, SignerID: "b", Inflight: 2, Window: 8}})
	seen := map[string]bool{}
	for line := range strings.SplitSeq(output.String(), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, _, _ := strings.Cut(line, "} ")
		if seen[key] {
			t.Fatalf("duplicate series: %s", key)
		}
		seen[key] = true
	}
	if strings.Contains(output.String(), "guid=") || strings.Contains(output.String(), "tx_hash=") {
		t.Fatal("unbounded labels")
	}
}

func TestPricingSourceFailuresAndSnapshotAgeDuringOutage(t *testing.T) {
	registry := NewRegistry()
	now := time.Unix(1_700_000_000, 0)
	registry.now = func() time.Time { return now }
	registry.RecordPricingSnapshot(1, 2, common.Address{}, now, time.Hour)
	registry.RecordPricingSourceFailure(1, "coingecko", "primary", "stale")
	registry.RecordPricingSourceFailure(1, "coingecko", "primary", "stale")
	for _, elapsed := range []time.Duration{0, 15 * time.Minute} {
		now = time.Unix(1_700_000_000, 0).Add(elapsed)
		snapshot := registry.RuntimeSnapshot()
		if len(snapshot.PricingSourceFailures) != 1 || snapshot.PricingSourceFailures[0].Count != 2 {
			t.Fatalf("failure stats=%v", snapshot.PricingSourceFailures)
		}
		if snapshot.PricingSnapshots[0].AgeSeconds != elapsed.Seconds() {
			t.Fatalf("age=%v", snapshot.PricingSnapshots[0])
		}
		output := renderPrometheus(db.StatsSnapshot{}, false, snapshot)
		if !strings.Contains(output, `laz_pricing_source_failures_total{eid="1",source="coingecko",role="primary",category="stale"} 2`) {
			t.Fatalf("missing counter: %s", output)
		}
	}
}
