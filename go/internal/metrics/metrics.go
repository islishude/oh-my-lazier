package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// Server exposes worker health and metrics endpoints.
type Server struct {
	server *http.Server
	logger *slog.Logger
}

// New creates a metrics HTTP server.
func New(listenAddress string, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return &Server{
		server: &http.Server{Addr: listenAddress, Handler: mux},
		logger: logger,
	}
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
