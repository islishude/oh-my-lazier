package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/readiness"
)

// StatsProvider supplies read-only worker state for metrics rendering.
type StatsProvider interface {
	Stats(context.Context) (db.StatsSnapshot, error)
}

// RuntimeProvider supplies process-local metrics that are not stored in Postgres.
type RuntimeProvider interface {
	RuntimeSnapshot() RuntimeSnapshot
}

// RuntimeSnapshot is a process-local worker metrics snapshot.
type RuntimeSnapshot struct {
	Indexers []IndexerRuntimeStat
}

// IndexerRuntimeStat summarizes one in-process indexer loop.
type IndexerRuntimeStat struct {
	ChainEID                uint32
	ChainName               string
	PollSuccess             bool
	SuccessPolls            uint64
	ErrorPolls              uint64
	LastSuccessUnix         int64
	LastErrorUnix           int64
	LastPollDurationSeconds float64
	ObservedHeadBlock       uint64
	ConfirmedToBlock        uint64
	SourceTransactions      uint64
	DVNTransactions         uint64
	DestinationLogs         uint64
}

// Registry records process-local worker metrics.
type Registry struct {
	mu       sync.Mutex
	indexers map[indexerKey]*IndexerRuntimeStat
	now      func() time.Time
}

type indexerKey struct {
	chainEID  uint32
	chainName string
}

// NewRegistry creates an empty process-local metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		indexers: make(map[indexerKey]*IndexerRuntimeStat),
		now:      time.Now,
	}
}

// RecordIndexerPoll records one indexer polling attempt.
func (r *Registry) RecordIndexerPoll(chainEID uint32, chainName string, observedHeadBlock uint64, confirmedToBlock uint64, sourceTransactions int, dvnTransactions int, destinationLogs int, duration time.Duration, err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := indexerKey{chainEID: chainEID, chainName: chainName}
	stat := r.indexers[key]
	if stat == nil {
		stat = &IndexerRuntimeStat{ChainEID: chainEID, ChainName: chainName}
		r.indexers[key] = stat
	}
	stat.LastPollDurationSeconds = duration.Seconds()
	stat.ObservedHeadBlock = observedHeadBlock
	stat.ConfirmedToBlock = confirmedToBlock
	now := r.now().Unix()
	if err != nil {
		stat.PollSuccess = false
		stat.ErrorPolls++
		stat.LastErrorUnix = now
		return
	}
	stat.PollSuccess = true
	stat.SuccessPolls++
	stat.LastSuccessUnix = now
	stat.SourceTransactions += uint64(sourceTransactions)
	stat.DVNTransactions += uint64(dvnTransactions)
	stat.DestinationLogs += uint64(destinationLogs)
}

// RuntimeSnapshot returns a stable copy of process-local metrics.
func (r *Registry) RuntimeSnapshot() RuntimeSnapshot {
	if r == nil {
		return RuntimeSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	snapshot := RuntimeSnapshot{Indexers: make([]IndexerRuntimeStat, 0, len(r.indexers))}
	for _, stat := range r.indexers {
		snapshot.Indexers = append(snapshot.Indexers, *stat)
	}
	sort.Slice(snapshot.Indexers, func(a, b int) bool {
		if snapshot.Indexers[a].ChainEID != snapshot.Indexers[b].ChainEID {
			return snapshot.Indexers[a].ChainEID < snapshot.Indexers[b].ChainEID
		}
		return snapshot.Indexers[a].ChainName < snapshot.Indexers[b].ChainName
	})
	return snapshot
}

// Server exposes worker health and metrics endpoints.
type Server struct {
	server   *http.Server
	logger   *slog.Logger
	provider StatsProvider
}

// New creates a metrics HTTP server.
func New(listenAddress string, provider StatsProvider, logger *slog.Logger, runtime ...RuntimeProvider) *Server {
	handler := Handler(provider, runtime...)
	return &Server{
		server:   &http.Server{Addr: listenAddress, Handler: handler},
		logger:   logger,
		provider: provider,
	}
}

// Handler builds the health and metrics HTTP handler.
func Handler(provider StatsProvider, runtime ...RuntimeProvider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := provider.Stats(r.Context())
		if err != nil {
			http.Error(w, "not ready\n", http.StatusServiceUnavailable)
			return
		}
		report := readiness.Evaluate(snapshot)
		if !report.Ready {
			http.Error(w, "not ready\n", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := provider.Stats(r.Context())
		runtimeSnapshot := collectRuntime(runtime)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(renderPrometheus(snapshot, err == nil, runtimeSnapshot)))
	})
	return mux
}

