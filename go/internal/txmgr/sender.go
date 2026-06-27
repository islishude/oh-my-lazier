package txmgr

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/signer"
	"github.com/jackc/pgx/v5"
)

const (
	executorCommitVerificationPurpose = "executor_commit_verification"
	executorLzReceivePurpose          = "executor_lz_receive"
)

// ChainClient is the tx manager's RPC boundary for nonce reads and broadcasts.
type ChainClient interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// ErrNoQueuedTx indicates no queued outbox row exists for the signer on a chain.
var ErrNoQueuedTx = errors.New("no queued tx")

// ErrNoReceiptUpdate indicates no broadcast tx receipt changed durable state.
var ErrNoReceiptUpdate = errors.New("no receipt update")

// ProcessNext signs and broadcasts one queued outbox transaction for a signer.
func (m *Manager) ProcessNext(ctx context.Context, chainEID uint32, chainID *big.Int, signer signer.Signer, client ChainClient) (int64, error) {
	if chainID == nil || chainID.Sign() <= 0 {
		return 0, errors.New("chain id is required")
	}
	rpcNonce, err := client.PendingNonceAt(ctx, signer.Address())
	if err != nil {
		return 0, err
	}
	claimed, err := m.store.ClaimNextNonce(ctx, chainEID, signer.Address().Hex(), rpcNonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoQueuedTx
	}
	if err != nil {
		return 0, err
	}
	outboxTx, err := m.store.GetOutboxTx(ctx, claimed.ID)
	if err != nil {
		return 0, err
	}
	signed, err := signOutboxTx(ctx, outboxTx, chainID, signer)
	if err != nil {
		_ = m.store.MarkTxFailed(ctx, outboxTx.ID, err)
		return 0, err
	}
	if err := m.store.MarkTxSigned(ctx, outboxTx.ID, signed.Hash()); err != nil {
		return 0, err
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		_ = m.store.MarkTxFailed(ctx, outboxTx.ID, err)
		return 0, err
	}
	if err := m.store.MarkTxBroadcast(ctx, outboxTx.ID, signed.Hash()); err != nil {
		return 0, err
	}
	return outboxTx.ID, nil
}

// ProcessReceipts checks broadcast transactions and marks mined receipts as confirmed or failed.
func (m *Manager) ProcessReceipts(ctx context.Context, target Target, limit int) (int64, error) {
	if target.Signer == nil {
		return 0, errors.New("target signer is required")
	}
	if target.Client == nil {
		return 0, errors.New("target client is required")
	}
	broadcasts, err := m.store.ListBroadcastTx(ctx, target.ChainEID, target.Signer.Address().Hex(), limit)
	if err != nil {
		return 0, err
	}
	for _, outboxTx := range broadcasts {
		receipt, err := target.Client.TransactionReceipt(ctx, outboxTx.TxHash)
		if errors.Is(err, ethereum.NotFound) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if receipt.Status == types.ReceiptStatusSuccessful {
			if err := m.applyWorkflowReceipt(ctx, outboxTx, true); err != nil {
				return 0, err
			}
			if err := m.store.MarkTxConfirmed(ctx, outboxTx.ID, outboxTx.TxHash); err != nil {
				return 0, err
			}
			return outboxTx.ID, nil
		}
		if err := m.applyWorkflowReceipt(ctx, outboxTx, false); err != nil {
			return 0, err
		}
		if err := m.store.MarkTxFailed(ctx, outboxTx.ID, fmt.Errorf("transaction receipt status %d", receipt.Status)); err != nil {
			return 0, err
		}
		return outboxTx.ID, nil
	}
	return 0, ErrNoReceiptUpdate
}

func (m *Manager) applyWorkflowReceipt(ctx context.Context, outboxTx db.OutboxTx, success bool) error {
	if len(outboxTx.GUID) != common.HashLength {
		return nil
	}
	guid := common.BytesToHash(outboxTx.GUID)
	switch outboxTx.Purpose {
	case executorCommitVerificationPurpose:
		if success {
			return m.markExecutorReceipt(ctx, guid, func() error {
				return m.store.MarkExecutorCommitted(ctx, guid, outboxTx.TxHash)
			}, packets.ExecutorCommitted, packets.ExecutorExecutable, packets.ExecutorLzReceiveTxEnqueued, packets.ExecutorLzReceiveFailed, packets.ExecutorDelivered)
		}
	case executorLzReceivePurpose:
		if success {
			return m.markExecutorReceipt(ctx, guid, func() error {
				return m.store.MarkExecutorDelivered(ctx, guid, outboxTx.TxHash)
			}, packets.ExecutorDelivered)
		}
		return m.markExecutorReceipt(ctx, guid, func() error {
			return m.store.MarkExecutorReceiveFailed(ctx, guid, outboxTx.TxHash, "lzReceive transaction reverted")
		}, packets.ExecutorLzReceiveFailed)
	}
	return nil
}

func (m *Manager) markExecutorReceipt(ctx context.Context, guid common.Hash, mark func() error, alreadyApplied ...packets.ExecutorState) error {
	if m.executorStatusMatches(ctx, guid, alreadyApplied) {
		return nil
	}
	if err := mark(); err != nil {
		if m.executorStatusMatches(ctx, guid, alreadyApplied) {
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) executorStatusMatches(ctx context.Context, guid common.Hash, statuses []packets.ExecutorState) bool {
	packet, err := m.store.GetPacket(ctx, guid)
	if err != nil {
		return false
	}
	for _, status := range statuses {
		if packet.Status == string(status) {
			return true
		}
	}
	return false
}

func signOutboxTx(ctx context.Context, outboxTx db.OutboxTx, chainID *big.Int, signer signer.Signer) (*types.Transaction, error) {
	if outboxTx.Status != db.TxStatusNonceAssigned {
		return nil, fmt.Errorf("outbox tx %d status %q is not signable", outboxTx.ID, outboxTx.Status)
	}
	if outboxTx.GasLimit == 0 {
		return nil, fmt.Errorf("outbox tx %d gas limit is required", outboxTx.ID)
	}
	if outboxTx.MaxFeePerGas == nil || outboxTx.MaxFeePerGas.Sign() <= 0 {
		return nil, fmt.Errorf("outbox tx %d max fee per gas is required", outboxTx.ID)
	}
	if outboxTx.MaxPriorityFeePerGas == nil || outboxTx.MaxPriorityFeePerGas.Sign() <= 0 {
		return nil, fmt.Errorf("outbox tx %d max priority fee per gas is required", outboxTx.ID)
	}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     outboxTx.Nonce,
		GasTipCap: outboxTx.MaxPriorityFeePerGas,
		GasFeeCap: outboxTx.MaxFeePerGas,
		Gas:       outboxTx.GasLimit,
		To:        &outboxTx.To,
		Value:     outboxTx.Value,
		Data:      outboxTx.Calldata,
	})
	return signer.SignTx(ctx, tx, chainID)
}
