package executor

import (
	"context"
	"log/slog"
)

// Worker runs executor commit and delivery workflows.
type Worker struct {
	logger *slog.Logger
}

// New creates an executor worker.
func New(logger *slog.Logger) *Worker {
	return &Worker{logger: logger}
}

// RunCommitter starts the commitVerification enqueue loop.
func (w *Worker) RunCommitter(ctx context.Context) error {
	w.logger.Info("executor committer loop started")
	<-ctx.Done()
	return ctx.Err()
}

// RunDeliverer starts the lzReceive delivery loop.
func (w *Worker) RunDeliverer(ctx context.Context) error {
	w.logger.Info("executor deliverer loop started")
	<-ctx.Done()
	return ctx.Err()
}
