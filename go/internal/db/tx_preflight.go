package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// NextPreflightObservation orders observations before their RPC checks begin.
// Tokens are database-wide and survive worker restarts and clock disagreement.
func (s *Store) NextPreflightObservation(ctx context.Context) (int64, error) {
	var token int64
	err := s.pool.QueryRow(ctx, `SELECT nextval('tx_preflight_observations')`).Scan(&token)
	return token, err
}

// RecordPreflight records an observation only while the row is still unclaimed
// and no newer observation has completed. An empty reason records recovery.
// The returned booleans indicate whether it applied and cleared a prior failure.
func (s *Store) RecordPreflight(ctx context.Context, id, token int64, reason, detail string) (bool, bool, error) {
	var recovered bool
	err := s.pool.QueryRow(ctx, `WITH previous AS MATERIALIZED (
 SELECT id, preflight_reason FROM tx_outbox
 WHERE id=$1 AND status='queued' AND nonce IS NULL AND active_attempt_id IS NULL
 AND preflight_observation_id < $2 FOR UPDATE
 ) UPDATE tx_outbox o SET
 preflight_observation_id=$2, preflight_observed_at=clock_timestamp(),
 preflight_failure_since=CASE WHEN $3='' THEN NULL ELSE COALESCE(o.preflight_failure_since,clock_timestamp()) END,
 preflight_reason=$3, preflight_error=$4
 FROM previous WHERE o.id=previous.id
 RETURNING previous.preflight_reason <> '' AND $3=''`, id, token, reason, detail).Scan(&recovered)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return err == nil, recovered, err
}

// TxPreflightStat summarizes unclaimed transactions blocked by RPC preflight.
type TxPreflightStat struct {
	ChainEID         uint32
	SignerID         string
	Purpose          string
	Reason           string
	Count            uint64
	OldestAgeSeconds float64
}

func (s *Store) txPreflightStats(ctx context.Context) ([]TxPreflightStat, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT o.chain_eid,o.signer_id,o.purpose,o.preflight_reason,
 count(*), GREATEST(0,EXTRACT(EPOCH FROM (now()-min(o.preflight_failure_since))))::double precision
 FROM tx_outbox o LEFT JOIN packets p ON p.guid=o.guid
 WHERE o.status='queued' AND o.nonce IS NULL AND o.active_attempt_id IS NULL
 AND o.preflight_failure_since IS NOT NULL
 AND CASE WHEN p.guid IS NULL THEN EXISTS (SELECT 1 FROM chains c WHERE c.eid=o.chain_eid AND c.enabled)
 ELSE %s END
 GROUP BY o.chain_eid,o.signer_id,o.purpose,o.preflight_reason
 ORDER BY o.chain_eid,o.signer_id,o.purpose,o.preflight_reason`, fmt.Sprintf(statsEnabledPacketScopeSQL, "p")))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TxPreflightStat
	for rows.Next() {
		var stat TxPreflightStat
		if err := rows.Scan(&stat.ChainEID, &stat.SignerID, &stat.Purpose, &stat.Reason, &stat.Count, &stat.OldestAgeSeconds); err != nil {
			return nil, err
		}
		result = append(result, stat)
	}
	return result, rows.Err()
}
