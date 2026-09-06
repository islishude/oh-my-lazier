package txmgr

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
	"github.com/jackc/pgx/v5"
)

func visibilityVerdict(rows []rpcquorum.TransactionVisibility) (absent, visible bool) {
	missing := 0
	for _, r := range rows {
		switch r.State {
		case "pending", "mined":
			visible = true
		case "absent":
			missing++
		}
	}
	return !visible && len(rows) > 0 && missing >= len(rows)/2+1, visible
}

// ProcessRecovery probes one due head without claiming any higher nonce's budget.
func (m *Manager) ProcessRecovery(ctx context.Context, target Target) error {
	if err := validateTarget(target); err != nil {
		return err
	}
	now := m.options.Now()
	signer := target.Signer.Address().Hex()
	t, err := m.store.ClaimRecovery(ctx, target.ChainEID, signer, now, m.options.MaxInflightPerSigner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	obs := db.RecoveryObservation{Now: now, Reason: "rpc_unavailable"}
	probe, cancel := context.WithTimeout(ctx, m.options.PreSignRPCTimeout)
	defer cancel()
	// Inspect every attempt, including a former attempt mined after replacement.
	hashes, err := m.store.RecoveryHashes(probe, t.ID)
	if err == nil {
		for _, hash := range hashes {
			receipt, e := target.Client.TransactionReceipt(probe, hash)
			if errors.Is(e, ethereum.NotFound) {
				continue
			}
			if e != nil {
				err = e
				break
			}
			if receipt != nil {
				canonical, e := m.receiptOnCanonicalChain(probe, target, receipt)
				if e != nil {
					err = e
					break
				}
				if canonical {
					obs.Visible = true
					obs.Reason = ""
					return m.finishRecovery(ctx, target, t, obs)
				}
			}
		}
	}
	if err != nil {
		return m.finishRecovery(ctx, target, t, obs)
	}
	nonce, err := target.Client.NonceAt(probe, target.Signer.Address(), nil)
	if err != nil {
		return m.finishRecovery(ctx, target, t, obs)
	}
	obs.Nonce = &nonce
	if nonce > t.Nonce {
		if err = m.finishRecovery(ctx, target, t, obs); err != nil {
			return err
		}
		return m.store.MarkRecoveryNonceConsumed(ctx, t)
	}
	rows := target.Client.TransactionVisibility(probe, t.Hash)
	obs.Absent, obs.Visible = visibilityVerdict(rows)
	if obs.Visible || obs.Absent {
		obs.Reason = ""
	}
	if t.FirstBroadcastAt == nil {
		obs.Reason = "evidence_missing"
	} else if obs.Absent && t.AbsentSince != nil && t.VisibilityCheckedAt != nil && now.Sub(*t.VisibilityCheckedAt) <= 2*time.Minute && now.Sub(*t.AbsentSince) >= time.Minute {
		obs.Reason = "transaction_unseen"
		obs.Replay = t.Broadcasts < db.TxMaxBroadcasts
	}
	if t.Replacements >= db.TxMaxReplacements && (obs.Visible || obs.Absent) {
		obs.Reason = "replacement_exhausted"
	}
	return m.finishRecovery(ctx, target, t, obs)
}

func (m *Manager) finishRecovery(ctx context.Context, target Target, t db.RecoveryTask, o db.RecoveryObservation) error {
	signer := target.Signer.Address().Hex()
	if err := m.store.FinishRecovery(ctx, t, target.ChainEID, signer, o); err != nil {
		return err
	}
	// Intermittent RPC failures must not reset a persistent fee/budget age;
	// successful signing clears it instead.
	return m.logRecoveryReason(ctx, target, t.ID, t.AttemptID, o.Reason, nil, o.Now, false)
}

func (m *Manager) logRecoveryReason(ctx context.Context, target Target, id, attempt int64, reason string, detail any, now time.Time, force bool) error {
	if !force && reason != "replacement_exhausted" && reason != "evidence_missing" {
		keep, err := m.store.RecoveryBlocked(ctx, id)
		if err != nil {
			return err
		}
		if keep {
			return nil
		}
	}
	due, old, err := m.store.SetRecoveryReason(ctx, id, attempt, reason, detail, now)
	if err != nil {
		return err
	}
	if !due || (reason == "" && old == "") {
		return nil
	}
	args := []any{"id", id, "chain_eid", target.ChainEID, "signer", target.Signer.Address().Hex(), "reason", reason, "previous_reason", old, "detail", detail}
	if reason == "" {
		m.logger.Info("tx recovery blockage cleared", args...)
	} else {
		m.logger.Warn("tx recovery blocked", args...)
	}
	return nil
}
