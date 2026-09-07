package db

import (
	"context"
	"errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
)

// RebroadcastAttempt is the immutable active transaction and its owning lane.
// RawTx is sensitive and must never be included in diagnostics.
type RebroadcastAttempt struct {
	BroadcastClaim
	ChainEID uint32
	SignerID string
}

// GetRebroadcastAttempt reads the current attempt without authorizing a send.
func (s *Store) GetRebroadcastAttempt(ctx context.Context, id int64) (RebroadcastAttempt, error) {
	var a RebroadcastAttempt
	var hash []byte
	err := s.pool.QueryRow(ctx, `SELECT a.id,o.id,o.purpose,a.nonce,a.tx_hash,a.raw_tx,a.kind,o.chain_eid,o.signer_id
 FROM tx_outbox o JOIN tx_attempts a ON a.id=o.active_attempt_id AND a.outbox_id=o.id
 WHERE o.id=$1 AND o.nonce=a.nonce`, id).Scan(&a.AttemptID, &a.OutboxID, &a.Purpose, &a.Nonce, &hash, &a.RawTx, &a.Kind, &a.ChainEID, &a.SignerID)
	if err != nil {
		return RebroadcastAttempt{}, err
	}
	if len(hash) != common.HashLength {
		return RebroadcastAttempt{}, errors.New("invalid persisted transaction hash")
	}
	a.TxHash = common.BytesToHash(hash)
	return a, nil
}

// ClaimManualRebroadcast reserves exactly one operator send, including beyond
// the automatic replay budget. Both leases fence concurrent sending and signing.
// A preflight snapshot is checked again under the signer and row locks.
func (s *Store) ClaimManualRebroadcast(ctx context.Context, expected RebroadcastAttempt, token uuid.UUID, ttl time.Duration) error {
	if expected.OutboxID <= 0 || expected.AttemptID <= 0 || expected.ChainEID == 0 || expected.SignerID == "" || token == uuid.Nil || ttl <= 0 {
		return errors.New("manual rebroadcast requires an attempt, lane, token and positive ttl")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lockSignerNonce(ctx, tx, expected.ChainEID, expected.SignerID); err != nil {
		return err
	}
	var count int64
	err = tx.QueryRow(ctx, `SELECT a.broadcast_count FROM tx_outbox o
 JOIN tx_attempts a ON a.id=o.active_attempt_id AND a.outbox_id=o.id
 WHERE o.id=$1 AND a.id=$2 AND o.chain_eid=$3 AND o.signer_id=$4
 AND a.raw_tx=$5 AND a.tx_hash=$6 AND o.nonce=a.nonce AND a.nonce=$7
 AND ((o.status IN ('signed','broadcast') AND o.held_reason IS NULL)
      OR (o.status='held' AND o.held_reason='broadcast_exhausted'))
 AND a.state IN ('signed','ambiguous','submitted') AND o.receipt_outcome IS NULL
 AND (o.cancel_requested_at IS NULL OR a.kind='cancel')
 AND (o.lease_until IS NULL OR o.lease_until<=now())
 AND (a.broadcast_lease_until IS NULL OR a.broadcast_lease_until<=now())
 AND NOT EXISTS(SELECT 1 FROM tx_outbox h WHERE h.chain_eid=o.chain_eid AND h.signer_id=o.signer_id
 AND h.nonce<o.nonce AND h.status NOT IN ('confirmed','failed'))
 FOR UPDATE OF o,a`, expected.OutboxID, expected.AttemptID, expected.ChainEID, expected.SignerID, expected.RawTx, expected.TxHash.Bytes(), expected.Nonce).Scan(&count)
	if err != nil {
		return err
	}
	// Preserve first-broadcast evidence and the cumulative automatic budget.
	_, err = tx.Exec(ctx, `UPDATE tx_outbox SET replay_authorized=false,
 first_broadcast_at=COALESCE(first_broadcast_at,(SELECT min(created_at) FROM tx_attempts WHERE outbox_id=$1 AND broadcast_count>0),now()),
 next_recovery_at=now()+$2::bigint*interval '1 second',
 lease_token=$3,lease_until=clock_timestamp()+$4::interval,updated_at=now() WHERE id=$1`,
		expected.OutboxID, 60*(1<<uint(min(count, 3))), token, pgInterval(ttl))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tx_attempts SET state=CASE WHEN state='signed' THEN 'ambiguous' ELSE state END,
 broadcast_count=broadcast_count+1,last_broadcast_at=now(),next_broadcast_at=now()+$2::interval,
 broadcast_lease_token=$3,broadcast_lease_until=clock_timestamp()+$4::interval,updated_at=now() WHERE id=$1`,
		expected.AttemptID, pgInterval(broadcastReplayDelay(count+1)), token, pgInterval(ttl))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
