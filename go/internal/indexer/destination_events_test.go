package indexer

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/jackc/pgx/v5"
)

func TestApplyExecutorDestinationLogMarksCommitted(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Status = "COMMIT_TX_ENQUEUED"
	store := &fakeReceiptStore{}
	log := testPacketVerifiedLog(t, packet)

	applied, err := ApplyExecutorDestinationLog(context.Background(), store, packet, log)
	if err != nil {
		t.Fatalf("ApplyExecutorDestinationLog() error = %v", err)
	}
	if !applied {
		t.Fatal("applied = false, want true")
	}
	if store.committedGUID != packet.GUID {
		t.Fatalf("committed guid = %s, want %s", store.committedGUID, packet.GUID)
	}
	if store.committedTxHash != log.TxHash {
		t.Fatalf("committed tx = %s, want %s", store.committedTxHash, log.TxHash)
	}
}

func TestApplyExecutorDestinationLogMarksDelivered(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Status = "LZ_RECEIVE_TX_ENQUEUED"
	store := &fakeReceiptStore{}
	log := testPacketDeliveredLog(t, packet)

	applied, err := ApplyExecutorDestinationLog(context.Background(), store, packet, log)
	if err != nil {
		t.Fatalf("ApplyExecutorDestinationLog() error = %v", err)
	}
	if !applied {
		t.Fatal("applied = false, want true")
	}
	if store.deliveredGUID != packet.GUID {
		t.Fatalf("delivered guid = %s, want %s", store.deliveredGUID, packet.GUID)
	}
	if store.deliveredTxHash != log.TxHash {
		t.Fatalf("delivered tx = %s, want %s", store.deliveredTxHash, log.TxHash)
	}
}

func TestApplyExecutorDestinationLogMarksReceiveFailed(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Status = "LZ_RECEIVE_TX_ENQUEUED"
	store := &fakeReceiptStore{}
	log := testLzReceiveAlertLog(t, packet, []byte{0xde, 0xad})

	applied, err := ApplyExecutorDestinationLog(context.Background(), store, packet, log)
	if err != nil {
		t.Fatalf("ApplyExecutorDestinationLog() error = %v", err)
	}
	if !applied {
		t.Fatal("applied = false, want true")
	}
	if store.failedGUID != packet.GUID {
		t.Fatalf("failed guid = %s, want %s", store.failedGUID, packet.GUID)
	}
	if store.failedTxHash != log.TxHash {
		t.Fatalf("failed tx = %s, want %s", store.failedTxHash, log.TxHash)
	}
	if store.failedReason != "dead" {
		t.Fatalf("failed reason = %q, want dead", store.failedReason)
	}
}

func TestApplyExecutorDestinationLogRejectsMismatchedPacket(t *testing.T) {
	packet := testDestinationPacketRecord()
	log := testPacketVerifiedLog(t, packet)
	packet.PayloadHash = common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")

	applied, err := ApplyExecutorDestinationLog(context.Background(), &fakeReceiptStore{}, packet, log)
	if err == nil {
		t.Fatal("ApplyExecutorDestinationLog() error = nil, want mismatch")
	}
	if applied {
		t.Fatal("applied = true, want false on mismatch")
	}
}

func TestApplyExecutorDestinationLogsLooksUpAndAppliesKnownEvents(t *testing.T) {
	packet := testDestinationPacketRecord()
	verifiedLog := testPacketVerifiedLog(t, packet)
	alertLog := testLzReceiveAlertLog(t, packet, []byte{0xde, 0xad})
	store := &fakeDestinationStore{
		byGUID: map[common.Hash]db.PacketRecord{
			packet.GUID: packet,
		},
		byDestination: map[string]db.PacketRecord{
			destinationLookupKey(packet.DstEID, packet.SrcEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64()): packet,
		},
	}

	applied, err := ApplyExecutorDestinationLogs(context.Background(), store, packet.DstEID, []gethtypes.Log{
		{Topics: []common.Hash{common.HexToHash("0x01")}},
		verifiedLog,
		alertLog,
	})
	if err != nil {
		t.Fatalf("ApplyExecutorDestinationLogs() error = %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2", applied)
	}
	if store.committedGUID != packet.GUID {
		t.Fatalf("committed guid = %s, want %s", store.committedGUID, packet.GUID)
	}
	if store.failedGUID != packet.GUID {
		t.Fatalf("failed guid = %s, want %s", store.failedGUID, packet.GUID)
	}
}

func TestApplyExecutorDestinationLogsSkipsUnknownPackets(t *testing.T) {
	packet := testDestinationPacketRecord()
	store := &fakeDestinationStore{}

	applied, err := ApplyExecutorDestinationLogs(context.Background(), store, packet.DstEID, []gethtypes.Log{
		testPacketVerifiedLog(t, packet),
	})
	if err != nil {
		t.Fatalf("ApplyExecutorDestinationLogs() error = %v", err)
	}
	if applied != 0 {
		t.Fatalf("applied = %d, want 0", applied)
	}
}

type fakeReceiptStore struct {
	committedGUID   common.Hash
	committedTxHash common.Hash
	deliveredGUID   common.Hash
	deliveredTxHash common.Hash
	failedGUID      common.Hash
	failedTxHash    common.Hash
	failedReason    string
}

