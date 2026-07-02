package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

const (
	// TxStatusQueued means a transaction request is waiting for nonce assignment or re-signing.
	TxStatusQueued = "queued"
	// TxStatusNonceAssigned means the tx manager has reserved a nonce for signing.
	TxStatusNonceAssigned = "nonce_assigned"
	// TxStatusSigned means the transaction was signed but not yet broadcast.
	TxStatusSigned = "signed"
	// TxStatusBroadcast means the transaction was submitted and is awaiting a receipt.
	TxStatusBroadcast = "broadcast"
	// TxStatusConfirmed means the transaction receipt succeeded.
	TxStatusConfirmed = "confirmed"
	// TxStatusFailed means the transaction needs retry or manual review.
	TxStatusFailed = "failed"
)

const maxDBNonce = uint64(1<<63 - 1)

// ErrNonceCursorMissing indicates a signer has no durable local nonce cursor yet.
var ErrNonceCursorMissing = errors.New("tx nonce cursor missing")

// TxRequest describes a durable transaction request before nonce assignment.
type TxRequest struct {
	ChainEID uint32
	Purpose  string
	GUID     []byte
	To       common.Address
	Calldata []byte
	Value    *big.Int
	SignerID string
}

// ClaimedTx records the queued outbox row and nonce reserved by ClaimNextNonce.
type ClaimedTx struct {
	ID    int64
	Nonce uint64
}

// OutboxTx is a transaction request after it has been persisted.
type OutboxTx struct {
	ID                   int64
	ChainEID             uint32
	Purpose              string
	GUID                 []byte
	To                   common.Address
	Calldata             []byte
	Value                *big.Int
	GasLimit             uint64
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	Nonce                uint64
	TxHash               common.Hash
	SignerID             string
	Status               string
	Attempts             uint32
}

// QueuedOutboxTx is a queued transaction request before the tx manager decides whether to sign it.
type QueuedOutboxTx struct {
	ID                   int64
	ChainEID             uint32
	Purpose              string
	GUID                 []byte
	To                   common.Address
	Calldata             []byte
	Value                *big.Int
	GasLimit             uint64
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	Nonce                *uint64
	TxHash               common.Hash
	SignerID             string
	Status               string
	Attempts             uint32
}

// EnqueueTx inserts a transaction request into tx_outbox with queued status.
func (s *Store) EnqueueTx(ctx context.Context, request TxRequest) (int64, error) {
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

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tx_outbox (
			chain_eid, purpose, guid, to_address, calldata, value,
			signer_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, request.ChainEID, request.Purpose, optionalBytes(request.GUID), addressBytes(request.To), request.Calldata, value.String(), request.SignerID, TxStatusQueued).Scan(&id)
	return id, err
}

// PeekQueuedTx returns the next queued outbox row for one chain signer without reserving a nonce.
func (s *Store) PeekQueuedTx(ctx context.Context, chainEID uint32, signerID string) (QueuedOutboxTx, error) {
	if chainEID == 0 {
		return QueuedOutboxTx{}, errors.New("chain eid is required")
	}
	if signerID == "" {
		return QueuedOutboxTx{}, errors.New("signer id is required")
	}
	var row outboxTxRow
	err := s.pool.QueryRow(ctx, `
		SELECT
			id, chain_eid, purpose, guid, to_address, calldata, value::text,
			gas_limit::text, max_fee_per_gas::text, max_priority_fee_per_gas::text,
			nonce, tx_hash, signer_id, status, attempts
		FROM tx_outbox
		WHERE chain_eid = $1 AND signer_id = $2 AND status = $3
		ORDER BY id
		LIMIT 1
	`, chainEID, signerID, TxStatusQueued).Scan(
		&row.ID,
		&row.ChainEID,
		&row.Purpose,
		&row.GUID,
		&row.ToAddress,
		&row.Calldata,
		&row.Value,
		&row.GasLimit,
		&row.MaxFeePerGas,
		&row.MaxPriorityFeePerGas,
		&row.Nonce,
		&row.TxHash,
		&row.SignerID,
		&row.Status,
		&row.Attempts,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return QueuedOutboxTx{}, pgx.ErrNoRows
	}
	if err != nil {
		return QueuedOutboxTx{}, err
	}
	return row.toQueuedOutboxTx()
}

// ClaimNextNonce reserves the next local nonce for one queued outbox row.
//
// The transaction-scoped advisory lock serializes nonce assignment per
// (chain_eid, signer_id). If a queued replacement already has a nonce, that
// nonce is preserved; otherwise the assigned nonce comes from tx_nonce_cursors.
func (s *Store) ClaimNextNonce(ctx context.Context, chainEID uint32, signerID string) (ClaimedTx, error) {
	return s.claimQueuedNonce(ctx, 0, chainEID, signerID)
}

