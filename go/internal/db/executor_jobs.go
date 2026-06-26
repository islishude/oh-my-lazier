package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/jackc/pgx/v5"
)

// ExecutorJobRecord records an OpenExecutor assignment for a known packet.
type ExecutorJobRecord struct {
	GUID        common.Hash
	AssignedFee *big.Int
	Status      string
	LastError   string
}

// ExecutorWorkItem is a packet plus its executor job state selected for processing.
type ExecutorWorkItem struct {
	Packet PacketRecord
	Job    ExecutorJobRecord
}

// UpsertExecutorJob persists executor assignment state for a packet.
func (s *Store) UpsertExecutorJob(ctx context.Context, job ExecutorJobRecord) error {
	if job.GUID == (common.Hash{}) {
		return errors.New("executor job guid is required")
	}
	if job.AssignedFee != nil && job.AssignedFee.Sign() < 0 {
		return errors.New("executor assigned fee must be non-negative")
	}
	if job.Status == "" {
		return errors.New("executor job status is required")
	}
	var fee any
	if job.AssignedFee != nil {
		fee = job.AssignedFee.String()
	}
	var lastError any
	if job.LastError != "" {
		lastError = job.LastError
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO executor_jobs (guid, assigned, assigned_fee, status, last_error)
		VALUES ($1, true, $2, $3, $4)
		ON CONFLICT (guid) DO UPDATE SET
			assigned = true,
			assigned_fee = EXCLUDED.assigned_fee,
			status = EXCLUDED.status,
			last_error = EXCLUDED.last_error,
			updated_at = now()
	`, job.GUID.Bytes(), fee, job.Status, lastError)
	return err
}

// ListExecutorWork returns executor jobs in one durable status with the packet data needed to act.
func (s *Store) ListExecutorWork(ctx context.Context, status string, limit int) ([]ExecutorWorkItem, error) {
	if status == "" {
		return nil, errors.New("executor status is required")
	}
	if limit <= 0 {
		return nil, errors.New("executor work limit must be positive")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			p.guid, p.src_eid, p.dst_eid, p.nonce::text, p.sender, p.receiver,
			p.send_lib, p.src_tx_hash, p.src_block_number, p.src_log_index,
			p.encoded_packet, p.packet_header, p.message, p.payload_hash,
			p.options, p.status, ej.assigned_fee::text, ej.status
		FROM executor_jobs ej
		JOIN packets p ON p.guid = ej.guid
		WHERE ej.status = $1 AND (ej.next_retry_at IS NULL OR ej.next_retry_at <= now())
		ORDER BY ej.updated_at, ej.guid
		LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	work := make([]ExecutorWorkItem, 0)
	for rows.Next() {
		var row executorWorkRow
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
			&row.AssignedFee,
			&row.JobStatus,
		); err != nil {
			return nil, err
		}
		item, err := row.toExecutorWorkItem()
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

// EnqueueExecutorTx inserts an outbox tx and advances packet/job status atomically.
func (s *Store) EnqueueExecutorTx(ctx context.Context, guid common.Hash, expectedStatus, nextStatus string, request TxRequest) (int64, error) {
	if guid == (common.Hash{}) {
		return 0, errors.New("executor job guid is required")
	}
	if expectedStatus == "" || nextStatus == "" {
		return 0, errors.New("executor transition statuses are required")
	}
	if len(request.GUID) == 0 {
		request.GUID = guid.Bytes()
	}
	if len(request.GUID) != common.HashLength || common.BytesToHash(request.GUID) != guid {
		return 0, errors.New("tx request guid does not match executor job")
	}
	if request.ChainEID == 0 {
		return 0, errors.New("chain eid is required")
	}
	if request.Purpose == "" {
		return 0, errors.New("purpose is required")
	}
	if request.To == (common.Address{}) {
		return 0, errors.New("to address is required")
	}
	if request.SignerID == "" {
		return 0, errors.New("signer id is required")
	}
	value := request.Value
	if value == nil {
		value = new(big.Int)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM executor_jobs
		WHERE guid = $1
		FOR UPDATE
	`, guid.Bytes()).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("executor job %s not found", guid)
	}
	if err != nil {
		return 0, err
	}
	if currentStatus != expectedStatus {
		return 0, fmt.Errorf("executor job %s status is %s, want %s", guid, currentStatus, expectedStatus)
	}

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO tx_outbox (
			chain_eid, purpose, guid, to_address, calldata, value, gas_limit,
			max_fee_per_gas, max_priority_fee_per_gas, signer_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, request.ChainEID, request.Purpose, optionalBytes(request.GUID), addressBytes(request.To), request.Calldata, value.String(), numericString(request.GasLimit), numericString(request.MaxFeePerGas), numericString(request.MaxPriorityFeePerGas), request.SignerID, TxStatusQueued).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE executor_jobs
		SET status = $1, updated_at = now()
		WHERE guid = $2
	`, nextStatus, guid.Bytes()); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE packets
		SET status = $1, updated_at = now()
		WHERE guid = $2
	`, nextStatus, guid.Bytes()); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

