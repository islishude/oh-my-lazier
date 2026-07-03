package dvn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
)

const loopInterval = 5 * time.Second

const (
	// TxPurposeVerify identifies ReceiveUln302.verify outbox requests.
	TxPurposeVerify = "dvn_verify"
)

var (
	endpointViewABI   = lzabi.EndpointV2ABI()
	receiveUlnViewABI = lzabi.ReceiveUln302ABI()
)

// Store is the durable DVN state required by the worker.
type Store interface {
	ListDVNWork(ctx context.Context, status string, limit int) ([]db.DVNWorkItem, error)
	MarkDVNWaitingConfirmations(ctx context.Context, guid common.Hash, expectedStatus string) error
	MarkDVNQuorumChecking(ctx context.Context, guid common.Hash, expectedStatus string) error
	MarkDVNReadyToVerify(ctx context.Context, guid common.Hash, expectedStatus string, quorumResult []byte) error
	MarkDVNWouldVerify(ctx context.Context, guid common.Hash, expectedStatus string, quorumResult []byte) error
	EnqueueDVNVerifyTx(ctx context.Context, guid common.Hash, expectedStatus, nextStatus string, request db.TxRequest, quorumResult []byte) (int64, error)
	MarkDVNVerifiedFromChain(ctx context.Context, guid common.Hash, expectedStatus string, quorumResult []byte) error
	MarkDVNQuorumConflict(ctx context.Context, guid common.Hash, expectedStatus, reason string, quorumResult []byte) error
	MarkDVNReorgDetected(ctx context.Context, guid common.Hash, expectedStatus, reason string, quorumResult []byte) error
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

// ContractCaller is the eth_call surface used by destination-chain reconciliation.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// Settings controls active DVN verification transaction generation.
type Settings struct {
	SignerID string
}

// Worker runs the DVN verification workflow.
type Worker struct {
	store    Store
	registry *chain.Registry
	settings map[uint32]Settings
	heads    map[uint32]HeadReader
	receipts map[uint32]ReceiptReader
	callers  map[uint32]ContractCaller
	logger   *slog.Logger
}

// New creates a DVN worker.
func New(store Store, registry *chain.Registry, logger *slog.Logger) *Worker {
	return NewWithSettings(store, registry, nil, logger)
}

// NewWithSettings creates a DVN worker with explicit active-mode transaction settings by destination EID.
func NewWithSettings(store Store, registry *chain.Registry, settings map[uint32]Settings, logger *slog.Logger) *Worker {
	heads := make(map[uint32]HeadReader)
	receipts := make(map[uint32]ReceiptReader)
	callers := make(map[uint32]ContractCaller)
	if registry != nil {
		for _, configuredChain := range registry.All() {
			if configuredChain.RPC != nil {
				heads[configuredChain.EID] = configuredChain.RPC
				receipts[configuredChain.EID] = configuredChain.RPC
				callers[configuredChain.EID] = configuredChain.RPC
			}
		}
	}
	return NewWithClientsSettingsAndCallers(store, registry, settings, heads, receipts, callers, logger)
}

// NewWithHeads creates a DVN worker with explicit head readers for tests.
func NewWithHeads(store Store, heads map[uint32]HeadReader, logger *slog.Logger) *Worker {
	return NewWithClients(store, heads, nil, logger)
}

// NewWithClients creates a DVN worker with explicit source-chain clients for tests.
func NewWithClients(store Store, heads map[uint32]HeadReader, receipts map[uint32]ReceiptReader, logger *slog.Logger) *Worker {
	return NewWithClientsAndSettings(store, nil, nil, heads, receipts, logger)
}

// NewWithClientsAndSettings creates a DVN worker with explicit clients and active-mode settings for tests.
func NewWithClientsAndSettings(store Store, registry *chain.Registry, settings map[uint32]Settings, heads map[uint32]HeadReader, receipts map[uint32]ReceiptReader, logger *slog.Logger) *Worker {
	return NewWithClientsSettingsAndCallers(store, registry, settings, heads, receipts, nil, logger)
}

// NewWithClientsSettingsAndCallers creates a DVN worker with explicit clients and destination callers for tests.
func NewWithClientsSettingsAndCallers(store Store, registry *chain.Registry, settings map[uint32]Settings, heads map[uint32]HeadReader, receipts map[uint32]ReceiptReader, callers map[uint32]ContractCaller, logger *slog.Logger) *Worker {
	copiedHeads := make(map[uint32]HeadReader, len(heads))
	maps.Copy(copiedHeads, heads)
	copiedReceipts := make(map[uint32]ReceiptReader, len(receipts))
	maps.Copy(copiedReceipts, receipts)
	copiedCallers := make(map[uint32]ContractCaller, len(callers))
	maps.Copy(copiedCallers, callers)
	copiedSettings := make(map[uint32]Settings, len(settings))
	maps.Copy(copiedSettings, settings)
	return &Worker{store: store, registry: registry, settings: copiedSettings, heads: copiedHeads, receipts: copiedReceipts, callers: copiedCallers, logger: logger}
}

// Run starts the DVN verifier loop until the context is canceled.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.Info("dvn verifier loop started")
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
		if !processed {
			processed, err = w.ProcessReadyToVerifyOnce(ctx)
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
	for _, status := range []string{string(packets.DVNReorgDetected), string(packets.DVNAssigned), string(packets.DVNWaitingConfirmations)} {
		work, err := w.store.ListDVNWork(ctx, status, 1)
		if err != nil {
			return false, err
		}
		if len(work) == 0 {
			continue
		}
		item := work[0]
		if status == string(packets.DVNReorgDetected) {
			if err := w.store.MarkDVNWaitingConfirmations(ctx, item.Packet.GUID, status); err != nil {
				return false, err
			}
			w.logger.Warn("dvn job rolled back after source reorg", "guid", item.Packet.GUID, "src_eid", item.Packet.SrcEID)
			return true, nil
		}
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
		if errors.Is(err, ethereum.NotFound) {
			return true, w.markReorgDetected(ctx, item.Packet, err)
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
	if err := w.store.MarkDVNReadyToVerify(ctx, item.Packet.GUID, string(packets.DVNQuorumChecking), payload); err != nil {
		return false, err
	}
	w.logger.Info("dvn job ready to verify", "guid", item.Packet.GUID, "src_eid", item.Packet.SrcEID)
	return true, nil
}

// ProcessReadyToVerifyOnce advances one verified DVN job into shadow report or active tx enqueue.
func (w *Worker) ProcessReadyToVerifyOnce(ctx context.Context) (bool, error) {
	work, err := w.store.ListDVNWork(ctx, string(packets.DVNReadyToVerify), 1)
	if err != nil {
		return false, err
	}
	if len(work) == 0 {
		return false, nil
	}
	item := work[0]
	if len(item.Job.QuorumResult) == 0 {
		return false, fmt.Errorf("dvn job %s ready to verify without quorum result", item.Packet.GUID)
	}
	if w.registry == nil {
		return false, workerloop.Fatal(errors.New("dvn verifier requires chain registry"))
	}
	pathway, err := w.registry.Pathway(item.Packet.SrcEID, item.Packet.DstEID, item.Packet.Sender, item.Packet.Receiver)
	if err != nil {
		return false, err
	}
	switch pathway.DVNMode {
	case config.DVNModeShadow:
		if err := w.store.MarkDVNWouldVerify(ctx, item.Packet.GUID, string(packets.DVNReadyToVerify), item.Job.QuorumResult); err != nil {
			return false, err
		}
		w.logger.Info("dvn shadow job would verify", "guid", item.Packet.GUID, "src_eid", item.Packet.SrcEID)
	case config.DVNModeActive:
		complete, err := w.verificationAlreadyComplete(ctx, item.Packet, pathway, item.Job.ConfirmationsRequired)
		if err != nil {
			return false, err
		}
		if complete {
			if err := w.store.MarkDVNVerifiedFromChain(ctx, item.Packet.GUID, string(packets.DVNReadyToVerify), item.Job.QuorumResult); err != nil {
				return false, err
			}
			w.logger.Info("dvn verification already completed on chain", "guid", item.Packet.GUID)
			return true, nil
		}
		request, err := w.buildVerifyTx(item.Packet, pathway, item.Job.ConfirmationsRequired)
		if err != nil {
			return false, err
		}
		id, err := w.store.EnqueueDVNVerifyTx(ctx, item.Packet.GUID, string(packets.DVNReadyToVerify), string(packets.DVNVerifyTxEnqueued), request, item.Job.QuorumResult)
		if err != nil {
			return false, err
		}
		w.logger.Info("enqueued dvn verify tx", "guid", item.Packet.GUID, "tx_outbox_id", id)
	default:
		return false, workerloop.Fatal(fmt.Errorf("unsupported dvn mode %q", pathway.DVNMode))
	}
	return true, nil
}

func (w *Worker) verificationAlreadyComplete(ctx context.Context, packet db.PacketRecord, pathway chain.Pathway, confirmations uint64) (bool, error) {
	if err := packet.Validate(); err != nil {
		return false, err
	}
	if confirmations == 0 {
		return false, errors.New("dvn confirmations required is required")
	}
	if w.registry == nil {
		return false, workerloop.Fatal(errors.New("dvn active mode requires chain registry"))
	}
	dstChain, err := w.registry.Get(packet.DstEID)
	if err != nil {
		return false, err
	}
	caller := w.caller(packet.DstEID)
	if caller == nil {
		return false, fmt.Errorf("missing destination caller for eid %d", packet.DstEID)
	}
	payloadHash, err := callInboundPayloadHash(ctx, caller, dstChain.EndpointAddress, packet)
	if err != nil {
		return false, err
	}
	if payloadHash == packet.PayloadHash {
		return true, nil
	}
	submitted, observedConfirmations, err := callHashLookup(ctx, caller, pathway.ReceiveLib, crypto.Keccak256Hash(packet.PacketHeader), packet.PayloadHash, pathway.SourceWorkers.OpenDVN)
	if err != nil {
		return false, err
	}
	return submitted && observedConfirmations >= confirmations, nil
}

func callInboundPayloadHash(ctx context.Context, caller ContractCaller, endpoint common.Address, packet db.PacketRecord) (common.Hash, error) {
	if endpoint == (common.Address{}) {
		return common.Hash{}, errors.New("endpoint address is required")
	}
	data, err := endpointViewABI.Pack(
		"inboundPayloadHash",
		packet.Receiver,
		packet.SrcEID,
		common.BytesToHash(packet.Sender.Bytes()),
		packet.Nonce.Uint64(),
	)
	if err != nil {
		return common.Hash{}, err
	}
	result, err := caller.CallContract(ctx, ethereum.CallMsg{To: &endpoint, Data: data}, nil)
	if err != nil {
		return common.Hash{}, err
	}
	values, err := endpointViewABI.Unpack("inboundPayloadHash", result)
	if err != nil {
		return common.Hash{}, err
	}
	if len(values) != 1 {
		return common.Hash{}, fmt.Errorf("inboundPayloadHash returned %d values, want 1", len(values))
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return common.Hash{}, fmt.Errorf("inboundPayloadHash returned %T, want bytes32", values[0])
	}
	return common.BytesToHash(value[:]), nil
}

func callHashLookup(ctx context.Context, caller ContractCaller, receiveLib common.Address, headerHash, payloadHash common.Hash, dvn common.Address) (bool, uint64, error) {
	if receiveLib == (common.Address{}) {
		return false, 0, errors.New("receive lib address is required")
	}
	if dvn == (common.Address{}) {
		return false, 0, errors.New("open dvn address is required")
	}
	data, err := receiveUlnViewABI.Pack("hashLookup", headerHash, payloadHash, dvn)
	if err != nil {
		return false, 0, err
	}
	result, err := caller.CallContract(ctx, ethereum.CallMsg{To: &receiveLib, Data: data}, nil)
	if err != nil {
		return false, 0, err
	}
	values, err := receiveUlnViewABI.Unpack("hashLookup", result)
	if err != nil {
		return false, 0, err
	}
	if len(values) != 2 {
		return false, 0, fmt.Errorf("hashLookup returned %d values, want 2", len(values))
	}
	submitted, ok := values[0].(bool)
	if !ok {
		return false, 0, fmt.Errorf("hashLookup submitted returned %T, want bool", values[0])
	}
	observedConfirmations, ok := values[1].(uint64)
	if !ok {
		return false, 0, fmt.Errorf("hashLookup confirmations returned %T, want uint64", values[1])
	}
	return submitted, observedConfirmations, nil
}

func (w *Worker) buildVerifyTx(packet db.PacketRecord, pathway chain.Pathway, confirmations uint64) (db.TxRequest, error) {
	if w.registry == nil {
		return db.TxRequest{}, workerloop.Fatal(errors.New("dvn active mode requires chain registry"))
	}
	settings, ok := w.settings[packet.DstEID]
	if !ok {
		return db.TxRequest{}, workerloop.Fatal(fmt.Errorf("dvn active mode requires tx settings for destination eid %d", packet.DstEID))
	}
	if settings.SignerID == "" {
		return db.TxRequest{}, workerloop.Fatal(errors.New("dvn active mode requires signer id"))
	}
	return BuildVerifyTx(packet, pathway.ReceiveLib, confirmations, settings.SignerID)
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

func (w *Worker) markReorgDetected(ctx context.Context, packet db.PacketRecord, err error) error {
	payload, marshalErr := json.Marshal(map[string]any{
		"tx_hash": packet.SrcTxHash.Hex(),
		"error":   err.Error(),
	})
	if marshalErr != nil {
		return marshalErr
	}
	if err := w.store.MarkDVNReorgDetected(ctx, packet.GUID, string(packets.DVNQuorumChecking), err.Error(), payload); err != nil {
		return err
	}
	w.logger.Warn("dvn source reorg detected", "guid", packet.GUID, "src_eid", packet.SrcEID, "tx_hash", packet.SrcTxHash)
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

func (w *Worker) caller(eid uint32) ContractCaller {
	if w == nil {
		return nil
	}
	return w.callers[eid]
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

// BuildVerifyCalldata ABI-encodes ReceiveUln302.verify for active DVN mode.
func BuildVerifyCalldata(packet db.PacketRecord, confirmations uint64) ([]byte, error) {
	if err := packet.Validate(); err != nil {
		return nil, err
	}
	if confirmations == 0 {
		return nil, errors.New("dvn confirmations required is required")
	}
	return lzabi.PackReceiveUln302Verify(packet.PacketHeader, packet.PayloadHash, confirmations)
}

// BuildVerifyTx creates the outbox request for ReceiveUln302.verify.
func BuildVerifyTx(packet db.PacketRecord, receiveLib common.Address, confirmations uint64, signerID string) (db.TxRequest, error) {
	if receiveLib == (common.Address{}) {
		return db.TxRequest{}, errors.New("receive lib address is required")
	}
	if signerID == "" {
		return db.TxRequest{}, errors.New("signer id is required")
	}
	calldata, err := BuildVerifyCalldata(packet, confirmations)
	if err != nil {
		return db.TxRequest{}, err
	}
	return db.TxRequest{
		ChainEID: packet.DstEID,
		Purpose:  TxPurposeVerify,
		GUID:     packet.GUID.Bytes(),
		To:       receiveLib,
		Calldata: calldata,
		Value:    new(big.Int),
		SignerID: signerID,
	}, nil
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
