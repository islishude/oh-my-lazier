package dvn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
)

const loopInterval = 5 * time.Second

// Mode selects whether the DVN verifier only reports or also submits verification transactions.
type Mode string

const (
	// ModeShadow verifies and reports what the DVN would submit without sending transactions.
	ModeShadow Mode = "shadow"
	// ModeActive verifies and enqueues active DVN verification transactions.
	ModeActive Mode = "active"
)

// Store is the durable DVN state required by the worker.
type Store interface {
	ListDVNWork(ctx context.Context, status string, limit int) ([]db.DVNWorkItem, error)
	MarkDVNWaitingConfirmations(ctx context.Context, guid common.Hash, expectedStatus string) error
	MarkDVNQuorumChecking(ctx context.Context, guid common.Hash, expectedStatus string) error
	MarkDVNWouldVerify(ctx context.Context, guid common.Hash, expectedStatus string, quorumResult []byte) error
	MarkDVNQuorumConflict(ctx context.Context, guid common.Hash, expectedStatus, reason string, quorumResult []byte) error
	PauseChain(ctx context.Context, eid uint32) error
	PausePathwayForPacket(ctx context.Context, guid common.Hash) error
}

// HeadReader reads a source chain head.
type HeadReader interface {
	CheckHead(ctx context.Context) (rpcquorum.HeadResult, error)
}

// ReceiptReader reads source-chain transaction receipts.
type ReceiptReader interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*gethtypes.Receipt, error)
}

// Worker runs the DVN verification workflow.
type Worker struct {
	mode     Mode
	store    Store
	heads    map[uint32]HeadReader
	receipts map[uint32]ReceiptReader
	logger   *slog.Logger
}

// New creates a DVN worker for the configured mode.
func New(mode string, store Store, registry *chain.Registry, logger *slog.Logger) *Worker {
	heads := make(map[uint32]HeadReader)
	receipts := make(map[uint32]ReceiptReader)
	if registry != nil {
		for _, configuredChain := range registry.All() {
			if configuredChain.RPC != nil {
				heads[configuredChain.EID] = configuredChain.RPC
				receipts[configuredChain.EID] = configuredChain.RPC
			}
		}
	}
	return NewWithClients(mode, store, heads, receipts, logger)
}

// NewWithHeads creates a DVN worker with explicit head readers for tests.
func NewWithHeads(mode string, store Store, heads map[uint32]HeadReader, logger *slog.Logger) *Worker {
	return NewWithClients(mode, store, heads, nil, logger)
}

// NewWithClients creates a DVN worker with explicit source-chain clients for tests.
func NewWithClients(mode string, store Store, heads map[uint32]HeadReader, receipts map[uint32]ReceiptReader, logger *slog.Logger) *Worker {
	copiedHeads := make(map[uint32]HeadReader, len(heads))
	maps.Copy(copiedHeads, heads)
	copiedReceipts := make(map[uint32]ReceiptReader, len(receipts))
	maps.Copy(copiedReceipts, receipts)
	return &Worker{mode: Mode(mode), store: store, heads: copiedHeads, receipts: copiedReceipts, logger: logger}
}

