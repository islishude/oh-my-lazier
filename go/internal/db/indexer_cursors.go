package db

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
)

// GetIndexerCursor returns the last successfully indexed block for one chain stream.
func (s *Store) GetIndexerCursor(ctx context.Context, chainEID uint32, stream string) (uint64, error) {
	if chainEID == 0 {
		return 0, errors.New("indexer cursor chain eid is required")
	}
	if stream == "" {
		return 0, errors.New("indexer cursor stream is required")
	}
	var lastBlock int64
	err := s.pool.QueryRow(ctx, `
		SELECT last_block
		FROM indexer_cursors
		WHERE chain_eid = $1 AND stream = $2
	`, chainEID, stream).Scan(&lastBlock)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, pgx.ErrNoRows
	}
	if err != nil {
		return 0, err
	}
	if lastBlock < 0 {
		return 0, errors.New("indexer cursor last block is negative")
	}
	return uint64(lastBlock), nil
}

// UpdateIndexerCursor records the last successfully indexed block for one chain stream.
func (s *Store) UpdateIndexerCursor(ctx context.Context, chainEID uint32, stream string, lastBlock uint64) error {
	if chainEID == 0 {
		return errors.New("indexer cursor chain eid is required")
	}
	if stream == "" {
		return errors.New("indexer cursor stream is required")
	}
	if lastBlock > math.MaxInt64 {
		return errors.New("indexer cursor last block exceeds database integer limit")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO indexer_cursors (chain_eid, stream, last_block)
		VALUES ($1, $2, $3)
		ON CONFLICT (chain_eid, stream) DO UPDATE SET
			last_block = GREATEST(indexer_cursors.last_block, EXCLUDED.last_block),
			updated_at = now()
	`, chainEID, stream, int64(lastBlock))
	return err
}