// ClaimTxNonce reserves a nonce for the selected queued outbox row.
func (s *Store) ClaimTxNonce(ctx context.Context, id int64, chainEID uint32, signerID string) (ClaimedTx, error) {
	if id <= 0 {
		return ClaimedTx{}, errors.New("outbox tx id is required")
	}
	return s.claimQueuedNonce(ctx, id, chainEID, signerID)
}

// BootstrapTxNonceCursor inserts a local signer nonce cursor when one does not exist.
//
// This is the only tx manager boundary that accepts an RPC nonce. Existing
// cursors are never updated from RPC; first-use bootstrap chooses the greater
// of the RPC pending nonce and all locally recorded outbox nonces plus one.
func (s *Store) BootstrapTxNonceCursor(ctx context.Context, chainEID uint32, signerID string, rpcPendingNonce uint64) (bool, error) {
	if chainEID == 0 {
		return false, errors.New("chain eid is required")
	}
	if signerID == "" {
		return false, errors.New("signer id is required")
	}
	if rpcPendingNonce > maxDBNonce {
		return false, fmt.Errorf("rpc pending nonce %d exceeds database nonce limit", rpcPendingNonce)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockSignerNonce(ctx, tx, chainEID, signerID); err != nil {
		return false, err
	}
	localNext, err := s.localNextNonce(ctx, tx, chainEID, signerID)
	if err != nil {
		return false, err
	}
	nextNonce := rpcPendingNonce
	if localNext > nextNonce {
		nextNonce = localNext
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO tx_nonce_cursors (chain_eid, signer_id, next_nonce)
		VALUES ($1, $2, $3)
		ON CONFLICT (chain_eid, signer_id) DO NOTHING
	`, chainEID, signerID, int64(nextNonce))
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) claimQueuedNonce(ctx context.Context, selectedID int64, chainEID uint32, signerID string) (ClaimedTx, error) {
	if chainEID == 0 {
		return ClaimedTx{}, errors.New("chain eid is required")
	}
	if signerID == "" {
		return ClaimedTx{}, errors.New("signer id is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ClaimedTx{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockSignerNonce(ctx, tx, chainEID, signerID); err != nil {
		return ClaimedTx{}, err
	}

	var id int64
	var assignedNonce *int64
	var queryErr error
	if selectedID > 0 {
		queryErr = tx.QueryRow(ctx, `
			SELECT id, nonce
			FROM tx_outbox
			WHERE id = $1 AND chain_eid = $2 AND signer_id = $3 AND status = $4
			FOR UPDATE
		`, selectedID, chainEID, signerID, TxStatusQueued).Scan(&id, &assignedNonce)
	} else {
		queryErr = tx.QueryRow(ctx, `
			SELECT id, nonce
			FROM tx_outbox
			WHERE chain_eid = $1 AND signer_id = $2 AND status = $3
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		`, chainEID, signerID, TxStatusQueued).Scan(&id, &assignedNonce)
	}
	if errors.Is(queryErr, pgx.ErrNoRows) {
		return ClaimedTx{}, pgx.ErrNoRows
	}
	if queryErr != nil {
		return ClaimedTx{}, queryErr
	}

	var nextNonce uint64
	if assignedNonce != nil {
		if *assignedNonce < 0 {
			return ClaimedTx{}, fmt.Errorf("negative nonce for chain %d signer %s", chainEID, signerID)
		}
		nextNonce = uint64(*assignedNonce)
	} else {
		nextNonce, err = s.claimCursorNonce(ctx, tx, chainEID, signerID)
		if err != nil {
			return ClaimedTx{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tx_outbox
		SET nonce = $1, status = $2, updated_at = now()
		WHERE id = $3
	`, nextNonce, TxStatusNonceAssigned, id); err != nil {
		return ClaimedTx{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ClaimedTx{}, err
	}
	return ClaimedTx{ID: id, Nonce: nextNonce}, nil
}

// GetOutboxTx returns one persisted transaction request.
func (s *Store) GetOutboxTx(ctx context.Context, id int64) (OutboxTx, error) {
	var row outboxTxRow
	err := s.pool.QueryRow(ctx, `
		SELECT
			id, chain_eid, purpose, guid, to_address, calldata, value::text,
			gas_limit::text, max_fee_per_gas::text, max_priority_fee_per_gas::text,
			nonce, tx_hash, signer_id, status, attempts
		FROM tx_outbox
		WHERE id = $1
	`, id).Scan(
		&row.ID,
		&row.ChainEID,
		&row.Purpose,
		&row.GUID,
		&row.ToAddress,
		&row.Calldata,
		&row.Value,
		&row.GasLimit,
		&row.MaxFeePerGas,
		&row.MaxPriorityFeePerGas,
		&row.Nonce,
		&row.TxHash,
		&row.SignerID,
		&row.Status,
		&row.Attempts,
	)
	if err != nil {
		return OutboxTx{}, err
	}
	return row.toOutboxTx()
}

// ListBroadcastTx returns broadcast transactions waiting for receipts for one chain signer.
func (s *Store) ListBroadcastTx(ctx context.Context, chainEID uint32, signerID string, limit int) ([]OutboxTx, error) {
	if chainEID == 0 {
		return nil, errors.New("chain eid is required")
	}
	if signerID == "" {
		return nil, errors.New("signer id is required")
	}
	if limit <= 0 {
		return nil, errors.New("broadcast tx limit must be positive")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			id, chain_eid, purpose, guid, to_address, calldata, value::text,
			gas_limit::text, max_fee_per_gas::text, max_priority_fee_per_gas::text,
			nonce, tx_hash, signer_id, status, attempts
		FROM tx_outbox
		WHERE chain_eid = $1 AND signer_id = $2 AND status = $3 AND tx_hash IS NOT NULL
		ORDER BY updated_at, id
		LIMIT $4
	`, chainEID, signerID, TxStatusBroadcast, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]OutboxTx, 0)
	for rows.Next() {
		var row outboxTxRow
		if err := rows.Scan(
			&row.ID,
			&row.ChainEID,
			&row.Purpose,
			&row.GUID,
			&row.ToAddress,
			&row.Calldata,
			&row.Value,
			&row.GasLimit,
			&row.MaxFeePerGas,
			&row.MaxPriorityFeePerGas,
			&row.Nonce,
			&row.TxHash,
			&row.SignerID,
			&row.Status,
			&row.Attempts,
		); err != nil {
			return nil, err
		}
		tx, err := row.toOutboxTx()
		if err != nil {
			return nil, err
		}
		out = append(out, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkTxSigned records that an outbox transaction was signed.
func (s *Store) MarkTxSigned(ctx context.Context, id int64, txHash common.Hash) error {
	return s.updateTxStatus(ctx, id, TxStatusSigned, txHash, "")
}

// MarkTxSignedWithGasAndFees records the signed transaction hash and exact gas settings used to sign it.
func (s *Store) MarkTxSignedWithGasAndFees(ctx context.Context, id int64, txHash common.Hash, gasLimit uint64, maxFeePerGas, maxPriorityFeePerGas *big.Int) error {
	if txHash == (common.Hash{}) {
		return errors.New("tx hash is required")
	}
	if gasLimit == 0 {
		return errors.New("gas limit is required")
	}
	if maxFeePerGas == nil || maxFeePerGas.Sign() <= 0 {
		return errors.New("max fee per gas is required")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tx_outbox
		SET
			status = $1,
			tx_hash = $2,
			gas_limit = $3,
			max_fee_per_gas = $4,
			max_priority_fee_per_gas = $5,
			last_error = NULL,
			updated_at = now()
		WHERE id = $6
	`, TxStatusSigned, txHash.Bytes(), gasLimit, maxFeePerGas.String(), numericString(maxPriorityFeePerGas), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox tx %d not found", id)
	}
	return nil
}

// MarkTxBroadcast records that an outbox transaction was broadcast.
func (s *Store) MarkTxBroadcast(ctx context.Context, id int64, txHash common.Hash) error {
	return s.updateTxStatus(ctx, id, TxStatusBroadcast, txHash, "")
}

// MarkTxConfirmed records that an outbox transaction receipt succeeded.
func (s *Store) MarkTxConfirmed(ctx context.Context, id int64, txHash common.Hash) error {
	return s.updateTxStatus(ctx, id, TxStatusConfirmed, txHash, "")
}

// MarkTxFailed records that an outbox transaction failed and is eligible for later retry policy.
func (s *Store) MarkTxFailed(ctx context.Context, id int64, failure error) error {
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	return s.updateTxStatus(ctx, id, TxStatusFailed, common.Hash{}, message)
}

// PrepareReplacementTx resets a transaction for re-signing while preserving its nonce and last signed fees.
func (s *Store) PrepareReplacementTx(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tx_outbox
		SET
			status = $1,
			tx_hash = NULL,
			attempts = attempts + 1,
			last_error = NULL,
			updated_at = now()
		WHERE id = $2 AND nonce IS NOT NULL
	`, TxStatusQueued, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox tx %d is not replaceable", id)
	}
	return nil
}

// RetryFailedTx returns a failed transaction request to the queue.
//
// Rows that already consumed a nonce are cloned to preserve the original nonce
// evidence. Rows without a nonce are requeued in place because no nonce was
// consumed.
func (s *Store) RetryFailedTx(ctx context.Context, id int64) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var assignedNonce *int64
	if err := tx.QueryRow(ctx, `
		SELECT nonce
		FROM tx_outbox
		WHERE id = $1 AND status = $2
		FOR UPDATE
	`, id, TxStatusFailed).Scan(&assignedNonce); errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("outbox tx %d is not failed", id)
	} else if err != nil {
		return 0, err
	}

	if assignedNonce == nil {
		tag, err := tx.Exec(ctx, `
			UPDATE tx_outbox
			SET
				status = $1,
				tx_hash = NULL,
				gas_limit = NULL,
				max_fee_per_gas = NULL,
				max_priority_fee_per_gas = NULL,
				attempts = attempts + 1,
				last_error = NULL,
				updated_at = now()
			WHERE id = $2 AND status = $3 AND nonce IS NULL
		`, TxStatusQueued, id, TxStatusFailed)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf("outbox tx %d is not failed without nonce", id)
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return id, nil
	}
	if *assignedNonce < 0 {
		return 0, fmt.Errorf("negative nonce for outbox tx %d", id)
	}

	var retryID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO tx_outbox (
			chain_eid, purpose, guid, to_address, calldata, value,
			signer_id, status, attempts
		)
		SELECT
			chain_eid, purpose, guid, to_address, calldata, value,
			signer_id, $1, attempts + 1
		FROM tx_outbox
		WHERE id = $2 AND status = $3 AND nonce IS NOT NULL
		RETURNING id
	`, TxStatusQueued, id, TxStatusFailed).Scan(&retryID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return retryID, nil
}

func lockSignerNonce(ctx context.Context, tx pgx.Tx, chainEID uint32, signerID string) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1::integer, hashtext($2)::integer)", int32(chainEID), signerID)
	return err
}