// MarkExecutorCommitted records a successful commitVerification receipt.
func (s *Store) MarkExecutorCommitted(ctx context.Context, guid, txHash common.Hash) error {
	if txHash == (common.Hash{}) {
		return errors.New("executor commit tx hash is required")
	}
	return s.updateExecutorStatus(ctx, executorStatusUpdate{
		GUID:              guid,
		ExpectedStatus:    string(packets.ExecutorCommitTxEnqueued),
		NextStatus:        string(packets.ExecutorCommitted),
		CommitTxHashBytes: txHash.Bytes(),
	})
}

// MarkExecutorExecutable records that endpoint state allows lzReceive delivery.
func (s *Store) MarkExecutorExecutable(ctx context.Context, guid common.Hash) error {
	return s.updateExecutorStatus(ctx, executorStatusUpdate{
		GUID:           guid,
		ExpectedStatus: string(packets.ExecutorCommitted),
		NextStatus:     string(packets.ExecutorExecutable),
	})
}

// MarkExecutorDelivered records a successful lzReceive receipt or PacketDelivered event.
func (s *Store) MarkExecutorDelivered(ctx context.Context, guid, txHash common.Hash) error {
	if txHash == (common.Hash{}) {
		return errors.New("executor receive tx hash is required")
	}
	return s.updateExecutorStatus(ctx, executorStatusUpdate{
		GUID:               guid,
		ExpectedStatus:     string(packets.ExecutorLzReceiveTxEnqueued),
		NextStatus:         string(packets.ExecutorDelivered),
		ReceiveTxHashBytes: txHash.Bytes(),
	})
}

// MarkExecutorReceiveFailed records an LzReceiveAlert or failed lzReceive receipt.
func (s *Store) MarkExecutorReceiveFailed(ctx context.Context, guid, txHash common.Hash, reason string) error {
	if txHash == (common.Hash{}) {
		return errors.New("executor receive tx hash is required")
	}
	return s.updateExecutorStatus(ctx, executorStatusUpdate{
		GUID:               guid,
		ExpectedStatus:     string(packets.ExecutorLzReceiveTxEnqueued),
		NextStatus:         string(packets.ExecutorLzReceiveFailed),
		ReceiveTxHashBytes: txHash.Bytes(),
		LastError:          reason,
	})
}

type executorStatusUpdate struct {
	GUID               common.Hash
	ExpectedStatus     string
	NextStatus         string
	CommitTxHashBytes  []byte
	ReceiveTxHashBytes []byte
	LastError          string
}