// Run starts the DVN verifier loop until the context is canceled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("dvn verifier loop started", "mode", w.mode)
	for {
		processed, err := w.ProcessConfirmationsOnce(ctx)
		if err != nil {
			return err
		}
		if !processed {
			processed, err = w.ProcessQuorumOnce(ctx)
			if err != nil {
				return err
			}
		}
		if processed {
			continue
		}
		timer := time.NewTimer(loopInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ProcessConfirmationsOnce advances one DVN job once source confirmations are available.
func (w *Worker) ProcessConfirmationsOnce(ctx context.Context) (bool, error) {
	for _, status := range []string{string(packets.DVNAssigned), string(packets.DVNWaitingConfirmations)} {
		work, err := w.store.ListDVNWork(ctx, status, 1)
		if err != nil {
			return false, err
		}
		if len(work) == 0 {
			continue
		}
		item := work[0]
		headReader := w.head(item.Packet.SrcEID)
		if headReader == nil {
			return false, fmt.Errorf("missing source head reader for eid %d", item.Packet.SrcEID)
		}
		headResult, err := headReader.CheckHead(ctx)
		if err != nil {
			if rpcquorum.IsHeadConflict(err) {
				if pauseErr := w.store.PauseChain(ctx, item.Packet.SrcEID); pauseErr != nil {
					return false, pauseErr
				}
				w.logger.Warn("dvn head quorum conflict paused chain", "src_eid", item.Packet.SrcEID, "error", err.Error())
				return true, nil
			}
			return false, err
		}
		if headResult.Number == nil {
			return false, fmt.Errorf("source head result for eid %d is missing number", item.Packet.SrcEID)
		}
		head := headResult.Number.Uint64()
		if !hasRequiredConfirmations(item.Packet.SrcBlockNumber, item.Job.ConfirmationsRequired, head) {
			if status == string(packets.DVNAssigned) {
				if err := w.store.MarkDVNWaitingConfirmations(ctx, item.Packet.GUID, status); err != nil {
					return false, err
				}
			}
			return true, nil
		}
		if err := w.store.MarkDVNQuorumChecking(ctx, item.Packet.GUID, status); err != nil {
			return false, err
		}
		w.logger.Info("dvn job reached source confirmations", "guid", item.Packet.GUID, "src_eid", item.Packet.SrcEID)
		return true, nil
	}
	return false, nil
}

// ProcessQuorumOnce verifies one confirmed DVN job against source-chain receipt evidence.
func (w *Worker) ProcessQuorumOnce(ctx context.Context) (bool, error) {
	work, err := w.store.ListDVNWork(ctx, string(packets.DVNQuorumChecking), 1)
	if err != nil {
		return false, err
	}
	if len(work) == 0 {
		return false, nil
	}
	item := work[0]
	receiptReader := w.receipt(item.Packet.SrcEID)
	if receiptReader == nil {
		return false, fmt.Errorf("missing source receipt reader for eid %d", item.Packet.SrcEID)
	}
	receipt, err := receiptReader.TransactionReceipt(ctx, item.Packet.SrcTxHash)
	if err != nil {
		if rpcquorum.IsReceiptConflict(err) {
			return true, w.markQuorumConflict(ctx, item.Packet, err)
		}
		return false, err
	}
	report, err := verifySourceReceipt(item.Packet, receipt)
	if err != nil {
		return true, w.markQuorumConflict(ctx, item.Packet, err)
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return false, err
	}
	if err := w.store.MarkDVNWouldVerify(ctx, item.Packet.GUID, string(packets.DVNQuorumChecking), payload); err != nil {
		return false, err
	}
	w.logger.Info("dvn shadow job would verify", "guid", item.Packet.GUID, "src_eid", item.Packet.SrcEID)
	return true, nil
}

func (w *Worker) markQuorumConflict(ctx context.Context, packet db.PacketRecord, err error) error {
	payload, marshalErr := json.Marshal(map[string]any{
		"tx_hash": packet.SrcTxHash.Hex(),
		"error":   err.Error(),
	})
	if marshalErr != nil {
		return marshalErr
	}
	if err := w.store.MarkDVNQuorumConflict(ctx, packet.GUID, string(packets.DVNQuorumChecking), err.Error(), payload); err != nil {
		return err
	}
	if err := w.store.PausePathwayForPacket(ctx, packet.GUID); err != nil {
		return err
	}
	w.logger.Warn("dvn quorum conflict paused pathway", "guid", packet.GUID, "src_eid", packet.SrcEID, "dst_eid", packet.DstEID, "error", err.Error())
	return nil
}

func (w *Worker) head(eid uint32) HeadReader {
	if w == nil {
		return nil
	}
	return w.heads[eid]
}

func (w *Worker) receipt(eid uint32) ReceiptReader {
	if w == nil {
		return nil
	}
	return w.receipts[eid]
}

func hasRequiredConfirmations(blockNumber, confirmationsRequired, head uint64) bool {
	if confirmationsRequired == 0 {
		return false
	}
	if head < blockNumber {
		return false
	}
	return head-blockNumber+1 >= confirmationsRequired
}

// QuorumReport is the persisted shadow-mode evidence for a packet that would be verified.
type QuorumReport struct {
	TxHash      string `json:"tx_hash"`
	BlockNumber uint64 `json:"block_number"`
	BlockHash   string `json:"block_hash"`
	LogIndex    uint   `json:"log_index"`
	GUID        string `json:"guid"`
	PayloadHash string `json:"payload_hash"`
}

func verifySourceReceipt(packet db.PacketRecord, receipt *gethtypes.Receipt) (QuorumReport, error) {
	if receipt == nil {
		return QuorumReport{}, errors.New("source receipt is missing")
	}
	if receipt.TxHash != packet.SrcTxHash {
		return QuorumReport{}, fmt.Errorf("receipt tx hash %s does not match packet source tx %s", receipt.TxHash, packet.SrcTxHash)
	}
	if receipt.Status != gethtypes.ReceiptStatusSuccessful {
		return QuorumReport{}, fmt.Errorf("source tx receipt status is %d", receipt.Status)
	}
	for _, log := range receipt.Logs {
		if log == nil || log.Index != packet.SrcLogIndex {
			continue
		}
		record, err := indexer.PacketRecordFromSentLog(*log)
		if err != nil {
			return QuorumReport{}, err
		}
		if err := validateReceiptPacket(packet, record); err != nil {
			return QuorumReport{}, err
		}
		return QuorumReport{
			TxHash:      receipt.TxHash.Hex(),
			BlockNumber: log.BlockNumber,
			BlockHash:   log.BlockHash.Hex(),
			LogIndex:    log.Index,
			GUID:        packet.GUID.Hex(),
			PayloadHash: packet.PayloadHash.Hex(),
		}, nil
	}
	return QuorumReport{}, fmt.Errorf("source receipt missing PacketSent log index %d", packet.SrcLogIndex)
}

func validateReceiptPacket(expected, actual db.PacketRecord) error {
	if actual.GUID != expected.GUID {
		return fmt.Errorf("receipt packet guid %s does not match stored guid %s", actual.GUID, expected.GUID)
	}
	if actual.SrcEID != expected.SrcEID || actual.DstEID != expected.DstEID {
		return fmt.Errorf("receipt packet pathway %d -> %d does not match stored pathway %d -> %d", actual.SrcEID, actual.DstEID, expected.SrcEID, expected.DstEID)
	}
	if actual.Sender != expected.Sender || actual.Receiver != expected.Receiver {
		return fmt.Errorf("receipt packet oapps %s -> %s do not match stored oapps %s -> %s", actual.Sender, actual.Receiver, expected.Sender, expected.Receiver)
	}
	if actual.SendLib != expected.SendLib {
		return fmt.Errorf("receipt send lib %s does not match stored send lib %s", actual.SendLib, expected.SendLib)
	}
	if actual.PayloadHash != expected.PayloadHash {
		return fmt.Errorf("receipt payload hash %s does not match stored payload hash %s", actual.PayloadHash, expected.PayloadHash)
	}
	if !bytes.Equal(actual.EncodedPacket, expected.EncodedPacket) {
		return errors.New("receipt encoded packet does not match stored encoded packet")
	}
	return nil
}
