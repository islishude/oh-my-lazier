package indexer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/jackc/pgx/v5"
)

// ExecutorReceiptStore persists executor destination-chain event outcomes.
type ExecutorReceiptStore interface {
	MarkExecutorCommitted(ctx context.Context, guid, txHash common.Hash) error
	MarkExecutorDelivered(ctx context.Context, guid, txHash common.Hash) error
	MarkExecutorReceiveFailed(ctx context.Context, guid, txHash common.Hash, reason string) error
}

// ExecutorDestinationStore loads known packets and persists destination-chain executor outcomes.
type ExecutorDestinationStore interface {
	ExecutorReceiptStore
	GetExecutorJob(ctx context.Context, guid common.Hash) (db.ExecutorJobRecord, error)
	GetPacket(ctx context.Context, guid common.Hash) (db.PacketRecord, error)
	GetPacketByDestination(ctx context.Context, dstEID, srcEID uint32, sender, receiver common.Address, nonce uint64) (db.PacketRecord, error)
}

// DVNDestinationStore loads known packets and persists destination-chain DVN verification outcomes.
type DVNDestinationStore interface {
	GetDVNJob(ctx context.Context, guid common.Hash) (db.DVNJobRecord, error)
	GetPacketByVerification(ctx context.Context, dstEID uint32, packetHeader []byte, payloadHash common.Hash) (db.PacketRecord, error)
	MarkDVNVerified(ctx context.Context, guid, txHash common.Hash) error
}

// DestinationStore persists all destination-chain event outcomes.
type DestinationStore interface {
	ExecutorDestinationStore
	DVNDestinationStore
}

type destinationLogObserver struct {
	executorApplied func(packet db.PacketRecord, job db.ExecutorJobRecord, log gethtypes.Log)
	executorSkipped func(reason string, packet db.PacketRecord, job db.ExecutorJobRecord, log gethtypes.Log)
	dvnApplied      func(packet db.PacketRecord, job db.DVNJobRecord, log gethtypes.Log)
	dvnSkipped      func(reason string, packet db.PacketRecord, job db.DVNJobRecord, log gethtypes.Log)
}

// ApplyExecutorDestinationLogs applies known EndpointV2 destination logs for one destination chain.
func ApplyExecutorDestinationLogs(ctx context.Context, store ExecutorDestinationStore, dstEID uint32, logs []gethtypes.Log) (int, error) {
	return applyExecutorDestinationLogs(ctx, store, dstEID, logs, destinationLogObserver{})
}

func applyExecutorDestinationLogs(ctx context.Context, store ExecutorDestinationStore, dstEID uint32, logs []gethtypes.Log, observer destinationLogObserver) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("executor destination store is required")
	}
	if dstEID == 0 {
		return 0, fmt.Errorf("destination eid is required")
	}
	applied := 0
	for _, log := range logs {
		packet, ok, err := packetForDestinationLog(ctx, store, dstEID, log)
		if errors.Is(err, pgx.ErrNoRows) {
			if observer.executorSkipped != nil {
				observer.executorSkipped("unknown_packet", db.PacketRecord{}, db.ExecutorJobRecord{}, log)
			}
			continue
		}
		if err != nil {
			return applied, err
		}
		if !ok {
			if observer.executorSkipped != nil {
				observer.executorSkipped("unsupported_event", db.PacketRecord{}, db.ExecutorJobRecord{}, log)
			}
			continue
		}
		if packet.DstEID != dstEID {
			return applied, fmt.Errorf("packet %s destination eid %d does not match indexed chain %d", packet.GUID, packet.DstEID, dstEID)
		}
		job, err := store.GetExecutorJob(ctx, packet.GUID)
		if errors.Is(err, pgx.ErrNoRows) {
			if observer.executorSkipped != nil {
				observer.executorSkipped("missing_executor_job", packet, db.ExecutorJobRecord{}, log)
			}
			continue
		} else if err != nil {
			return applied, err
		}
		if executorDestinationLogAlreadyApplied(job.Status, log.Topics[0]) {
			if observer.executorSkipped != nil {
				observer.executorSkipped("already_applied", packet, job, log)
			}
			continue
		}
		didApply, err := ApplyExecutorDestinationLog(ctx, store, packet, log)
		if err != nil {
			return applied, err
		}
		if didApply {
			if observer.executorApplied != nil {
				observer.executorApplied(packet, job, log)
			}
			applied++
		}
	}
	return applied, nil
}

func executorDestinationLogAlreadyApplied(status string, topic common.Hash) bool {
	switch topic {
	case lzabi.PacketVerifiedTopic():
		switch status {
		case string(packets.ExecutorCommitted),
			string(packets.ExecutorExecutable),
			string(packets.ExecutorLzReceiveTxEnqueued),
			string(packets.ExecutorDelivered),
			string(packets.ExecutorLzReceiveFailed):
			return true
		}
	case lzabi.PacketDeliveredTopic():
		return status == string(packets.ExecutorDelivered)
	case lzabi.LzReceiveAlertTopic():
		return status == string(packets.ExecutorLzReceiveFailed)
	}
	return false
}

