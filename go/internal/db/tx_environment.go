package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// These predicates use the outbox/active-attempt aliases o and a. Keeping the
// eligibility shared makes recovery, replacement and held metrics agree.
var environmentalHoldSQL = fmt.Sprintf(`(
 o.status='held' AND o.held_reason='broadcast_exhausted'
 AND o.receipt_outcome IS NULL AND o.cancel_requested_at IS NULL
 AND a.kind IN ('original','replacement') AND a.state='ambiguous'
 AND a.send_error_class='retryable_env' AND a.broadcast_count >= %d
 AND o.pre_sign_failure_count < %d
 AND (SELECT count(*) FROM tx_attempts r WHERE r.outbox_id=o.id AND r.kind='replacement') < %d
)`, TxMaxBroadcasts, TxMaxPreSignFailures, TxMaxReplacements)

const environmentalEvidenceSQL = `(
 o.first_broadcast_at IS NOT NULL
 AND o.absent_since IS NOT NULL
 AND o.visibility_checked_at >= o.absent_since + interval '60 seconds'
 AND o.visibility_checked_at BETWEEN now()-interval '2 minutes' AND now()
 AND o.next_recovery_at <= now()
 AND a.last_broadcast_at <= now()-interval '60 seconds'
 AND (o.recovery_lease_until IS NULL OR o.recovery_lease_until<=now())
 AND (a.broadcast_lease_until IS NULL OR a.broadcast_lease_until<=now())
 AND NOT EXISTS (SELECT 1 FROM tx_outbox h WHERE h.chain_eid=o.chain_eid
   AND h.signer_id=o.signer_id AND h.nonce<o.nonce AND h.status NOT IN ('confirmed','failed'))
 AND EXISTS (SELECT 1 FROM tx_recovery_lanes l WHERE l.chain_eid=o.chain_eid
   AND l.signer_id=o.signer_id AND l.observed_nonce<=o.nonce
   AND l.nonce_observed_at BETWEEN now()-interval '2 minutes' AND now()
   AND l.nonce_observed_at>=o.visibility_checked_at)
)`

func environmentalReplayDelay(count int64) time.Duration {
	return 20 * time.Second * time.Duration(1<<uint(max(0, min(count-1, 3))))
}

// checkEnvironmentalReplacement rechecks the automatic authorization under the
// caller's outbox lock, both before signing and before publishing its result.
func checkEnvironmentalReplacement(ctx context.Context, tx pgx.Tx, id int64) error {
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT COALESCE(`+environmentalHoldSQL+` AND `+environmentalEvidenceSQL+`
 AND o.replace_requested_at IS NULL, false)
 FROM tx_outbox o JOIN tx_attempts a ON a.id=o.active_attempt_id AND a.outbox_id=o.id
 WHERE o.id=$1`, id).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrActiveAttemptChanged
	}
	return nil
}