// RenderPrometheus converts a stats snapshot to Prometheus text exposition format.
func RenderPrometheus(snapshot db.StatsSnapshot) string {
	return renderPrometheus(snapshot, true, RuntimeSnapshot{})
}

func renderPrometheus(snapshot db.StatsSnapshot, dbSnapshotAvailable bool, runtime RuntimeSnapshot) string {
	var output strings.Builder
	output.WriteString("# HELP laz_worker_info Static worker process information.\n")
	output.WriteString("# TYPE laz_worker_info gauge\n")
	output.WriteString("laz_worker_info 1\n")
	output.WriteString("# HELP laz_metrics_db_snapshot_available Whether the metrics endpoint could read the DB-backed stats snapshot.\n")
	output.WriteString("# TYPE laz_metrics_db_snapshot_available gauge\n")
	fmt.Fprintf(&output, "laz_metrics_db_snapshot_available %d\n", boolGauge(dbSnapshotAvailable))
	if dbSnapshotAvailable {
		renderDBMetrics(&output, snapshot)
	}
	renderRuntimeMetrics(&output, runtime)
	return output.String()
}

func renderDBMetrics(output *strings.Builder, snapshot db.StatsSnapshot) {
	output.WriteString("# HELP laz_chain_enabled Whether a configured chain is enabled.\n")
	output.WriteString("# TYPE laz_chain_enabled gauge\n")
	for _, stat := range snapshot.Chains {
		fmt.Fprintf(output, "laz_chain_enabled{eid=%q,name=%s} %d\n", strconv.FormatUint(uint64(stat.EID), 10), label(stat.Name), boolGauge(stat.Enabled))
	}
	output.WriteString("# HELP laz_chain_paused Whether a chain is paused by safety logic.\n")
	output.WriteString("# TYPE laz_chain_paused gauge\n")
	for _, stat := range snapshot.Chains {
		fmt.Fprintf(output, "laz_chain_paused{eid=%q,name=%s} %d\n", strconv.FormatUint(uint64(stat.EID), 10), label(stat.Name), boolGauge(stat.Paused))
	}
	output.WriteString("# HELP laz_pathway_paused Whether a configured pathway is paused by safety logic.\n")
	output.WriteString("# TYPE laz_pathway_paused gauge\n")
	for _, stat := range snapshot.Pathways {
		fmt.Fprintf(output, "laz_pathway_paused{src_eid=%q,dst_eid=%q} %d\n", uint32Label(stat.SrcEID), uint32Label(stat.DstEID), boolGauge(stat.Paused))
	}
	output.WriteString("# HELP laz_packets_total Packets by source, destination, and status.\n")
	output.WriteString("# TYPE laz_packets_total gauge\n")
	for _, stat := range snapshot.Packets {
		fmt.Fprintf(output, "laz_packets_total{src_eid=%q,dst_eid=%q,status=%s} %d\n", uint32Label(stat.SrcEID), uint32Label(stat.DstEID), label(stat.Status), stat.Count)
	}
	output.WriteString("# HELP laz_executor_jobs_total Executor jobs by status.\n")
	output.WriteString("# TYPE laz_executor_jobs_total gauge\n")
	for _, stat := range snapshot.ExecutorJobs {
		fmt.Fprintf(output, "laz_executor_jobs_total{status=%s} %d\n", label(stat.Status), stat.Count)
	}
	output.WriteString("# HELP laz_dvn_jobs_total DVN jobs by status.\n")
	output.WriteString("# TYPE laz_dvn_jobs_total gauge\n")
	for _, stat := range snapshot.DVNJobs {
		fmt.Fprintf(output, "laz_dvn_jobs_total{status=%s} %d\n", label(stat.Status), stat.Count)
	}
	output.WriteString("# HELP laz_tx_outbox_total Transaction outbox rows by chain and status.\n")
	output.WriteString("# TYPE laz_tx_outbox_total gauge\n")
	for _, stat := range snapshot.TxOutbox {
		fmt.Fprintf(output, "laz_tx_outbox_total{chain_eid=%q,status=%s} %d\n", uint32Label(stat.ChainEID), label(stat.Status), stat.Count)
	}
	output.WriteString("# HELP laz_indexer_cursor_last_block Last indexed block by chain and stream.\n")
	output.WriteString("# TYPE laz_indexer_cursor_last_block gauge\n")
	for _, stat := range snapshot.IndexerCursors {
		fmt.Fprintf(output, "laz_indexer_cursor_last_block{chain_eid=%q,stream=%s} %d\n", uint32Label(stat.ChainEID), label(stat.Stream), stat.LastBlock)
	}
}