func (s *Store) localNextNonce(ctx context.Context, tx pgx.Tx, chainEID uint32, signerID string) (uint64, error) {
	var dbMax *int64
	if err := tx.QueryRow(ctx, `
		SELECT max(nonce)::bigint
		FROM tx_outbox
		WHERE chain_eid = $1 AND signer_id = $2 AND nonce IS NOT NULL
	`, chainEID, signerID).Scan(&dbMax); err != nil {
		return 0, err
	}
	if dbMax == nil {
		return 0, nil
	}
	if *dbMax < 0 {
		return 0, fmt.Errorf("negative nonce for chain %d signer %s", chainEID, signerID)
	}
	if uint64(*dbMax) >= maxDBNonce {
		return 0, fmt.Errorf("nonce overflow for chain %d signer %s", chainEID, signerID)
	}
	return uint64(*dbMax) + 1, nil
}

func (s *Store) claimCursorNonce(ctx context.Context, tx pgx.Tx, chainEID uint32, signerID string) (uint64, error) {
	var dbNext int64
	err := tx.QueryRow(ctx, `
		SELECT next_nonce
		FROM tx_nonce_cursors
		WHERE chain_eid = $1 AND signer_id = $2
		FOR UPDATE
	`, chainEID, signerID).Scan(&dbNext)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNonceCursorMissing
	}
	if err != nil {
		return 0, err
	}
	if dbNext < 0 {
		return 0, fmt.Errorf("negative nonce cursor for chain %d signer %s", chainEID, signerID)
	}
	if uint64(dbNext) >= maxDBNonce {
		return 0, fmt.Errorf("nonce cursor overflow for chain %d signer %s", chainEID, signerID)
	}
	nextNonce := uint64(dbNext)
	if _, err := tx.Exec(ctx, `
		UPDATE tx_nonce_cursors
		SET next_nonce = $1, updated_at = now()
		WHERE chain_eid = $2 AND signer_id = $3
	`, int64(nextNonce+1), chainEID, signerID); err != nil {
		return 0, err
	}
	return nextNonce, nil
}

