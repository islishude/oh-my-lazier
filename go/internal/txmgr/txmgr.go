package txmgr

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/signer"
)

const (
	defaultPollInterval = 5 * time.Second

	// DefaultStaleBroadcastReplacementAfter is the production default before same-nonce replacement.
	DefaultStaleBroadcastReplacementAfter = 15 * time.Minute
	// DefaultPreSignRPCTimeout bounds the estimate-gas and fee-quote preflight for one attempt.
	DefaultPreSignRPCTimeout = 30 * time.Second
	// DefaultSignTimeout bounds one KMS or keystore SignTx call so a hung signer
	// backend cannot outlive the signing lease.
	DefaultSignTimeout = 30 * time.Second
	// DefaultSigningLeaseTTL covers the preflight, signing, and the attempt insert.
	DefaultSigningLeaseTTL = 90 * time.Second
	// DefaultSendTimeout bounds one SendTransaction call; the broadcast lease lets
	// another instance replay the persisted raw if this one hangs or dies.
	DefaultSendTimeout = 15 * time.Second
	// DefaultBroadcastLeaseTTL is how long a claimed attempt stays reserved for one sender.
	DefaultBroadcastLeaseTTL = 45 * time.Second
	// DefaultNonceReconcileInterval is the backoff between confirmed-nonce
	// reconciliation passes for a signer lane with held rows.
	DefaultNonceReconcileInterval = time.Minute
)

// Options controls tx manager runtime behavior.
type Options struct {
	// Now is the recovery clock; defaults to time.Now.
	Now func() time.Time
	// MaxInflightPerSigner bounds new nonce assignment.
	MaxInflightPerSigner int
	// StaleBroadcastReplacementAfter is how long a broadcast row can lack a receipt before same-nonce replacement.
	StaleBroadcastReplacementAfter time.Duration
	// PreSignRPCTimeout bounds the estimate-gas and fee-quote preflight for one attempt.
	PreSignRPCTimeout time.Duration
	// SignTimeout bounds one SignTx call.
	SignTimeout time.Duration
	// SigningLeaseTTL is the outbox signing lease duration; it must cover the
	// preflight, the signing call, and the durable attempt insert.
	SigningLeaseTTL time.Duration
	// SendTimeout bounds one SendTransaction call.
	SendTimeout time.Duration
	// BroadcastLeaseTTL is the attempt broadcast lease duration.
	BroadcastLeaseTTL time.Duration
	// NonceReconcileInterval is the backoff between confirmed-nonce
	// reconciliation passes for one signer lane.
	NonceReconcileInterval time.Duration
}

// Target binds one configured chain RPC client to the signer that should consume its tx_outbox rows.
type Target struct {
	ChainEID uint32
	// ChainName identifies the configured chain in transaction-manager logs and
	// provider status metrics reported through the balance monitor.
	ChainName string
	ChainID   *big.Int
	Signer    signer.Signer
	Client    ChainClient
	// Confirmations is the number of blocks a receipt must be buried under
	// before its terminal workflow state is applied, so a short reorg cannot
	// leave the database terminal for a transaction the chain rolled back. The
	// indexer independently uses the quorum safe block. Zero disables this gate.
	Confirmations       uint64
	FeePolicies         map[string]FeePolicy
	MinNativeBalanceWei *big.Int
}

// Manager owns transaction outbox processing and nonce assignment.
type Manager struct {
	store        *db.Store
	targets      []Target
	pollInterval time.Duration
	options      Options
	logger       *slog.Logger
}

// New creates a transaction manager using the shared store.
func New(store *db.Store, logger *slog.Logger) *Manager {
	return NewWithTargets(store, nil, logger)
}

// NewWithOptions creates a transaction manager with runtime options.
func NewWithOptions(store *db.Store, logger *slog.Logger, options Options) *Manager {
	return NewWithTargetsAndOptions(store, nil, logger, options)
}

// NewWithTargets creates a transaction manager with configured chain/signing targets.
func NewWithTargets(store *db.Store, targets []Target, logger *slog.Logger) *Manager {
	return NewWithTargetsAndOptions(store, targets, logger, Options{})
}

// NewWithTargetsAndOptions creates a transaction manager with configured targets and runtime options.
func NewWithTargetsAndOptions(store *db.Store, targets []Target, logger *slog.Logger, options Options) *Manager {
	copiedTargets := make([]Target, len(targets))
	copy(copiedTargets, targets)
	manager := &Manager{
		store:        store,
		targets:      copiedTargets,
		pollInterval: defaultPollInterval,
		options:      normalizeOptions(options),
		logger:       logger,
	}
	if store != nil {
		store.SetMaxInflight(manager.options.MaxInflightPerSigner)
	}
	return manager
}

