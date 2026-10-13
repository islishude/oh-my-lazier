package txmgr

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/islishude/oh-my-lazier/go/internal/db"
)

// InvalidPriorityFeeError identifies an unusable RPC priority-fee suggestion.
type InvalidPriorityFeeError struct {
	OutboxID int64
	Value    string
}

func (e *InvalidPriorityFeeError) Error() string {
	return fmt.Sprintf("outbox tx %d priority fee per gas must be positive; RPC suggested %s", e.OutboxID, e.Value)
}

func priorityFeeValue(value *big.Int) string {
	if value == nil {
		return "nil"
	}
	return value.String()
}

type preflightError struct {
	stage string
	cause error
}

func (e *preflightError) Error() string { return e.cause.Error() }
func (e *preflightError) Unwrap() error { return e.cause }

func preflightReason(err error) string {
	if _, ok := errors.AsType[*InvalidPriorityFeeError](err); ok {
		return "invalid_priority_fee"
	}
	if _, ok := errors.AsType[*FeeCapError](err); ok {
		return "fee_cap"
	}
	if stage, ok := errors.AsType[*preflightError](err); ok {
		return stage.stage
	}
	return "fee_quote_error"
}

func (m *Manager) recordQueuedPreflightFailure(ctx context.Context, target Target, signerID string, queued db.QueuedOutboxTx, observation int64, cause error) (int64, error) {
	reason := preflightReason(cause)
	// Every failed attempt is visible at the default log level, even if a newer
	// concurrent observation prevents this one from updating durable state.
	m.logger.Warn("deferred tx outbox row", "reason", reason, "id", queued.ID, "chain_eid", target.ChainEID, "chain_name", target.ChainName, "signer", signerID, "purpose", queued.Purpose, "error", cause.Error())
	if _, _, err := m.store.RecordPreflight(ctx, queued.ID, observation, reason, cause.Error()); err != nil {
		return 0, fmt.Errorf("record preflight failure for outbox tx %d: %w", queued.ID, err)
	}
	return 0, fmt.Errorf("%w: preflight for outbox tx %d: %w", ErrTxDeferred, queued.ID, cause)
}
