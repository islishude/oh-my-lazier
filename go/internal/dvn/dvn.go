package dvn

import (
	"context"
	"log/slog"
)

// Mode selects whether the DVN verifier only reports or also submits verification transactions.
type Mode string

const (
	// ModeShadow verifies and reports what the DVN would submit without sending transactions.
	ModeShadow Mode = "shadow"
	// ModeActive verifies and enqueues active DVN verification transactions.
	ModeActive Mode = "active"
)

// Worker runs the DVN verification workflow.
type Worker struct {
	mode   Mode
	logger *slog.Logger
}

// New creates a DVN worker for the configured mode.
func New(mode string, logger *slog.Logger) *Worker {
	return &Worker{mode: Mode(mode), logger: logger}
}

// Run starts the DVN verifier loop until the context is canceled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("dvn verifier loop started", "mode", w.mode)
	<-ctx.Done()
	return ctx.Err()
}
