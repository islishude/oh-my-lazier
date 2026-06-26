package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

// DVNJobRecord records an OpenDVN assignment for a known packet.
type DVNJobRecord struct {
	GUID                  common.Hash
	ConfirmationsRequired uint64
	Status                string
}

// UpsertDVNJob persists DVN assignment state for a packet.
func (s *Store) UpsertDVNJob(ctx context.Context, job DVNJobRecord) error {
	if job.GUID == (common.Hash{}) {
		return errors.New("dvn job guid is required")
	}
	if job.ConfirmationsRequired == 0 {
		return errors.New("dvn confirmations required is required")
	}
	if job.Status == "" {
		return errors.New("dvn job status is required")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dvn_jobs (guid, assigned, confirmations_required, status)
		VALUES ($1, true, $2, $3)
		ON CONFLICT (guid) DO UPDATE SET
			assigned = true,
			confirmations_required = EXCLUDED.confirmations_required,
			status = EXCLUDED.status,
			updated_at = now()
	`, job.GUID.Bytes(), job.ConfirmationsRequired, job.Status)
	return err
}

// ListDVNWork returns DVN jobs in one durable status with packet data needed to verify.
func (s *Store) ListDVNWork(ctx context.Context, status string, limit int) ([]DVNWorkItem, error) {
	if status == "" {
		return nil, errors.New("dvn status is required")
	}
	if limit <= 0 {
		return nil, errors.New("dvn work limit must be positive")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			p.guid, p.src_eid, p.dst_eid, p.nonce::text, p.sender, p.receiver,
			p.send_lib, p.src_tx_hash, p.src_block_number, p.src_log_index,
			p.encoded_packet, p.packet_header, p.message, p.payload_hash,
			p.options, p.status, dj.confirmations_required, dj.status
		FROM dvn_jobs dj
		JOIN packets p ON p.guid = dj.guid
		WHERE dj.status = $1 AND (dj.next_retry_at IS NULL OR dj.next_retry_at <= now())
		ORDER BY dj.updated_at, dj.guid
		LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	work := make([]DVNWorkItem, 0)
	for rows.Next() {
		var row dvnWorkRow
		if err := rows.Scan(
			&row.GUID,
			&row.SrcEID,
			&row.DstEID,
			&row.Nonce,
			&row.Sender,
			&row.Receiver,
			&row.SendLib,
			&row.SrcTxHash,
			&row.SrcBlockNumber,
			&row.SrcLogIndex,
			&row.EncodedPacket,
			&row.PacketHeader,
			&row.Message,
			&row.PayloadHash,
			&row.Options,
			&row.PacketStatus,
			&row.ConfirmationsRequired,
			&row.JobStatus,
		); err != nil {
			return nil, err
		}
		item, err := row.toDVNWorkItem()
		if err != nil {
			return nil, err
		}
		work = append(work, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return work, nil
}

// MarkDVNWaitingConfirmations records that the source packet has not reached required confirmations yet.
func (s *Store) MarkDVNWaitingConfirmations(ctx context.Context, guid common.Hash, expectedStatus string) error {
	return s.updateDVNStatus(ctx, dvnStatusUpdate{GUID: guid, ExpectedStatus: expectedStatus, NextStatus: string(packets.DVNWaitingConfirmations)})
}

// MarkDVNQuorumChecking records that a DVN job has enough source confirmations for quorum checks.
func (s *Store) MarkDVNQuorumChecking(ctx context.Context, guid common.Hash, expectedStatus string) error {
	return s.updateDVNStatus(ctx, dvnStatusUpdate{GUID: guid, ExpectedStatus: expectedStatus, NextStatus: string(packets.DVNQuorumChecking)})
}

// MarkDVNWouldVerify records a shadow-mode report for a packet that passed quorum checks.
func (s *Store) MarkDVNWouldVerify(ctx context.Context, guid common.Hash, expectedStatus string, quorumResult []byte) error {
	if len(quorumResult) == 0 {
		return errors.New("dvn quorum result is required")
	}
	return s.updateDVNStatus(ctx, dvnStatusUpdate{GUID: guid, ExpectedStatus: expectedStatus, NextStatus: string(packets.DVNWouldVerify), QuorumResult: quorumResult})
}

// MarkDVNQuorumConflict records a quorum verification conflict requiring operator review.
func (s *Store) MarkDVNQuorumConflict(ctx context.Context, guid common.Hash, expectedStatus, reason string, quorumResult []byte) error {
	if reason == "" {
		return errors.New("dvn quorum conflict reason is required")
	}
	return s.updateDVNStatus(ctx, dvnStatusUpdate{GUID: guid, ExpectedStatus: expectedStatus, NextStatus: string(packets.DVNQuorumConflict), LastError: reason, QuorumResult: quorumResult})
}

type dvnStatusUpdate struct {
	GUID           common.Hash
	ExpectedStatus string
	NextStatus     string
	LastError      string
	QuorumResult   []byte
}

func (s *Store) updateDVNStatus(ctx context.Context, update dvnStatusUpdate) error {
	if update.GUID == (common.Hash{}) {
		return errors.New("dvn job guid is required")
	}
	if update.ExpectedStatus == "" || update.NextStatus == "" {
		return errors.New("dvn transition statuses are required")
	}
	lastErrorArg := any(nil)
	if update.LastError != "" {
		lastErrorArg = update.LastError
	}
	quorumResultArg := any(nil)
	if len(update.QuorumResult) != 0 {
		quorumResultArg = string(update.QuorumResult)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE dvn_jobs
		SET
			status = $1,
			last_error = $4,
			quorum_result = COALESCE($5::jsonb, quorum_result),
			updated_at = now()
		WHERE guid = $2 AND status = $3
	`, update.NextStatus, update.GUID.Bytes(), update.ExpectedStatus, lastErrorArg, quorumResultArg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("dvn job %s is not in status %s", update.GUID, update.ExpectedStatus)
	}
	return nil
}

