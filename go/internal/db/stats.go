package db

import (
	"context"
	"fmt"
)

// StatsSnapshot is a point-in-time summary used by the HTTP metrics endpoint.
type StatsSnapshot struct {
	Chains         []ChainStat
	Pathways       []PathwayStat
	Packets        []PacketStat
	ExecutorJobs   []StatusStat
	DVNJobs        []StatusStat
	TxOutbox       []TxOutboxStat
	IndexerCursors []IndexerCursorStat
}

// ChainStat summarizes one configured chain.
type ChainStat struct {
	EID     uint32
	Name    string
	Enabled bool
	Paused  bool
}

// PathwayStat summarizes one configured pathway.
type PathwayStat struct {
	SrcEID  uint32
	DstEID  uint32
	Enabled bool
	Paused  bool
}

// PacketStat counts packets by source, destination, and packet state.
type PacketStat struct {
	SrcEID uint32
	DstEID uint32
	Status string
	Count  uint64
}

// StatusStat counts rows by state for tables without chain-specific labels.
type StatusStat struct {
	Status string
	Count  uint64
}

// TxOutboxStat counts transaction requests by chain and state.
type TxOutboxStat struct {
	ChainEID uint32
	Status   string
	Count    uint64
}

// IndexerCursorStat exposes durable indexer cursor progress.
type IndexerCursorStat struct {
	ChainEID  uint32
	Stream    string
	LastBlock uint64
}

// Stats returns a read-only worker summary for health and metrics reporting.
func (s *Store) Stats(ctx context.Context) (StatsSnapshot, error) {
	chains, err := s.chainStats(ctx)
	if err != nil {
		return StatsSnapshot{}, err
	}
	pathways, err := s.pathwayStats(ctx)
	if err != nil {
		return StatsSnapshot{}, err
	}
	packets, err := s.packetStats(ctx)
	if err != nil {
		return StatsSnapshot{}, err
	}
	executorJobs, err := s.statusStats(ctx, "executor_jobs")
	if err != nil {
		return StatsSnapshot{}, err
	}
	dvnJobs, err := s.statusStats(ctx, "dvn_jobs")
	if err != nil {
		return StatsSnapshot{}, err
	}
	txOutbox, err := s.txOutboxStats(ctx)
	if err != nil {
		return StatsSnapshot{}, err
	}
	indexerCursors, err := s.indexerCursorStats(ctx)
	if err != nil {
		return StatsSnapshot{}, err
	}
	return StatsSnapshot{
		Chains:         chains,
		Pathways:       pathways,
		Packets:        packets,
		ExecutorJobs:   executorJobs,
		DVNJobs:        dvnJobs,
		TxOutbox:       txOutbox,
		IndexerCursors: indexerCursors,
	}, nil
}

func (s *Store) chainStats(ctx context.Context) ([]ChainStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT eid, name, enabled, paused
		FROM chains
		ORDER BY eid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ChainStat
	for rows.Next() {
		var stat ChainStat
		if err := rows.Scan(&stat.EID, &stat.Name, &stat.Enabled, &stat.Paused); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) pathwayStats(ctx context.Context) ([]PathwayStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT src_eid, dst_eid, enabled, paused
		FROM pathways
		ORDER BY src_eid, dst_eid, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PathwayStat
	for rows.Next() {
		var stat PathwayStat
		if err := rows.Scan(&stat.SrcEID, &stat.DstEID, &stat.Enabled, &stat.Paused); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) packetStats(ctx context.Context) ([]PacketStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT src_eid, dst_eid, status, count(*)::bigint
		FROM packets
		GROUP BY src_eid, dst_eid, status
		ORDER BY src_eid, dst_eid, status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PacketStat
	for rows.Next() {
		var stat PacketStat
		if err := rows.Scan(&stat.SrcEID, &stat.DstEID, &stat.Status, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) statusStats(ctx context.Context, table string) ([]StatusStat, error) {
	switch table {
	case "executor_jobs", "dvn_jobs":
	default:
		return nil, fmt.Errorf("unsupported stats table %q", table)
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT status, count(*)::bigint
		FROM %s
		GROUP BY status
		ORDER BY status
	`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []StatusStat
	for rows.Next() {
		var stat StatusStat
		if err := rows.Scan(&stat.Status, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) txOutboxStats(ctx context.Context) ([]TxOutboxStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chain_eid, status, count(*)::bigint
		FROM tx_outbox
		GROUP BY chain_eid, status
		ORDER BY chain_eid, status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []TxOutboxStat
	for rows.Next() {
		var stat TxOutboxStat
		if err := rows.Scan(&stat.ChainEID, &stat.Status, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}

func (s *Store) indexerCursorStats(ctx context.Context) ([]IndexerCursorStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT chain_eid, stream, last_block
		FROM indexer_cursors
		ORDER BY chain_eid, stream
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []IndexerCursorStat
	for rows.Next() {
		var stat IndexerCursorStat
		if err := rows.Scan(&stat.ChainEID, &stat.Stream, &stat.LastBlock); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}
