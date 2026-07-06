package txmgr

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"
)

const defaultBalancePollInterval = time.Minute

// BalanceRecorder records signer balance polling results.
type BalanceRecorder interface {
	RecordSignerBalance(chainEID uint32, signerID string, balance, minNativeBalanceWei *big.Int, duration time.Duration, err error)
}

// BalanceMonitor polls native balances for transaction-signing targets.
type BalanceMonitor struct {
	targets      []Target
	recorder     BalanceRecorder
	logger       *slog.Logger
	pollInterval time.Duration
}

// NewBalanceMonitor creates a signer balance monitor for tx manager targets.
func NewBalanceMonitor(targets []Target, recorder BalanceRecorder, logger *slog.Logger) *BalanceMonitor {
	copiedTargets := make([]Target, len(targets))
	copy(copiedTargets, targets)
	return &BalanceMonitor{
		targets:      copiedTargets,
		recorder:     recorder,
		logger:       logger,
		pollInterval: defaultBalancePollInterval,
	}
}

// Run polls configured signer balances until the context is canceled.
func (m *BalanceMonitor) Run(ctx context.Context) error {
	if err := m.pollOnce(ctx); err != nil {
		return err
	}
	for {
		timer := time.NewTimer(m.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if err := m.pollOnce(ctx); err != nil {
			return err
		}
	}
}

func (m *BalanceMonitor) pollOnce(ctx context.Context) error {
	for _, target := range m.targets {
		if target.Signer == nil {
			return errors.New("balance monitor target signer is required")
		}
		if target.Client == nil {
			return errors.New("balance monitor target client is required")
		}
		signerID := target.Signer.Address().Hex()
		started := time.Now()
		balance, err := target.Client.BalanceAt(ctx, target.Signer.Address(), nil)
		if err == nil && balance == nil {
			err = errors.New("signer balance is required")
		}
		duration := time.Since(started)
		if m.recorder != nil {
			m.recorder.RecordSignerBalance(target.ChainEID, signerID, balance, target.MinNativeBalanceWei, duration, err)
		}
		if err != nil {
			m.logger.Warn("failed signer balance poll", "chain_eid", target.ChainEID, "signer", signerID, "error", err.Error())
			continue
		}
		if target.MinNativeBalanceWei != nil && balance.Cmp(target.MinNativeBalanceWei) < 0 {
			m.logger.Warn("low signer native balance", "chain_eid", target.ChainEID, "signer", signerID, "balance_wei", balance.String(), "min_native_balance_wei", target.MinNativeBalanceWei.String())
		}
	}
	return nil
}
