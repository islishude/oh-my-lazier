package db

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
)

const (
	// TxStatusQueued means a transaction request is waiting for nonce assignment.
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

var pendingNonceStatuses = []string{
	TxStatusNonceAssigned,
	TxStatusSigned,
	TxStatusBroadcast,
}

// TxRequest describes a durable transaction request before nonce assignment.
type TxRequest struct {
	ChainEID             uint32
	Purpose              string
	GUID                 []byte
	To                   common.Address
	Calldata             []byte
	Value                *big.Int
	GasLimit             *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	SignerID             string
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
			chain_eid, purpose, guid, to_address, calldata, value, gas_limit,
			max_fee_per_gas, max_priority_fee_per_gas, signer_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, request.ChainEID, request.Purpose, optionalBytes(request.GUID), addressBytes(request.To), request.Calldata, value.String(), numericString(request.GasLimit), numericString(request.MaxFeePerGas), numericString(request.MaxPriorityFeePerGas), request.SignerID, TxStatusQueued).Scan(&id)
	return id, err
}

// ClaimNextNonce reserves the next nonce for one queued outbox row.
//
// The transaction-scoped advisory lock serializes nonce assignment per
// (chain_eid, signer_id). The assigned nonce is max(rpcPendingNonce,
// highest locally pending nonce + 1).
func (s *Store) ClaimNextNonce(ctx context.Context, chainEID uint32, signerID string, rpcPendingNonce uint64) (ClaimedTx, error) {
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

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1::integer, hashtext($2)::integer)", int32(chainEID), signerID); err != nil {
		return ClaimedTx{}, err
	}

	var id int64
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM tx_outbox
		WHERE chain_eid = $1 AND signer_id = $2 AND status = $3
		ORDER BY id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, chainEID, signerID, TxStatusQueued).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ClaimedTx{}, pgx.ErrNoRows
	}
	if err != nil {
		return ClaimedTx{}, err
	}

	nextNonce, err := s.nextNonce(ctx, tx, chainEID, signerID, rpcPendingNonce)
	if err != nil {
		return ClaimedTx{}, err
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
			nonce, signer_id, status, attempts
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
		&row.SignerID,
		&row.Status,
		&row.Attempts,
	)
	if err != nil {
		return OutboxTx{}, err
	}
	return row.toOutboxTx()
}

// MarkTxSigned records that an outbox transaction was signed.
func (s *Store) MarkTxSigned(ctx context.Context, id int64, txHash common.Hash) error {
	return s.updateTxStatus(ctx, id, TxStatusSigned, txHash, "")
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

// PrepareReplacementTx bumps fees on a nonce-assigned transaction while preserving its nonce.
func (s *Store) PrepareReplacementTx(ctx context.Context, id int64, maxFeePerGas, maxPriorityFeePerGas *big.Int) error {
	if maxFeePerGas == nil || maxFeePerGas.Sign() <= 0 {
		return errors.New("replacement max fee per gas is required")
	}
	if maxPriorityFeePerGas == nil || maxPriorityFeePerGas.Sign() <= 0 {
		return errors.New("replacement max priority fee per gas is required")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tx_outbox
		SET
			max_fee_per_gas = $1,
			max_priority_fee_per_gas = $2,
			status = $3,
			tx_hash = NULL,
			attempts = attempts + 1,
			last_error = NULL,
			updated_at = now()
		WHERE id = $4 AND nonce IS NOT NULL
	`, maxFeePerGas.String(), maxPriorityFeePerGas.String(), TxStatusNonceAssigned, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("outbox tx %d is not replaceable", id)
	}
	return nil
}

func (s *Store) nextNonce(ctx context.Context, tx pgx.Tx, chainEID uint32, signerID string, rpcPendingNonce uint64) (uint64, error) {
	var dbMax *int64
	if err := tx.QueryRow(ctx, `
		SELECT max(nonce)::bigint
		FROM tx_outbox
		WHERE chain_eid = $1 AND signer_id = $2 AND status = ANY($3)
	`, chainEID, signerID, pendingNonceStatuses).Scan(&dbMax); err != nil {
		return 0, err
	}
	if dbMax == nil {
		return rpcPendingNonce, nil
	}
	if *dbMax < 0 {
		return 0, fmt.Errorf("negative nonce for chain %d signer %s", chainEID, signerID)
	}
	if *dbMax == int64(^uint64(0)>>1) {
		return 0, fmt.Errorf("nonce overflow for chain %d signer %s", chainEID, signerID)
	}
	localNext := uint64(*dbMax) + 1
	if rpcPendingNonce > localNext {
		return rpcPendingNonce, nil
	}
	return localNext, nil
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
	SignerID             string
	Status               string
	Attempts             uint32
}

func (r outboxTxRow) toOutboxTx() (OutboxTx, error) {
	value, err := parseBigInt("value", &r.Value, true)
	if err != nil {
		return OutboxTx{}, err
	}
	maxFeePerGas, err := parseBigInt("max_fee_per_gas", r.MaxFeePerGas, false)
	if err != nil {
		return OutboxTx{}, err
	}
	maxPriorityFeePerGas, err := parseBigInt("max_priority_fee_per_gas", r.MaxPriorityFeePerGas, false)
	if err != nil {
		return OutboxTx{}, err
	}
	gasLimit, err := parseUint64("gas_limit", r.GasLimit)
	if err != nil {
		return OutboxTx{}, err
	}
	if r.Nonce == nil {
		return OutboxTx{}, errors.New("outbox tx nonce is not assigned")
	}
	if *r.Nonce < 0 {
		return OutboxTx{}, fmt.Errorf("outbox tx nonce is negative: %d", *r.Nonce)
	}
	if len(r.ToAddress) != common.AddressLength {
		return OutboxTx{}, fmt.Errorf("outbox tx to_address has length %d", len(r.ToAddress))
	}
	return OutboxTx{
		ID:                   r.ID,
		ChainEID:             r.ChainEID,
		Purpose:              r.Purpose,
		GUID:                 cloneOptionalBytes(r.GUID),
		To:                   common.BytesToAddress(r.ToAddress),
		Calldata:             cloneBytes(r.Calldata),
		Value:                value,
		GasLimit:             gasLimit,
		MaxFeePerGas:         maxFeePerGas,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		Nonce:                uint64(*r.Nonce),
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

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)
	return copied
}

func cloneOptionalBytes(value *[]byte) []byte {
	if value == nil {
		return nil
	}
	return cloneBytes(*value)
}
