package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

// SourcePacketSkip records a source-chain assignment that this worker deliberately did not persist.
type SourcePacketSkip struct {
	Role           string
	SrcEID         uint32
	DstEID         uint32
	Nonce          uint64
	Sender         common.Address
	Receiver       common.Address
	GUID           common.Hash
	SrcTxHash      common.Hash
	SrcBlockNumber uint64
	SrcLogIndex    uint
	Reason         string
	Worker         common.Address
}

// RecordSourcePacketSkip stores a durable tombstone for a skipped source assignment.
func (s *Store) RecordSourcePacketSkip(ctx context.Context, skip SourcePacketSkip) error {
	if err := validateSourcePacketSkip(skip); err != nil {
		return err
	}
	var worker any
	if skip.Worker != (common.Address{}) {
		worker = skip.Worker.Bytes()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO source_packet_skips (
			role, src_eid, dst_eid, nonce, sender, receiver, guid,
			src_tx_hash, src_block_number, src_log_index, reason, worker
		)
		VALUES ($1, $2, $3, $4::numeric, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (role, src_eid, dst_eid, sender, receiver, nonce) DO UPDATE SET
			guid = EXCLUDED.guid,
			src_tx_hash = EXCLUDED.src_tx_hash,
			src_block_number = EXCLUDED.src_block_number,
			src_log_index = EXCLUDED.src_log_index,
			reason = EXCLUDED.reason,
			worker = EXCLUDED.worker,
			updated_at = now()
	`, skip.Role, skip.SrcEID, skip.DstEID, new(big.Int).SetUint64(skip.Nonce).String(), skip.Sender.Bytes(), skip.Receiver.Bytes(), skip.GUID.Bytes(), skip.SrcTxHash.Bytes(), skip.SrcBlockNumber, skip.SrcLogIndex, skip.Reason, worker)
	return err
}

// GetSourcePacketSkip returns a skipped source-assignment tombstone by packet identity.
func (s *Store) GetSourcePacketSkip(ctx context.Context, role string, srcEID, dstEID uint32, sender, receiver common.Address, nonce uint64) (SourcePacketSkip, error) {
	if role == "" {
		return SourcePacketSkip{}, errors.New("source packet skip role is required")
	}
	if srcEID == 0 || dstEID == 0 {
		return SourcePacketSkip{}, errors.New("source packet skip eids are required")
	}
	if sender == (common.Address{}) || receiver == (common.Address{}) {
		return SourcePacketSkip{}, errors.New("source packet skip sender and receiver are required")
	}
	if nonce == 0 {
		return SourcePacketSkip{}, errors.New("source packet skip nonce is required")
	}
	var row struct {
		role           string
		srcEID         uint32
		dstEID         uint32
		nonce          string
		sender         []byte
		receiver       []byte
		guid           []byte
		srcTxHash      []byte
		srcBlockNumber uint64
		srcLogIndex    uint
		reason         string
		worker         []byte
	}
	err := s.pool.QueryRow(ctx, `
		SELECT role, src_eid, dst_eid, nonce::text, sender, receiver, guid,
			src_tx_hash, src_block_number, src_log_index, reason, COALESCE(worker, ''::bytea)
		FROM source_packet_skips
		WHERE role = $1
			AND src_eid = $2
			AND dst_eid = $3
			AND sender = $4
			AND receiver = $5
			AND nonce = $6::numeric
	`, role, srcEID, dstEID, sender.Bytes(), receiver.Bytes(), new(big.Int).SetUint64(nonce).String()).Scan(
		&row.role,
		&row.srcEID,
		&row.dstEID,
		&row.nonce,
		&row.sender,
		&row.receiver,
		&row.guid,
		&row.srcTxHash,
		&row.srcBlockNumber,
		&row.srcLogIndex,
		&row.reason,
		&row.worker,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourcePacketSkip{}, pgx.ErrNoRows
	}
	if err != nil {
		return SourcePacketSkip{}, err
	}
	parsedNonce, ok := new(big.Int).SetString(row.nonce, 10)
	if !ok || !parsedNonce.IsUint64() {
		return SourcePacketSkip{}, fmt.Errorf("source packet skip nonce %q is invalid", row.nonce)
	}
	skip := SourcePacketSkip{
		Role:           row.role,
		SrcEID:         row.srcEID,
		DstEID:         row.dstEID,
		Nonce:          parsedNonce.Uint64(),
		Sender:         common.BytesToAddress(row.sender),
		Receiver:       common.BytesToAddress(row.receiver),
		GUID:           common.BytesToHash(row.guid),
		SrcTxHash:      common.BytesToHash(row.srcTxHash),
		SrcBlockNumber: row.srcBlockNumber,
		SrcLogIndex:    row.srcLogIndex,
		Reason:         row.reason,
	}
	if len(row.worker) > 0 {
		skip.Worker = common.BytesToAddress(row.worker)
	}
	return skip, nil
}

func validateSourcePacketSkip(skip SourcePacketSkip) error {
	if skip.Role == "" {
		return errors.New("source packet skip role is required")
	}
	if skip.SrcEID == 0 || skip.DstEID == 0 {
		return errors.New("source packet skip eids are required")
	}
	if skip.Nonce == 0 {
		return errors.New("source packet skip nonce is required")
	}
	if skip.Sender == (common.Address{}) || skip.Receiver == (common.Address{}) {
		return errors.New("source packet skip sender and receiver are required")
	}
	if skip.GUID == (common.Hash{}) {
		return errors.New("source packet skip guid is required")
	}
	if skip.SrcTxHash == (common.Hash{}) {
		return errors.New("source packet skip tx hash is required")
	}
	if skip.Reason == "" {
		return errors.New("source packet skip reason is required")
	}
	return nil
}
