package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps the Postgres connection pool used by worker state machines.
type Store struct {
	pool *pgxpool.Pool
}

// Connect opens the Postgres pool for the configured database URL.
func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Ping verifies that the database connection is usable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close releases the database connection pool.
func (s *Store) Close() {
	s.pool.Close()
}