// ApplyDVNDestinationLogs applies ReceiveUln302 PayloadVerified logs for configured pathway OpenDVNs.
func ApplyDVNDestinationLogs(ctx context.Context, store DVNDestinationStore, dstEID uint32, pathways []chain.Pathway, logs []gethtypes.Log) (int, error) {
	return applyDVNDestinationLogs(ctx, store, dstEID, pathways, logs, destinationLogObserver{})
}

func applyDVNDestinationLogs(ctx context.Context, store DVNDestinationStore, dstEID uint32, pathways []chain.Pathway, logs []gethtypes.Log, observer destinationLogObserver) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("dvn destination store is required")
	}
	if dstEID == 0 {
		return 0, fmt.Errorf("destination eid is required")
	}
	applied := 0
	for _, log := range logs {
		if len(log.Topics) == 0 || log.Topics[0] != lzabi.PayloadVerifiedTopic() {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("unsupported_event", db.PacketRecord{}, db.DVNJobRecord{}, log)
			}
			continue
		}
		event, err := lzabi.DecodePayloadVerified(log)
		if err != nil {
			return applied, err
		}
		packet, err := store.GetPacketByVerification(ctx, dstEID, event.Header, event.ProofHash)
		if errors.Is(err, pgx.ErrNoRows) {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("unknown_packet", db.PacketRecord{}, db.DVNJobRecord{}, log)
			}
			continue
		}
		if err != nil {
			return applied, err
		}
		if packet.DstEID != dstEID {
			return applied, fmt.Errorf("packet %s destination eid %d does not match indexed chain %d", packet.GUID, packet.DstEID, dstEID)
		}
		pathway, ok := pathwayForPacket(pathways, packet)
		if !ok {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("unknown_pathway", packet, db.DVNJobRecord{}, log)
			}
			continue
		}
		if event.DVN != pathway.DestinationWorkers.OpenDVN {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("unexpected_worker", packet, db.DVNJobRecord{}, log)
			}
			continue
		}
		if err := validatePayloadVerified(packet, event); err != nil {
			return applied, err
		}
		job, err := store.GetDVNJob(ctx, packet.GUID)
		if errors.Is(err, pgx.ErrNoRows) {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("missing_dvn_job", packet, db.DVNJobRecord{}, log)
			}
			continue
		}
		if err != nil {
			return applied, err
		}
		if job.Status == string(packets.DVNVerified) {
			if observer.dvnSkipped != nil {
				observer.dvnSkipped("already_applied", packet, job, log)
			}
			continue
		}
		if err := store.MarkDVNVerified(ctx, packet.GUID, log.TxHash); err != nil {
			return applied, err
		}
		if observer.dvnApplied != nil {
			observer.dvnApplied(packet, job, log)
		}
		applied++
	}
	return applied, nil
}

func pathwayForPacket(pathways []chain.Pathway, packet db.PacketRecord) (chain.Pathway, bool) {
	for _, pathway := range pathways {
		if pathway.SrcEID == packet.SrcEID &&
			pathway.DstEID == packet.DstEID &&
			pathway.SrcOApp == packet.Sender &&
			pathway.DstOApp == packet.Receiver {
			return pathway, true
		}
	}
	return chain.Pathway{}, false
}

func executorDestinationEventName(topic common.Hash) string {
	switch topic {
	case lzabi.PacketVerifiedTopic():
		return "PacketVerified"
	case lzabi.PacketDeliveredTopic():
		return "PacketDelivered"
	case lzabi.LzReceiveAlertTopic():
		return "LzReceiveAlert"
	default:
		return "unknown"
	}
}

func executorDestinationTargetStatus(topic common.Hash) string {
	switch topic {
	case lzabi.PacketVerifiedTopic():
		return string(packets.ExecutorCommitted)
	case lzabi.PacketDeliveredTopic():
		return string(packets.ExecutorDelivered)
	case lzabi.LzReceiveAlertTopic():
		return string(packets.ExecutorLzReceiveFailed)
	default:
		return ""
	}
}

func logTopic(log gethtypes.Log) common.Hash {
	if len(log.Topics) == 0 {
		return common.Hash{}
	}
	return log.Topics[0]
}

// ApplyExecutorDestinationLog applies one validated destination EndpointV2 log to executor state.
func ApplyExecutorDestinationLog(ctx context.Context, store ExecutorReceiptStore, packet db.PacketRecord, log gethtypes.Log) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("executor receipt store is required")
	}
	if err := packet.Validate(); err != nil {
		return false, err
	}
	if len(log.Topics) == 0 {
		return false, nil
	}
	switch log.Topics[0] {
	case lzabi.PacketVerifiedTopic():
		event, err := lzabi.DecodePacketVerified(log)
		if err != nil {
			return false, err
		}
		if err := validatePacketVerified(packet, event); err != nil {
			return false, err
		}
		return true, store.MarkExecutorCommitted(ctx, packet.GUID, log.TxHash)
	case lzabi.PacketDeliveredTopic():
		event, err := lzabi.DecodePacketDelivered(log)
		if err != nil {
			return false, err
		}
		if err := validatePacketDelivered(packet, event); err != nil {
			return false, err
		}
		return true, store.MarkExecutorDelivered(ctx, packet.GUID, log.TxHash)
	case lzabi.LzReceiveAlertTopic():
		event, err := lzabi.DecodeLzReceiveAlert(log)
		if err != nil {
			return false, err
		}
		if err := validateLzReceiveAlert(packet, event); err != nil {
			return false, err
		}
		return true, store.MarkExecutorReceiveFailed(ctx, packet.GUID, log.TxHash, hex.EncodeToString(event.Reason))
	default:
		return false, nil
	}
}

