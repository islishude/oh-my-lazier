package db

import (
	"context"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

// RecoveryStat aggregates one signer lane, never a GUID or transaction hash.
type RecoveryStat struct {
	ChainEID          uint32
	SignerID          string
	Inflight          int64
	Window            int
	HeadNonce         uint64
	FirstBroadcastAge float64
	NonceStallAge     float64
	Reason            string
	BlockedAge        float64
	Unseen            int64
	Replays           int64
	Replacements      int64
}

func (s *Store) recoveryStats(ctx context.Context) ([]RecoveryStat, error) {
	rows, err := s.pool.Query(ctx, `WITH lanes AS (
 SELECT chain_eid,signer_id,count(*) FILTER(WHERE nonce IS NOT NULL AND status NOT IN ('confirmed','failed')) AS inflight
 FROM tx_outbox GROUP BY chain_eid,signer_id
 ) SELECT l.chain_eid,l.signer_id,l.inflight,COALESCE(r.max_inflight,$1),COALESCE(h.nonce,0),
 COALESCE(GREATEST(0,extract(epoch FROM now()-COALESCE(h.first_broadcast_at,(SELECT min(created_at) FROM tx_attempts WHERE outbox_id=h.id AND broadcast_count>0)))),0)::float8,
 CASE WHEN l.inflight>0 AND r.nonce_observed_at>=now()-interval '2 minutes' THEN COALESCE(GREATEST(0,extract(epoch FROM now()-r.nonce_changed_at)),0) ELSE 0 END::float8,
 COALESCE(h.recovery_reason,''),COALESCE(GREATEST(0,extract(epoch FROM now()-h.recovery_since)),0)::float8,
 (SELECT count(*) FROM tx_outbox u WHERE u.chain_eid=l.chain_eid AND u.signer_id=l.signer_id AND u.status NOT IN ('confirmed','failed') AND u.absent_since IS NOT NULL AND u.visibility_checked_at>=u.absent_since+interval '60 seconds'),
 (SELECT COALESCE(sum(GREATEST(a.broadcast_count-1,0)),0)::bigint FROM tx_attempts a JOIN tx_outbox o ON o.id=a.outbox_id WHERE o.chain_eid=l.chain_eid AND o.signer_id=l.signer_id),
 (SELECT count(*) FROM tx_attempts a JOIN tx_outbox o ON o.id=a.outbox_id WHERE o.chain_eid=l.chain_eid AND o.signer_id=l.signer_id AND a.kind='replacement')
 FROM lanes l JOIN chains c ON c.eid=l.chain_eid AND c.enabled
 LEFT JOIN tx_recovery_lanes r USING(chain_eid,signer_id)
 LEFT JOIN LATERAL(SELECT * FROM tx_outbox o WHERE o.chain_eid=l.chain_eid AND o.signer_id=l.signer_id AND o.nonce IS NOT NULL AND o.status NOT IN ('confirmed','failed') ORDER BY nonce,id LIMIT 1) h ON true
 ORDER BY l.chain_eid,l.signer_id`, config.DefaultMaxInflightPerSigner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecoveryStat
	for rows.Next() {
		var r RecoveryStat
		if err = rows.Scan(&r.ChainEID, &r.SignerID, &r.Inflight, &r.Window, &r.HeadNonce, &r.FirstBroadcastAge, &r.NonceStallAge, &r.Reason, &r.BlockedAge, &r.Unseen, &r.Replays, &r.Replacements); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