func (s *Store) updateExecutorStatus(ctx context.Context, update executorStatusUpdate) error {
	if update.GUID == (common.Hash{}) {
		return errors.New("executor job guid is required")
	}
	if update.ExpectedStatus == "" || update.NextStatus == "" {
		return errors.New("executor transition statuses are required")
	}
	if len(update.CommitTxHashBytes) != 0 && len(update.CommitTxHashBytes) != common.HashLength {
		return errors.New("executor commit tx hash must be 32 bytes")
	}
	if len(update.ReceiveTxHashBytes) != 0 && len(update.ReceiveTxHashBytes) != common.HashLength {
		return errors.New("executor receive tx hash must be 32 bytes")
	}
	lastErrorArg := any(nil)
	if update.LastError != "" {
		lastErrorArg = update.LastError
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE executor_jobs
		SET
			status = $1,
			commit_tx_hash = COALESCE($4, commit_tx_hash),
			receive_tx_hash = COALESCE($5, receive_tx_hash),
			last_error = $6,
			updated_at = now()
		WHERE guid = $2 AND status = $3
	`, update.NextStatus, update.GUID.Bytes(), update.ExpectedStatus, optionalBytes(update.CommitTxHashBytes), optionalBytes(update.ReceiveTxHashBytes), lastErrorArg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("executor job %s is not in status %s", update.GUID, update.ExpectedStatus)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE packets
		SET status = $1, updated_at = now()
		WHERE guid = $2
	`, update.NextStatus, update.GUID.Bytes()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type executorWorkRow struct {
	GUID           []byte
	SrcEID         uint32
	DstEID         uint32
	Nonce          string
	Sender         []byte
	Receiver       []byte
	SendLib        []byte
	SrcTxHash      []byte
	SrcBlockNumber uint64
	SrcLogIndex    uint
	EncodedPacket  []byte
	PacketHeader   []byte
	Message        []byte
	PayloadHash    []byte
	Options        []byte
	PacketStatus   string
	AssignedFee    *string
	JobStatus      string
}

func (r executorWorkRow) toExecutorWorkItem() (ExecutorWorkItem, error) {
	if len(r.GUID) != common.HashLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work guid has length %d", len(r.GUID))
	}
	if len(r.Sender) != common.AddressLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work sender has length %d", len(r.Sender))
	}
	if len(r.Receiver) != common.AddressLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work receiver has length %d", len(r.Receiver))
	}
	if len(r.SendLib) != common.AddressLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work send_lib has length %d", len(r.SendLib))
	}
	if len(r.SrcTxHash) != common.HashLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work src_tx_hash has length %d", len(r.SrcTxHash))
	}
	if len(r.PayloadHash) != common.HashLength {
		return ExecutorWorkItem{}, fmt.Errorf("executor work payload_hash has length %d", len(r.PayloadHash))
	}
	nonce, err := parseBigInt("packet nonce", &r.Nonce, true)
	if err != nil {
		return ExecutorWorkItem{}, err
	}
	assignedFee, err := parseBigInt("assigned_fee", r.AssignedFee, false)
	if err != nil {
		return ExecutorWorkItem{}, err
	}
	guid := common.BytesToHash(r.GUID)
	packet := PacketRecord{
		GUID:           guid,
		SrcEID:         r.SrcEID,
		DstEID:         r.DstEID,
		Nonce:          nonce,
		Sender:         common.BytesToAddress(r.Sender),
		Receiver:       common.BytesToAddress(r.Receiver),
		SendLib:        common.BytesToAddress(r.SendLib),
		SrcTxHash:      common.BytesToHash(r.SrcTxHash),
		SrcBlockNumber: r.SrcBlockNumber,
		SrcLogIndex:    r.SrcLogIndex,
		EncodedPacket:  cloneBytes(r.EncodedPacket),
		PacketHeader:   cloneBytes(r.PacketHeader),
		Message:        cloneBytes(r.Message),
		PayloadHash:    common.BytesToHash(r.PayloadHash),
		Options:        cloneBytes(r.Options),
		Status:         r.PacketStatus,
	}
	return ExecutorWorkItem{
		Packet: packet,
		Job: ExecutorJobRecord{
			GUID:        guid,
			AssignedFee: assignedFee,
			Status:      r.JobStatus,
		},
	}, nil
}