// DVNWorkItem is a packet plus its DVN job state selected for processing.
type DVNWorkItem struct {
	Packet PacketRecord
	Job    DVNJobRecord
}

type dvnWorkRow struct {
	GUID                  []byte
	SrcEID                uint32
	DstEID                uint32
	Nonce                 string
	Sender                []byte
	Receiver              []byte
	SendLib               []byte
	SrcTxHash             []byte
	SrcBlockNumber        uint64
	SrcLogIndex           uint
	EncodedPacket         []byte
	PacketHeader          []byte
	Message               []byte
	PayloadHash           []byte
	Options               []byte
	PacketStatus          string
	ConfirmationsRequired uint64
	JobStatus             string
}

func (r dvnWorkRow) toDVNWorkItem() (DVNWorkItem, error) {
	packet, err := packetRow{
		GUID:           r.GUID,
		SrcEID:         r.SrcEID,
		DstEID:         r.DstEID,
		Nonce:          r.Nonce,
		Sender:         r.Sender,
		Receiver:       r.Receiver,
		SendLib:        r.SendLib,
		SrcTxHash:      r.SrcTxHash,
		SrcBlockNumber: r.SrcBlockNumber,
		SrcLogIndex:    r.SrcLogIndex,
		EncodedPacket:  r.EncodedPacket,
		PacketHeader:   r.PacketHeader,
		Message:        r.Message,
		PayloadHash:    r.PayloadHash,
		Options:        r.Options,
		Status:         r.PacketStatus,
	}.toPacketRecord()
	if err != nil {
		return DVNWorkItem{}, err
	}
	if r.ConfirmationsRequired == 0 {
		return DVNWorkItem{}, fmt.Errorf("dvn work confirmations required is zero")
	}
	return DVNWorkItem{
		Packet: packet,
		Job: DVNJobRecord{
			GUID:                  packet.GUID,
			ConfirmationsRequired: r.ConfirmationsRequired,
			Status:                r.JobStatus,
		},
	}, nil
}