func numericString(value *big.Int) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func optionalBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)
	return copied
}

func (s *Store) updateTxStatus(ctx context.Context, id int64, status string, txHash common.Hash, lastError string) error {
	var txHashArg any
	if txHash != (common.Hash{}) {
		hashBytes := txHash.Bytes()
		copied := make([]byte, len(hashBytes))
		copy(copied, hashBytes)
		txHashArg = copied
	}
	var lastErrorArg any
	if lastError != "" {
		lastErrorArg = lastError
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tx_outbox
		SET status = $1, tx_hash = COALESCE($2, tx_hash), last_error = $3, updated_at = now()
		WHERE id = $4
	`, status, txHashArg, lastErrorArg, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox tx %d not found", id)
	}
	return nil
}

type outboxTxRow struct {
	ID                   int64
	ChainEID             uint32
	Purpose              string
	GUID                 *[]byte
	ToAddress            []byte
	Calldata             []byte
	Value                string
	GasLimit             *string
	MaxFeePerGas         *string
	MaxPriorityFeePerGas *string
	Nonce                *int64
	TxHash               *[]byte
	SignerID             string
	Status               string
	Attempts             uint32
}

func (r outboxTxRow) toOutboxTx() (OutboxTx, error) {
	queued, err := r.toQueuedOutboxTx()
	if err != nil {
		return OutboxTx{}, err
	}
	nonce := uint64(0)
	if queued.Nonce != nil {
		nonce = *queued.Nonce
	}
	return OutboxTx{
		ID:                   queued.ID,
		ChainEID:             queued.ChainEID,
		Purpose:              queued.Purpose,
		GUID:                 queued.GUID,
		To:                   queued.To,
		Calldata:             queued.Calldata,
		Value:                queued.Value,
		GasLimit:             queued.GasLimit,
		MaxFeePerGas:         queued.MaxFeePerGas,
		MaxPriorityFeePerGas: queued.MaxPriorityFeePerGas,
		Nonce:                nonce,
		TxHash:               queued.TxHash,
		SignerID:             queued.SignerID,
		Status:               queued.Status,
		Attempts:             queued.Attempts,
	}, nil
}

func (r outboxTxRow) toQueuedOutboxTx() (QueuedOutboxTx, error) {
	value, err := parseBigInt("value", &r.Value, true)
	if err != nil {
		return QueuedOutboxTx{}, err
	}
	maxFeePerGas, err := parseBigInt("max_fee_per_gas", r.MaxFeePerGas, false)
	if err != nil {
		return QueuedOutboxTx{}, err
	}
	maxPriorityFeePerGas, err := parseBigInt("max_priority_fee_per_gas", r.MaxPriorityFeePerGas, false)
	if err != nil {
		return QueuedOutboxTx{}, err
	}
	gasLimit, err := parseUint64("gas_limit", r.GasLimit)
	if err != nil {
		return QueuedOutboxTx{}, err
	}
	var nonce *uint64
	if r.Nonce != nil {
		if *r.Nonce < 0 {
			return QueuedOutboxTx{}, fmt.Errorf("outbox tx nonce is negative: %d", *r.Nonce)
		}
		parsedNonce := uint64(*r.Nonce)
		nonce = &parsedNonce
	}
	if len(r.ToAddress) != common.AddressLength {
		return QueuedOutboxTx{}, fmt.Errorf("outbox tx to_address has length %d", len(r.ToAddress))
	}
	var txHash common.Hash
	if r.TxHash != nil {
		if len(*r.TxHash) != common.HashLength {
			return QueuedOutboxTx{}, fmt.Errorf("outbox tx tx_hash has length %d", len(*r.TxHash))
		}
		txHash = common.BytesToHash(*r.TxHash)
	}
	return QueuedOutboxTx{
		ID:                   r.ID,
		ChainEID:             r.ChainEID,
		Purpose:              r.Purpose,
		GUID:                 cloneOptionalBytes(r.GUID),
		To:                   common.BytesToAddress(r.ToAddress),
		Calldata:             bytes.Clone(r.Calldata),
		Value:                value,
		GasLimit:             gasLimit,
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		Nonce:                nonce,
		TxHash:               txHash,
		SignerID:             r.SignerID,
		Status:               r.Status,
		Attempts:             r.Attempts,
	}, nil
}

func parseBigInt(field string, value *string, required bool) (*big.Int, error) {
	if value == nil {
		if required {
			return nil, fmt.Errorf("%s is required", field)
		}
		return nil, nil
	}
	parsed, ok := new(big.Int).SetString(*value, 10)
	if !ok {
		return nil, fmt.Errorf("%s is not a valid integer", field)
	}
	return parsed, nil
}

func parseUint64(field string, value *string) (uint64, error) {
	if value == nil {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(*value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid uint64: %w", field, err)
	}
	return parsed, nil
}

func cloneOptionalBytes(value *[]byte) []byte {
	if value == nil {
		return nil
	}
	return bytes.Clone(*value)
}