func testDestinationPacketRecord() db.PacketRecord {
	return db.PacketRecord{
		GUID:           common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SrcEID:         40161,
		DstEID:         40245,
		Nonce:          big.NewInt(7),
		Sender:         common.HexToAddress("0x7777777777777777777777777777777777777777"),
		Receiver:       common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:        common.HexToAddress("0x9999999999999999999999999999999999999999"),
		SrcTxHash:      common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		SrcBlockNumber: 123,
		SrcLogIndex:    4,
		EncodedPacket:  []byte{0x01, 0x02},
		PacketHeader:   []byte{0x03, 0x04},
		Message:        []byte{0x05, 0x06},
		PayloadHash:    common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		Options:        []byte{0x07, 0x08},
		Status:         "ASSIGNED",
	}
}

type fakeDestinationStore struct {
	fakeReceiptStore
	byGUID        map[common.Hash]db.PacketRecord
	byDestination map[string]db.PacketRecord
}

func (s *fakeDestinationStore) GetPacket(_ context.Context, guid common.Hash) (db.PacketRecord, error) {
	packet, ok := s.byGUID[guid]
	if !ok {
		return db.PacketRecord{}, pgx.ErrNoRows
	}
	return packet, nil
}

func (s *fakeDestinationStore) GetPacketByDestination(_ context.Context, dstEID, srcEID uint32, sender, receiver common.Address, nonce uint64) (db.PacketRecord, error) {
	key := destinationLookupKey(dstEID, srcEID, sender, receiver, nonce)
	packet, ok := s.byDestination[key]
	if !ok {
		return db.PacketRecord{}, pgx.ErrNoRows
	}
	return packet, nil
}

func destinationLookupKey(dstEID, srcEID uint32, sender, receiver common.Address, nonce uint64) string {
	return fmt.Sprintf("%d:%d:%s:%s:%d", dstEID, srcEID, sender, receiver, nonce)
}

func (s *fakeReceiptStore) MarkExecutorCommitted(_ context.Context, guid, txHash common.Hash) error {
	s.committedGUID = guid
	s.committedTxHash = txHash
	return nil
}

func (s *fakeReceiptStore) MarkExecutorDelivered(_ context.Context, guid, txHash common.Hash) error {
	s.deliveredGUID = guid
	s.deliveredTxHash = txHash
	return nil
}

func (s *fakeReceiptStore) MarkExecutorReceiveFailed(_ context.Context, guid, txHash common.Hash, reason string) error {
	s.failedGUID = guid
	s.failedTxHash = txHash
	s.failedReason = reason
	return nil
}

func testPacketVerifiedLog(t *testing.T, packetRecord db.PacketRecord) gethtypes.Log {
	t.Helper()
	data, err := endpointEventInputs(t, "PacketVerified").Pack(originFromPacketRecord(packetRecord), packetRecord.Receiver, packetRecord.PayloadHash)
	if err != nil {
		t.Fatalf("Pack PacketVerified error = %v", err)
	}
	return gethtypes.Log{
		Topics: []common.Hash{lzabi.PacketVerifiedTopic()},
		Data:   data,
		TxHash: common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
	}
}

func testPacketDeliveredLog(t *testing.T, packetRecord db.PacketRecord) gethtypes.Log {
	t.Helper()
	data, err := endpointEventInputs(t, "PacketDelivered").Pack(originFromPacketRecord(packetRecord), packetRecord.Receiver)
	if err != nil {
		t.Fatalf("Pack PacketDelivered error = %v", err)
	}
	return gethtypes.Log{
		Topics: []common.Hash{lzabi.PacketDeliveredTopic()},
		Data:   data,
		TxHash: common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
	}
}

func testLzReceiveAlertLog(t *testing.T, packetRecord db.PacketRecord, reason []byte) gethtypes.Log {
	t.Helper()
	data, err := endpointEventInputs(t, "LzReceiveAlert").NonIndexed().Pack(
		originFromPacketRecord(packetRecord),
		packetRecord.GUID,
		big.NewInt(100_000),
		big.NewInt(0),
		packetRecord.Message,
		[]byte{},
		reason,
	)
	if err != nil {
		t.Fatalf("Pack LzReceiveAlert error = %v", err)
	}
	return gethtypes.Log{
		Topics: []common.Hash{
			lzabi.LzReceiveAlertTopic(),
			common.BytesToHash(packetRecord.Receiver.Bytes()),
			common.BytesToHash(common.HexToAddress("0x9999999999999999999999999999999999999999").Bytes()),
		},
		Data:   data,
		TxHash: common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333"),
	}
}

func originFromPacketRecord(packet db.PacketRecord) lzabi.Origin {
	return lzabi.Origin{
		SrcEID: packet.SrcEID,
		Sender: common.BytesToHash(packet.Sender.Bytes()),
		Nonce:  packet.Nonce.Uint64(),
	}
}

func endpointEventInputs(t *testing.T, name string) abi.Arguments {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(endpointEventsABIJSON))
	if err != nil {
		t.Fatalf("abi.JSON() error = %v", err)
	}
	return parsed.Events[name].Inputs
}
