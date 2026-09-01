package db

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

const (
	executorSourceJobLockSQL = `SELECT true FROM executor_jobs WHERE guid = $1 FOR UPDATE`
	dvnSourceJobLockSQL      = `SELECT true FROM dvn_jobs WHERE guid = $1 FOR UPDATE`
)

// UpsertExecutorAssignment atomically persists one packet and its executor assignment.
func (s *Store) UpsertExecutorAssignment(ctx context.Context, packet PacketRecord, job ExecutorJobRecord) error {
	if packet.GUID != job.GUID {
		return errors.New("executor assignment packet and job guid must match")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSourceAssignment(ctx, tx, packet.GUID, executorSourceJobLockSQL); err != nil {
		return err
	}
	if err := upsertPacket(ctx, tx, packet); err != nil {
		return err
	}
	if err := upsertExecutorJob(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// UpsertDVNAssignment atomically persists one packet and its DVN assignment.
func (s *Store) UpsertDVNAssignment(ctx context.Context, packet PacketRecord, job DVNJobRecord) error {
	if packet.GUID != job.GUID {
		return errors.New("dvn assignment packet and job guid must match")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockSourceAssignment(ctx, tx, packet.GUID, dvnSourceJobLockSQL); err != nil {
		return err
	}
	if err := upsertPacket(ctx, tx, packet); err != nil {
		return err
	}
	if err := upsertDVNJob(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockSourceAssignment(ctx context.Context, tx pgx.Tx, guid common.Hash, jobLockSQL string) error {
	// Serialize the two source roles for one packet even before either job row
	// exists. Once a replay sees an existing job, lock it before the packet so
	// assignment writes follow the same job -> packet order as worker state
	// transitions and cannot form a row-lock cycle with them.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 731927))`, guid.Hex()); err != nil {
		return err
	}
	var present bool
	err := tx.QueryRow(ctx, jobLockSQL, guid.Bytes()).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}