func renderRuntimeMetrics(output *strings.Builder, snapshot RuntimeSnapshot) {
	output.WriteString("# HELP laz_indexer_poll_success Whether the most recent indexer poll succeeded.\n")
	output.WriteString("# TYPE laz_indexer_poll_success gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_poll_success{chain_eid=%q,name=%s} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), boolGauge(stat.PollSuccess))
	}
	output.WriteString("# HELP laz_indexer_polls_total Indexer polling attempts by result.\n")
	output.WriteString("# TYPE laz_indexer_polls_total counter\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_polls_total{chain_eid=%q,name=%s,result=\"success\"} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.SuccessPolls)
		fmt.Fprintf(output, "laz_indexer_polls_total{chain_eid=%q,name=%s,result=\"error\"} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.ErrorPolls)
	}
	output.WriteString("# HELP laz_indexer_last_success_timestamp_seconds Unix timestamp for the most recent successful indexer poll.\n")
	output.WriteString("# TYPE laz_indexer_last_success_timestamp_seconds gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_last_success_timestamp_seconds{chain_eid=%q,name=%s} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.LastSuccessUnix)
	}
	output.WriteString("# HELP laz_indexer_last_error_timestamp_seconds Unix timestamp for the most recent failed indexer poll.\n")
	output.WriteString("# TYPE laz_indexer_last_error_timestamp_seconds gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_last_error_timestamp_seconds{chain_eid=%q,name=%s} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.LastErrorUnix)
	}
	output.WriteString("# HELP laz_indexer_last_poll_duration_seconds Duration of the most recent indexer poll.\n")
	output.WriteString("# TYPE laz_indexer_last_poll_duration_seconds gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_last_poll_duration_seconds{chain_eid=%q,name=%s} %s\n", uint32Label(stat.ChainEID), label(stat.ChainName), floatGauge(stat.LastPollDurationSeconds))
	}
	output.WriteString("# HELP laz_indexer_observed_head_block Most recent chain head observed by an indexer poll.\n")
	output.WriteString("# TYPE laz_indexer_observed_head_block gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_observed_head_block{chain_eid=%q,name=%s} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.ObservedHeadBlock)
	}
	output.WriteString("# HELP laz_indexer_confirmed_to_block Most recent confirmed block upper bound used by an indexer poll.\n")
	output.WriteString("# TYPE laz_indexer_confirmed_to_block gauge\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_confirmed_to_block{chain_eid=%q,name=%s} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.ConfirmedToBlock)
	}
	output.WriteString("# HELP laz_indexer_processed_total Items processed by indexer polls.\n")
	output.WriteString("# TYPE laz_indexer_processed_total counter\n")
	for _, stat := range snapshot.Indexers {
		fmt.Fprintf(output, "laz_indexer_processed_total{chain_eid=%q,name=%s,kind=\"source_transactions\"} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.SourceTransactions)
		fmt.Fprintf(output, "laz_indexer_processed_total{chain_eid=%q,name=%s,kind=\"dvn_transactions\"} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.DVNTransactions)
		fmt.Fprintf(output, "laz_indexer_processed_total{chain_eid=%q,name=%s,kind=\"destination_logs\"} %d\n", uint32Label(stat.ChainEID), label(stat.ChainName), stat.DestinationLogs)
	}
}

func collectRuntime(providers []RuntimeProvider) RuntimeSnapshot {
	if len(providers) == 0 || providers[0] == nil {
		return RuntimeSnapshot{}
	}
	return providers[0].RuntimeSnapshot()
}

// Run serves metrics until the context is canceled or the listener fails.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("metrics server started", "addr", s.server.Addr)
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		_ = s.server.Shutdown(context.Background())
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func uint32Label(value uint32) string {
	return strconv.FormatUint(uint64(value), 10)
}

func boolGauge(value bool) int {
	if value {
		return 1
	}
	return 0
}

func floatGauge(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func label(value string) string {
	return strconv.Quote(value)
}
