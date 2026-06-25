package txmgr

import (
	"context"
	"log/slog"

	"github.com/islishude/oh-my-lazier/go/internal/db"
)

// Manager owns transaction outbox processing and nonce assignment.
type Manager struct {
	store  *db.Store
	logger *slog.Logger
}

// New creates a transaction manager using the shared store.
func New(store *db.Store, logger *slog.Logger) *Manager {
	return &Manager{store: store, logger: logger}
}

// Run starts the transaction manager loop until the context is canceled.
func (m *Manager) Run(ctx context.Context) error {
	m.logger.Info("tx manager loop started")
	<-ctx.Done()
	return ctx.Err()
}