func packetForDestinationLog(ctx context.Context, store ExecutorDestinationStore, dstEID uint32, log gethtypes.Log) (db.PacketRecord, bool, error) {
	if len(log.Topics) == 0 {
		return db.PacketRecord{}, false, nil
	}
	switch log.Topics[0] {
	case lzabi.PacketVerifiedTopic():
		event, err := lzabi.DecodePacketVerified(log)
		if err != nil {
			return db.PacketRecord{}, false, err
		}
		packet, err := store.GetPacketByDestination(ctx, dstEID, event.Origin.SrcEID, originSenderAddress(event.Origin), event.Receiver, event.Origin.Nonce)
		return packet, true, err
	case lzabi.PacketDeliveredTopic():
		event, err := lzabi.DecodePacketDelivered(log)
		if err != nil {
			return db.PacketRecord{}, false, err
		}
		packet, err := store.GetPacketByDestination(ctx, dstEID, event.Origin.SrcEID, originSenderAddress(event.Origin), event.Receiver, event.Origin.Nonce)
		return packet, true, err
	case lzabi.LzReceiveAlertTopic():
		event, err := lzabi.DecodeLzReceiveAlert(log)
		if err != nil {
			return db.PacketRecord{}, false, err
		}
		packet, err := store.GetPacket(ctx, event.GUID)
		return packet, true, err
	default:
		return db.PacketRecord{}, false, nil
	}
}

func validatePacketVerified(packet db.PacketRecord, event lzabi.PacketVerified) error {
	if err := validateOrigin(packet, event.Origin); err != nil {
		return err
	}
	if event.Receiver != packet.Receiver {
		return fmt.Errorf("PacketVerified receiver %s does not match packet receiver %s", event.Receiver, packet.Receiver)
	}
	if event.PayloadHash != packet.PayloadHash {
		return fmt.Errorf("PacketVerified payload hash %s does not match packet payload hash %s", event.PayloadHash, packet.PayloadHash)
	}
	return nil
}

func validatePacketDelivered(packet db.PacketRecord, event lzabi.PacketDelivered) error {
	if err := validateOrigin(packet, event.Origin); err != nil {
		return err
	}
	if event.Receiver != packet.Receiver {
		return fmt.Errorf("PacketDelivered receiver %s does not match packet receiver %s", event.Receiver, packet.Receiver)
	}
	return nil
}

func validateLzReceiveAlert(packet db.PacketRecord, event lzabi.LzReceiveAlert) error {
	if err := validateOrigin(packet, event.Origin); err != nil {
		return err
	}
	if event.Receiver != packet.Receiver {
		return fmt.Errorf("LzReceiveAlert receiver %s does not match packet receiver %s", event.Receiver, packet.Receiver)
	}
	if event.GUID != packet.GUID {
		return fmt.Errorf("LzReceiveAlert guid %s does not match packet guid %s", event.GUID, packet.GUID)
	}
	if string(event.Message) != string(packet.Message) {
		return fmt.Errorf("LzReceiveAlert message does not match packet message")
	}
	return nil
}

func validatePayloadVerified(packet db.PacketRecord, event lzabi.PayloadVerified) error {
	if string(event.Header) != string(packet.PacketHeader) {
		return fmt.Errorf("PayloadVerified header does not match packet header")
	}
	if event.ProofHash != packet.PayloadHash {
		return fmt.Errorf("PayloadVerified proof hash %s does not match packet payload hash %s", event.ProofHash, packet.PayloadHash)
	}
	if event.Confirmations == nil || event.Confirmations.Sign() <= 0 {
		return fmt.Errorf("PayloadVerified confirmations must be positive")
	}
	return nil
}

func validateOrigin(packet db.PacketRecord, origin lzabi.Origin) error {
	if origin.SrcEID != packet.SrcEID {
		return fmt.Errorf("origin source eid %d does not match packet source eid %d", origin.SrcEID, packet.SrcEID)
	}
	if origin.Sender != common.BytesToHash(packet.Sender.Bytes()) {
		return fmt.Errorf("origin sender %s does not match packet sender %s", origin.Sender, packet.Sender)
	}
	if packet.Nonce == nil || !packet.Nonce.IsUint64() {
		return fmt.Errorf("packet nonce %s does not fit uint64", packet.Nonce)
	}
	if origin.Nonce != packet.Nonce.Uint64() {
		return fmt.Errorf("origin nonce %d does not match packet nonce %s", origin.Nonce, packet.Nonce)
	}
	return nil
}

func originSenderAddress(origin lzabi.Origin) common.Address {
	return common.BytesToAddress(origin.Sender.Bytes()[12:])
}
