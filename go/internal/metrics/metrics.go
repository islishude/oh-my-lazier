package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/islishude/oh-my-lazier/go/internal/db"
)

// StatsProvider supplies read-only worker state for metrics rendering.
type StatsProvider interface {
	Stats(context.Context) (db.StatsSnapshot, error)
}

// Server exposes worker health and metrics endpoints.
type Server struct {
	server   *http.Server
	logger   *slog.Logger
	provider StatsProvider
}

// New creates a metrics HTTP server.
func New(listenAddress string, provider StatsProvider, logger *slog.Logger) *Server {
	handler := Handler(provider)
	return &Server{
		server:   &http.Server{Addr: listenAddress, Handler: handler},
		logger:   logger,
		provider: provider,
	}
}

// Handler builds the health and metrics HTTP handler.
func Handler(provider StatsProvider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := provider.Stats(r.Context()); err != nil {
			http.Error(w, "not ready\n", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := provider.Stats(r.Context())
		if err != nil {
			http.Error(w, "metrics unavailable\n", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(RenderPrometheus(snapshot)))
	})
	return mux
}

// RenderPrometheus converts a stats snapshot to Prometheus text exposition format.
func RenderPrometheus(snapshot db.StatsSnapshot) string {
	var output strings.Builder
	output.WriteString("# HELP laz_worker_info Static worker process information.\n")
	output.WriteString("# TYPE laz_worker_info gauge\n")
	output.WriteString("laz_worker_info 1\n")
	output.WriteString("# HELP laz_chain_enabled Whether a configured chain is enabled.\n")
	output.WriteString("# TYPE laz_chain_enabled gauge\n")
	for _, stat := range snapshot.Chains {
		output.WriteString(fmt.Sprintf("laz_chain_enabled{eid=%q,name=%s} %d\n", strconv.FormatUint(uint64(stat.EID), 10), label(stat.Name), boolGauge(stat.Enabled)))
	}
	output.WriteString("# HELP laz_chain_paused Whether a chain is paused by safety logic.\n")
	output.WriteString("# TYPE laz_chain_paused gauge\n")
	for _, stat := range snapshot.Chains {
		output.WriteString(fmt.Sprintf("laz_chain_paused{eid=%q,name=%s} %d\n", strconv.FormatUint(uint64(stat.EID), 10), label(stat.Name), boolGauge(stat.Paused)))
	}
	output.WriteString("# HELP laz_pathway_paused Whether a configured pathway is paused by safety logic.\n")
	output.WriteString("# TYPE laz_pathway_paused gauge\n")
	for _, stat := range snapshot.Pathways {
		output.WriteString(fmt.Sprintf("laz_pathway_paused{src_eid=%q,dst_eid=%q} %d\n", uint32Label(stat.SrcEID), uint32Label(stat.DstEID), boolGauge(stat.Paused)))
	}
	output.WriteString("# HELP laz_packets_total Packets by source, destination, and status.\n")
	output.WriteString("# TYPE laz_packets_total gauge\n")
	for _, stat := range snapshot.Packets {
		output.WriteString(fmt.Sprintf("laz_packets_total{src_eid=%q,dst_eid=%q,status=%s} %d\n", uint32Label(stat.SrcEID), uint32Label(stat.DstEID), label(stat.Status), stat.Count))
	}
	output.WriteString("# HELP laz_executor_jobs_total Executor jobs by status.\n")
	output.WriteString("# TYPE laz_executor_jobs_total gauge\n")
	for _, stat := range snapshot.ExecutorJobs {
		output.WriteString(fmt.Sprintf("laz_executor_jobs_total{status=%s} %d\n", label(stat.Status), stat.Count))
	}
	output.WriteString("# HELP laz_dvn_jobs_total DVN jobs by status.\n")
	output.WriteString("# TYPE laz_dvn_jobs_total gauge\n")
	for _, stat := range snapshot.DVNJobs {
		output.WriteString(fmt.Sprintf("laz_dvn_jobs_total{status=%s} %d\n", label(stat.Status), stat.Count))
	}
	output.WriteString("# HELP laz_tx_outbox_total Transaction outbox rows by chain and status.\n")
	output.WriteString("# TYPE laz_tx_outbox_total gauge\n")
	for _, stat := range snapshot.TxOutbox {
		output.WriteString(fmt.Sprintf("laz_tx_outbox_total{chain_eid=%q,status=%s} %d\n", uint32Label(stat.ChainEID), label(stat.Status), stat.Count))
	}
	output.WriteString("# HELP laz_indexer_cursor_last_block Last indexed block by chain and stream.\n")
	output.WriteString("# TYPE laz_indexer_cursor_last_block gauge\n")
	for _, stat := range snapshot.IndexerCursors {
		output.WriteString(fmt.Sprintf("laz_indexer_cursor_last_block{chain_eid=%q,stream=%s} %d\n", uint32Label(stat.ChainEID), label(stat.Stream), stat.LastBlock))
	}
	return output.String()
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

func label(value string) string {
	return strconv.Quote(value)
}