func normalizeOptions(options Options) Options {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaxInflightPerSigner <= 0 {
		options.MaxInflightPerSigner = config.DefaultMaxInflightPerSigner
	}
	if options.StaleBroadcastReplacementAfter <= 0 {
		options.StaleBroadcastReplacementAfter = DefaultStaleBroadcastReplacementAfter
	}
	if options.PreSignRPCTimeout <= 0 {
		options.PreSignRPCTimeout = DefaultPreSignRPCTimeout
	}
	if options.SignTimeout <= 0 {
		options.SignTimeout = DefaultSignTimeout
	}
	if options.SigningLeaseTTL <= 0 {
		options.SigningLeaseTTL = DefaultSigningLeaseTTL
	}
	if options.SendTimeout <= 0 {
		options.SendTimeout = DefaultSendTimeout
	}
	if options.BroadcastLeaseTTL <= 0 {
		options.BroadcastLeaseTTL = DefaultBroadcastLeaseTTL
	}
	if options.NonceReconcileInterval <= 0 {
		options.NonceReconcileInterval = DefaultNonceReconcileInterval
	}
	return options
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
		didProcess, err := m.processTarget(ctx, target)
		processed = processed || didProcess
		if err != nil {
			return processed, err
		}
	}
	return processed, nil
}

type targetStage struct {
	run             func(context.Context, Target) (int64, error)
	noWork          []error
	deferred        []error
	progressError   error
	progressMessage string
	failureMessage  string
	successMessage  string
}

func (m *Manager) processTarget(ctx context.Context, target Target) (bool, error) {
	signerID := "<nil>"
	if target.Signer != nil {
		signerID = target.Signer.Address().Hex()
		if err := m.ProcessRecovery(ctx, target); err != nil {
			m.logger.Warn("tx recovery probe failed", "chain_eid", target.ChainEID, "error", err)
		}
	}
	// One durable action per target per pass. Receipts precede replacement,
	// and persisted broadcasts precede creating new signed work.
	stages := []targetStage{
		{
			run:            m.processOneReceipt,
			noWork:         []error{ErrNoReceiptUpdate},
			failureMessage: "tx receipt processing failed",
			successMessage: "processed tx receipt",
		},
		{
			run:            m.ProcessNonceReconciliation,
			noWork:         []error{db.ErrNoNonceReconcileWork},
			failureMessage: "nonce reconciliation failed",
			successMessage: "processed nonce reconciliation",
		},
		{
			run:            m.ProcessCancelRequest,
			noWork:         []error{db.ErrNoCancelWork, db.ErrOutboxLeaseLost, db.ErrActiveAttemptChanged},
			deferred:       []error{ErrTxDeferred},
			failureMessage: "cancel request processing failed",
			successMessage: "processed cancel request",
		},
		{
			run:             m.ProcessBroadcast,
			progressError:   db.ErrBroadcastLaneHeld,
			progressMessage: "held exhausted broadcast lane",
			noWork:          []error{db.ErrNoBroadcastCandidate, db.ErrSignerLaneBlocked, db.ErrOutboxLeaseLost},
			failureMessage:  "tx broadcast processing failed",
			successMessage:  "processed tx broadcast",
		},
		{
			run:            m.ProcessStaleBroadcastReplacement,
			noWork:         []error{db.ErrNoStaleBroadcastReplacement},
			deferred:       []error{ErrTxDeferred, db.ErrOutboxLeaseLost, db.ErrActiveAttemptChanged},
			failureMessage: "stale tx replacement processing failed",
			successMessage: "processed stale broadcast tx replacement",
		},
		{
			run:            m.ProcessFailedRetry,
			noWork:         []error{db.ErrNoFailedTxRetry},
			failureMessage: "failed tx retry processing failed",
			successMessage: "requeued failed tx outbox row",
		},
		{
			run:            m.ProcessNext,
			deferred:       []error{ErrNoQueuedTx, ErrTxDeferred, db.ErrSignerLaneBlocked, db.ErrOutboxLeaseLost, db.ErrTxSendScopeInactive},
			failureMessage: "queued tx processing failed",
			successMessage: "processed tx outbox row",
		},
	}
	for _, stage := range stages {
		id, err := stage.run(ctx, target)
		if matchesStageError(err, stage.noWork) {
			continue
		}
		if matchesStageError(err, stage.deferred) {
			return false, nil
		}
		if stage.progressError != nil && errors.Is(err, stage.progressError) {
			// Parking an exhausted lane is durable progress worth a hot rerun.
			m.logger.Info(stage.progressMessage, "chain_eid", target.ChainEID, "chain_name", target.ChainName, "signer", signerID)
			return true, nil
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, ctxErr
			}
			m.logger.Warn(stage.failureMessage, "chain_eid", target.ChainEID, "chain_name", target.ChainName, "signer", signerID, "error", err.Error())
			return false, nil
		}
		m.logger.Info(stage.successMessage, "id", id, "chain_eid", target.ChainEID, "chain_name", target.ChainName, "signer", signerID)
		return true, nil
	}
	return false, nil
}

func (m *Manager) processOneReceipt(ctx context.Context, target Target) (int64, error) {
	return m.ProcessReceipts(ctx, target, 1)
}

func matchesStageError(err error, candidates []error) bool {
	for _, candidate := range candidates {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}
