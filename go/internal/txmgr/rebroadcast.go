package txmgr

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

// RebroadcastStore fences an operator send and durably records its outcome.
type RebroadcastStore interface {
	GetRebroadcastAttempt(context.Context, int64) (db.RebroadcastAttempt, error)
	ClaimManualRebroadcast(context.Context, db.RebroadcastAttempt, uuid.UUID, time.Duration) error
	MarkAttemptSendResult(context.Context, int64, uuid.UUID, string, string) error
}

// TransactionSender submits an already signed transaction without loading a signer.
type TransactionSender interface {
	SendTransaction(context.Context, *types.Transaction) error
}

// RebroadcastResult contains only safe identifiers and canonical send diagnostics.
type RebroadcastResult struct {
	Action    string      `json:"action"`
	OutboxID  int64       `json:"outbox_id"`
	AttemptID int64       `json:"attempt_id"`
	TxHash    common.Hash `json:"tx_hash"`
	SendClass string      `json:"send_class"`
	Detail    string      `json:"detail"`
	Recorded  bool        `json:"recorded"`
}

// Rebroadcast validates, reserves and sends exactly one persisted active attempt.
// The caller must first validate the RPC's chain ID against chainID.
func Rebroadcast(ctx context.Context, store RebroadcastStore, client TransactionSender, a db.RebroadcastAttempt, chainID uint64) (RebroadcastResult, error) {
	result := RebroadcastResult{Action: "rebroadcast", OutboxID: a.OutboxID, AttemptID: a.AttemptID, TxHash: a.TxHash}
	tx, err := validateRebroadcast(a, chainID)
	if err != nil {
		return result, err
	}
	token := uuid.New()
	if err = store.ClaimManualRebroadcast(ctx, a, token, DefaultBroadcastLeaseTTL); err != nil {
		return result, errors.New("rebroadcast claim refused: attempt changed, lane blocked, lease busy, or database unavailable")
	}
	class, detail := sendPersistedTransaction(ctx, client, tx, DefaultSendTimeout)
	result.SendClass = class
	result.Detail = detail
	// A canceled RPC still needs its ambiguous outcome recorded. Bound cleanup
	// independently; it remains well inside the broadcast lease.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultSendTimeout)
	defer cancel()
	if err = store.MarkAttemptSendResult(recordCtx, a.AttemptID, token, class, detail); err != nil {
		return result, errors.New("transaction may have been accepted; send result could not be recorded; inspect before retrying")
	}
	result.Recorded = true
	if class != db.SendErrorAccepted {
		return result, errors.New("rebroadcast not acknowledged as accepted; inspect send_class and receipt status")
	}
	return result, nil
}

func validateRebroadcast(a db.RebroadcastAttempt, chainID uint64) (*types.Transaction, error) {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(a.RawTx); err != nil {
		return nil, errors.New("persisted transaction cannot be decoded")
	}
	encoded, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(encoded, a.RawTx) {
		return nil, errors.New("persisted transaction is not canonical")
	}
	if chainID == 0 || !tx.Protected() || tx.ChainId().Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return nil, errors.New("persisted transaction chain ID mismatch")
	}
	if tx.Hash() != a.TxHash || tx.Nonce() != a.Nonce {
		return nil, errors.New("persisted transaction hash or nonce mismatch")
	}
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil || !common.IsHexAddress(a.SignerID) || sender != common.HexToAddress(a.SignerID) {
		return nil, errors.New("persisted transaction signature or sender mismatch")
	}
	return tx, nil
}

func sendPersistedTransaction(ctx context.Context, client TransactionSender, tx *types.Transaction, timeout time.Duration) (string, string) {
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return classifyBroadcastError(client.SendTransaction(sendCtx, tx))
}
