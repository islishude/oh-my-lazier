package txmgr

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/signer"
)

const defaultPollInterval = 5 * time.Second

// Target binds one configured chain RPC client to the signer that should consume its tx_outbox rows.
type Target struct {
	ChainEID            uint32
	ChainID             *big.Int
	Signer              signer.Signer
	Client              ChainClient
	FeePolicies         map[string]FeePolicy
	MinNativeBalanceWei *big.Int
}

// Manager owns transaction outbox processing and nonce assignment.
type Manager struct {
	store        *db.Store
	targets      []Target
	pollInterval time.Duration
	logger       *slog.Logger
}

// New creates a transaction manager using the shared store.
func New(store *db.Store, logger *slog.Logger) *Manager {
	return NewWithTargets(store, nil, logger)
}

// NewWithTargets creates a transaction manager with configured chain/signing targets.
func NewWithTargets(store *db.Store, targets []Target, logger *slog.Logger) *Manager {
	copiedTargets := make([]Target, len(targets))
	copy(copiedTargets, targets)
	manager := &Manager{
		store:        store,
		targets:      copiedTargets,
		pollInterval: defaultPollInterval,
		logger:       logger,
	}
	return manager
}

// Run starts the transaction manager loop until the context is canceled.
func (m *Manager) Run(ctx context.Context) error {
	return m.runLoop(ctx, m.processOnce)
}

func (m *Manager) runLoop(ctx context.Context, processOnce func(context.Context) (bool, error)) error {
	m.logger.Info("tx manager loop started", "targets", len(m.targets))
	for {
		processed, err := processOnce(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		timer := time.NewTimer(m.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) processOnce(ctx context.Context) (bool, error) {
	processed := false
	for _, target := range m.targets {
		id, err := m.ProcessReceipts(ctx, target, 1)
		if errors.Is(err, ErrNoReceiptUpdate) {
			// No mined receipt yet; queued work may still be available.
		} else if err != nil {
			return processed, err
		} else {
			processed = true
			m.logger.Info("processed tx receipt", "id", id, "chain_eid", target.ChainEID, "signer", target.Signer.Address())
			continue
		}
		id, err = m.ProcessStaleBroadcastReplacement(ctx, target)
		if errors.Is(err, db.ErrNoStaleBroadcastReplacement) {
			// No stale pending broadcast; failed retries may still be due.
		} else if errors.Is(err, ErrTxDeferred) {
			// Replacement fee would exceed configured caps; keep polling the original tx hash.
			continue
		} else if err != nil {
			return processed, err
		} else {
			processed = true
			m.logger.Info("processed stale broadcast tx replacement", "id", id, "chain_eid", target.ChainEID, "signer", target.Signer.Address())
			continue
		}
		id, err = m.ProcessFailedRetry(ctx, target)
		if errors.Is(err, db.ErrNoFailedTxRetry) {
			// No due failed retry; queued work may still be available.
		} else if err != nil {
			return processed, err
		} else {
			processed = true
			m.logger.Info("requeued failed tx outbox row", "id", id, "chain_eid", target.ChainEID, "signer", target.Signer.Address())
			continue
		}
		id, err = m.ProcessNext(ctx, target)
		if errors.Is(err, ErrNoQueuedTx) {
			continue
		}
		if errors.Is(err, ErrTxDeferred) {
			continue
		}
		if err != nil {
			return processed, err
		}
		processed = true
		m.logger.Info("processed tx outbox row", "id", id, "chain_eid", target.ChainEID, "signer", target.Signer.Address())
	}
	return processed, nil
}
