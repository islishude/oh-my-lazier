package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RecoveryTask reserves a due visibility check against one immutable attempt.
type RecoveryTask struct {
	ID                  int64
	AttemptID           int64
	Token               uuid.UUID
	Hash                common.Hash
	Nonce               uint64
	AbsentSince         *time.Time
	FirstBroadcastAt    *time.Time
	VisibilityCheckedAt *time.Time
	Broadcasts          int64
	Replacements        int64
}

// ClaimRecovery selects the lowest outstanding nonce, never skipping a delayed
// head in order to spend a higher nonce's recovery budget.
func (s *Store) ClaimRecovery(ctx context.Context, eid uint32, signer string, now time.Time, window int) (RecoveryTask, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecoveryTask{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockSignerNonce(ctx, tx, eid, signer); err != nil {
		return RecoveryTask{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tx_recovery_lanes(chain_eid,signer_id,max_inflight) VALUES($1,$2,$3)
 ON CONFLICT(chain_eid,signer_id) DO UPDATE SET max_inflight=EXCLUDED.max_inflight WHERE tx_recovery_lanes.max_inflight<>EXCLUDED.max_inflight`, eid, signer, window); err != nil {
		return RecoveryTask{}, err
	}
	var t RecoveryTask
	var hash []byte
	t.Token = uuid.New()
	err = tx.QueryRow(ctx, `WITH head AS (
 SELECT id FROM tx_outbox WHERE chain_eid=$1 AND signer_id=$2 AND nonce IS NOT NULL
 AND status NOT IN ('confirmed','failed') ORDER BY nonce,id LIMIT 1
 ) UPDATE tx_outbox o SET recovery_lease_token=$3,recovery_lease_until=$4::timestamptz+interval '2 minutes',
 replay_authorized=false,
 first_broadcast_at=COALESCE(o.first_broadcast_at,(SELECT min(created_at) FROM tx_attempts WHERE outbox_id=o.id AND broadcast_count>0))
 FROM head,tx_attempts a WHERE o.id=head.id AND a.id=o.active_attempt_id AND a.outbox_id=o.id
 AND o.status='broadcast' AND o.receipt_outcome IS NULL AND o.cancel_requested_at IS NULL
 AND a.state IN ('submitted','ambiguous')
 AND (o.next_visibility_at IS NULL OR o.next_visibility_at<=$4)
 AND (o.recovery_lease_until IS NULL OR o.recovery_lease_until<=$4)
 RETURNING o.id,a.id,a.tx_hash,o.nonce,o.absent_since,o.first_broadcast_at,o.visibility_checked_at,a.broadcast_count,
 (SELECT count(*) FROM tx_attempts r WHERE r.outbox_id=o.id AND r.kind='replacement')`, eid, signer, t.Token, now).Scan(&t.ID, &t.AttemptID, &hash, &t.Nonce, &t.AbsentSince, &t.FirstBroadcastAt, &t.VisibilityCheckedAt, &t.Broadcasts, &t.Replacements)
	if errors.Is(err, pgx.ErrNoRows) {
		if e := tx.Commit(ctx); e != nil {
			return RecoveryTask{}, e
		}
		return RecoveryTask{}, err
	}
	if err != nil {
		return RecoveryTask{}, err
	}
	t.Hash = common.BytesToHash(hash)
	return t, tx.Commit(ctx)
}

// RecoveryObservation contains evidence for scheduling, never a terminal verdict.
type RecoveryObservation struct {
	Now     time.Time
	Absent  bool
	Visible bool
	Reason  string
	Nonce   *uint64
	Replay  bool
}

// FinishRecovery persists a fenced observation without resetting lifetime budgets.
func (s *Store) FinishRecovery(ctx context.Context, t RecoveryTask, eid uint32, signer string, o RecoveryObservation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Match the claim path's signer -> lane/outbox lock order.
	if err = lockSignerNonce(ctx, tx, eid, signer); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE tx_outbox SET
 last_seen_at=CASE WHEN $4 THEN $3 ELSE last_seen_at END,
 absent_since=CASE WHEN $5 THEN CASE WHEN visibility_checked_at<$3::timestamptz-interval '2 minutes' THEN $3 ELSE COALESCE(absent_since,$3) END ELSE NULL END,
 visibility_checked_at=$3,next_visibility_at=$3::timestamptz+interval '60 seconds',
 recovery_lease_token=NULL,recovery_lease_until=NULL,
 replay_authorized=$6 AND (next_recovery_at IS NULL OR next_recovery_at<=$3),
 next_recovery_at=CASE WHEN $7='rpc_unavailable' THEN $3::timestamptz+interval '60 seconds' ELSE next_recovery_at END
 WHERE id=$1 AND active_attempt_id=$2 AND recovery_lease_token=$8
 AND recovery_lease_until>now() AND status='broadcast' AND receipt_outcome IS NULL AND cancel_requested_at IS NULL`, t.ID, t.AttemptID, o.Now, o.Visible, o.Absent, o.Replay, o.Reason, t.Token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrOutboxLeaseLost
	}
	if o.Nonce != nil {
		_, err = tx.Exec(ctx, `UPDATE tx_recovery_lanes SET
 nonce_changed_at=CASE WHEN observed_nonce IS DISTINCT FROM $3 OR nonce_observed_at IS NULL OR nonce_observed_at<$4::timestamptz-interval '2 minutes' THEN $4 ELSE nonce_changed_at END,
 observed_nonce=$3,nonce_observed_at=$4 WHERE chain_eid=$1 AND signer_id=$2`, eid, signer, *o.Nonce, o.Now)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SetRecoveryReason records a bounded reason and returns whether a log is due.
// The active-attempt fence prevents delayed probes overwriting replacement state.
func (s *Store) SetRecoveryReason(ctx context.Context, id, attempt int64, reason string, detail any, now time.Time) (bool, string, error) {
	var payload []byte
	var err error
	if detail != nil {
		payload, err = json.Marshal(detail)
		if err != nil {
			return false, "", err
		}
	}
	var previous string
	var due bool
	err = s.pool.QueryRow(ctx, `WITH old AS (
 SELECT id,recovery_reason,(recovery_reason<>$3 OR recovery_logged_at IS NULL OR recovery_logged_at<=$5::timestamptz-interval '5 minutes') AS due
 FROM tx_outbox WHERE id=$1 AND active_attempt_id=$2 AND status NOT IN ('confirmed','failed') FOR UPDATE
 ) UPDATE tx_outbox o SET recovery_reason=$3,
 recovery_since=CASE WHEN $3='' THEN NULL WHEN o.recovery_reason<>$3 OR o.recovery_since IS NULL THEN $5 ELSE o.recovery_since END,
 recovery_detail=$4,recovery_logged_at=CASE WHEN old.due THEN $5 ELSE o.recovery_logged_at END
 FROM old WHERE o.id=old.id RETURNING old.due,old.recovery_reason`, id, attempt, reason, payload, now).Scan(&due, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	return due, previous, err
}

// RecoveryAttemptID resolves the current attempt for fee-preflight diagnostics.
func (s *Store) RecoveryAttemptID(ctx context.Context, id int64) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT active_attempt_id FROM tx_outbox WHERE id=$1`, id).Scan(&n)
	return n, err
}

// MarkRecoveryNonceConsumed routes unexplained consumption through the existing
// confirmed-nonce reconciler instead of treating a latest nonce as finality.
func (s *Store) MarkRecoveryNonceConsumed(ctx context.Context, t RecoveryTask) error {
	_, err := s.pool.Exec(ctx, `UPDATE tx_outbox SET status='held',held_reason='nonce_reconcile_required',replay_authorized=false
 WHERE id=$1 AND active_attempt_id=$2 AND status='broadcast' AND receipt_outcome IS NULL AND cancel_requested_at IS NULL`, t.ID, t.AttemptID)
	return err
}

// InspectTx returns only operator-safe fields; signed raw bytes and calldata are excluded.
func (s *Store) InspectTx(ctx context.Context, id int64) (json.RawMessage, error) {
	var result []byte
	err := s.pool.QueryRow(ctx, `SELECT jsonb_build_object(
 'id',o.id,'chain_eid',o.chain_eid,'signer',o.signer_id,'nonce',o.nonce,'status',o.status,
 'active_attempt_id',o.active_attempt_id,'replace_requested_at',o.replace_requested_at,
 'first_broadcast_at',COALESCE(o.first_broadcast_at,(SELECT min(created_at) FROM tx_attempts WHERE outbox_id=o.id AND broadcast_count>0)),
 'last_seen_at',o.last_seen_at,'absent_since',o.absent_since,'next_check_at',o.next_visibility_at,
 'next_action_at',o.next_recovery_at,'reason',o.recovery_reason,'blocked_since',o.recovery_since,'detail',o.recovery_detail,
 'next_sign_at',o.next_sign_at,'cancel_requested_at',o.cancel_requested_at,'held_reason',o.held_reason,'receipt_outcome',o.receipt_outcome,'pre_sign_failure_count',o.pre_sign_failure_count,
 'head', (SELECT jsonb_build_object('id',h.id,'nonce',h.nonce) FROM tx_outbox h WHERE h.chain_eid=o.chain_eid AND h.signer_id=o.signer_id AND h.nonce IS NOT NULL AND h.status NOT IN ('confirmed','failed') ORDER BY h.nonce LIMIT 1),
 'max_broadcasts',5,'max_automatic_replacements',5,
 'replacement_count',(SELECT count(*) FROM tx_attempts WHERE outbox_id=o.id AND kind='replacement'),
 'attempts',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',a.id,'hash','0x'||encode(a.tx_hash,'hex'),'kind',a.kind,'state',a.state,'broadcast_count',a.broadcast_count,'max_fee_per_gas',a.max_fee_per_gas,'max_priority_fee_per_gas',a.max_priority_fee_per_gas,'last_broadcast_at',a.last_broadcast_at,'send_error_class',a.send_error_class) ORDER BY a.id) FROM tx_attempts a WHERE a.outbox_id=o.id),'[]'::jsonb)
 ) FROM tx_outbox o WHERE o.id=$1`, id).Scan(&result)
	return result, err
}

// RecoveryHashes includes all signed attempts because send-result persistence can race a crash.
func (s *Store) RecoveryHashes(ctx context.Context, id int64) ([]common.Hash, error) {
	rows, err := s.pool.Query(ctx, `SELECT tx_hash FROM tx_attempts WHERE outbox_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hashes []common.Hash
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		hashes = append(hashes, common.BytesToHash(b))
	}
	return hashes, rows.Err()
}

// RecoveryBlocked preserves actionable fee/budget reasons through healthy probes.
func (s *Store) RecoveryBlocked(ctx context.Context, id int64) (bool, error) {
	var b bool
	err := s.pool.QueryRow(ctx, `SELECT recovery_reason IN ('fee_cap','replacement_exhausted') FROM tx_outbox WHERE id=$1`, id).Scan(&b)
	return b, err
}
