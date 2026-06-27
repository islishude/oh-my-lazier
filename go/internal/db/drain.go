package db

import (
	"context"
	"errors"

	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

// StatusCount summarizes durable workflow rows by status.
type StatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// DrainStatus reports whether one packet pathway has no unfinished worker state.
type DrainStatus struct {
	SrcEID                      uint32        `json:"src_eid"`
	DstEID                      uint32        `json:"dst_eid"`
	Ready                       bool          `json:"ready"`
	PacketsTotal                int64         `json:"packets_total"`
	ExecutorPending             []StatusCount `json:"executor_pending"`
	DVNPending                  []StatusCount `json:"dvn_pending"`
	OutboxPending               []StatusCount `json:"outbox_pending"`
	VerifiedButUndeliveredCount int64         `json:"verified_but_undelivered_count"`
}

// CheckDrainStatus summarizes in-flight packets, jobs, and tx outbox rows for
// one pathway before Executor/DVN config changes or rollback.
func (s *Store) CheckDrainStatus(ctx context.Context, srcEID, dstEID uint32) (DrainStatus, error) {
	if srcEID == 0 || dstEID == 0 {
		return DrainStatus{}, errors.New("source and destination eids are required")
	}
	if srcEID == dstEID {
		return DrainStatus{}, errors.New("source and destination eids must differ")
	}

	status := DrainStatus{SrcEID: srcEID, DstEID: dstEID}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM packets
		WHERE src_eid = $1 AND dst_eid = $2
	`, srcEID, dstEID).Scan(&status.PacketsTotal); err != nil {
		return DrainStatus{}, err
	}

	var err error
	status.ExecutorPending, err = s.statusCounts(ctx, `
		SELECT ej.status, count(*)
		FROM executor_jobs ej
		JOIN packets p ON p.guid = ej.guid
		WHERE p.src_eid = $1 AND p.dst_eid = $2 AND ej.status <> $3
		GROUP BY ej.status
		ORDER BY ej.status
	`, srcEID, dstEID, string(packets.ExecutorDelivered))
	if err != nil {
		return DrainStatus{}, err
	}
	status.DVNPending, err = s.statusCounts(ctx, `
		SELECT dj.status, count(*)
		FROM dvn_jobs dj
		JOIN packets p ON p.guid = dj.guid
		WHERE p.src_eid = $1
			AND p.dst_eid = $2
			AND dj.status <> ALL($3::text[])
		GROUP BY dj.status
		ORDER BY dj.status
	`, srcEID, dstEID, []string{string(packets.DVNWouldVerify), string(packets.DVNVerified)})
	if err != nil {
		return DrainStatus{}, err
	}
	status.OutboxPending, err = s.statusCounts(ctx, `
		SELECT tx.status, count(*)
		FROM tx_outbox tx
		JOIN packets p ON p.guid = tx.guid
		WHERE p.src_eid = $1 AND p.dst_eid = $2 AND tx.status <> $3
		GROUP BY tx.status
		ORDER BY tx.status
	`, srcEID, dstEID, TxStatusConfirmed)
	if err != nil {
		return DrainStatus{}, err
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM executor_jobs ej
		JOIN packets p ON p.guid = ej.guid
		WHERE p.src_eid = $1
			AND p.dst_eid = $2
			AND ej.status = ANY($3::text[])
	`, srcEID, dstEID, []string{
		string(packets.ExecutorCommitted),
		string(packets.ExecutorExecutable),
		string(packets.ExecutorLzReceiveTxEnqueued),
		string(packets.ExecutorLzReceiveFailed),
	}).Scan(&status.VerifiedButUndeliveredCount); err != nil {
		return DrainStatus{}, err
	}

	status.Ready = len(status.ExecutorPending) == 0 &&
		len(status.DVNPending) == 0 &&
		len(status.OutboxPending) == 0
	return status, nil
}

func (s *Store) statusCounts(ctx context.Context, sql string, args ...any) ([]StatusCount, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make([]StatusCount, 0)
	for rows.Next() {
		var count StatusCount
		if err := rows.Scan(&count.Status, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
