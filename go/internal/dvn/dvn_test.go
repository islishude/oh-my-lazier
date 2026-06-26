package dvn

import (
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
)

//go:embed testdata/abis/packet_sent.json
var packetSentABIJSON string

func TestProcessConfirmationsOnceWaitsForSourceConfirmations(t *testing.T) {
	packet := testDVNPacket()
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNAssigned),
			},
		}},
	}
	worker := NewWithHeads("shadow", store, map[uint32]HeadReader{packet.SrcEID: fakeHead{head: packet.SrcBlockNumber + 10}}, discardLogger())

	processed, err := worker.ProcessConfirmationsOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessConfirmationsOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.waitingGUID != packet.GUID {
		t.Fatalf("waiting guid = %s, want %s", store.waitingGUID, packet.GUID)
	}
}

func TestProcessConfirmationsOnceMarksQuorumChecking(t *testing.T) {
	packet := testDVNPacket()
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNWaitingConfirmations),
			},
		}},
	}
	worker := NewWithHeads("shadow", store, map[uint32]HeadReader{packet.SrcEID: fakeHead{head: packet.SrcBlockNumber + 11}}, discardLogger())

	processed, err := worker.ProcessConfirmationsOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessConfirmationsOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.quorumGUID != packet.GUID {
		t.Fatalf("quorum guid = %s, want %s", store.quorumGUID, packet.GUID)
	}
}

func TestProcessConfirmationsOncePausesChainOnHeadConflict(t *testing.T) {
	packet := testDVNPacket()
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNWaitingConfirmations),
			},
		}},
	}
	worker := NewWithHeads("shadow", store, map[uint32]HeadReader{packet.SrcEID: fakeHeadConflict{eid: packet.SrcEID}}, discardLogger())

	processed, err := worker.ProcessConfirmationsOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessConfirmationsOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.pausedChainEID != packet.SrcEID {
		t.Fatalf("paused chain eid = %d, want %d", store.pausedChainEID, packet.SrcEID)
	}
}

func TestProcessQuorumOnceMarksWouldVerify(t *testing.T) {
	packet := testDVNPacket()
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNQuorumChecking),
			},
		}},
	}
	worker := NewWithClients(
		"shadow",
		store,
		map[uint32]HeadReader{packet.SrcEID: fakeHead{head: packet.SrcBlockNumber + 12}},
		map[uint32]ReceiptReader{packet.SrcEID: fakeReceiptReader{receipt: testSourceReceipt(t, packet)}},
		discardLogger(),
	)

	processed, err := worker.ProcessQuorumOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessQuorumOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.wouldVerifyGUID != packet.GUID {
		t.Fatalf("would verify guid = %s, want %s", store.wouldVerifyGUID, packet.GUID)
	}
	if len(store.quorumResult) == 0 {
		t.Fatal("quorum result is empty")
	}
}

func TestProcessQuorumOnceMarksConflictOnMismatchedReceipt(t *testing.T) {
	packet := testDVNPacket()
	receipt := testSourceReceipt(t, packet)
	receipt.Status = gethtypes.ReceiptStatusFailed
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNQuorumChecking),
			},
		}},
	}
	worker := NewWithClients(
		"shadow",
		store,
		map[uint32]HeadReader{packet.SrcEID: fakeHead{head: packet.SrcBlockNumber + 12}},
		map[uint32]ReceiptReader{packet.SrcEID: fakeReceiptReader{receipt: receipt}},
		discardLogger(),
	)

	processed, err := worker.ProcessQuorumOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessQuorumOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.conflictGUID != packet.GUID {
		t.Fatalf("conflict guid = %s, want %s", store.conflictGUID, packet.GUID)
	}
	if store.conflictReason == "" {
		t.Fatal("conflict reason is empty")
	}
	if store.pausedPathwayGUID != packet.GUID {
		t.Fatalf("paused pathway guid = %s, want %s", store.pausedPathwayGUID, packet.GUID)
	}
}

func TestProcessQuorumOnceMarksConflictOnRPCDisagreement(t *testing.T) {
	packet := testDVNPacket()
	store := &fakeStore{
		work: []db.DVNWorkItem{{
			Packet: packet,
			Job: db.DVNJobRecord{
				GUID:                  packet.GUID,
				ConfirmationsRequired: 12,
				Status:                string(packets.DVNQuorumChecking),
			},
		}},
	}
	worker := NewWithClients(
		"shadow",
		store,
		map[uint32]HeadReader{packet.SrcEID: fakeHead{head: packet.SrcBlockNumber + 12}},
		map[uint32]ReceiptReader{packet.SrcEID: fakeReceiptConflictReader{txHash: packet.SrcTxHash}},
		discardLogger(),
	)

	processed, err := worker.ProcessQuorumOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessQuorumOnce() error = %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if store.conflictGUID != packet.GUID {
		t.Fatalf("conflict guid = %s, want %s", store.conflictGUID, packet.GUID)
	}
	if !strings.Contains(store.conflictReason, "rpc receipt quorum conflict") {
		t.Fatalf("conflict reason = %q, want rpc receipt quorum conflict", store.conflictReason)
	}
	if store.pausedPathwayGUID != packet.GUID {
		t.Fatalf("paused pathway guid = %s, want %s", store.pausedPathwayGUID, packet.GUID)
	}
}

type fakeStore struct {
	work              []db.DVNWorkItem
	waitingGUID       common.Hash
	quorumGUID        common.Hash
	wouldVerifyGUID   common.Hash
	conflictGUID      common.Hash
	conflictReason    string
	pausedChainEID    uint32
	pausedPathwayGUID common.Hash
	quorumResult      []byte
}

func (s *fakeStore) ListDVNWork(_ context.Context, status string, _ int) ([]db.DVNWorkItem, error) {
	for _, item := range s.work {
		if item.Job.Status == status {
			return []db.DVNWorkItem{item}, nil
		}
	}
	return nil, nil
}

func (s *fakeStore) MarkDVNWaitingConfirmations(_ context.Context, guid common.Hash, _ string) error {
	s.waitingGUID = guid
	return nil
}

func (s *fakeStore) MarkDVNQuorumChecking(_ context.Context, guid common.Hash, _ string) error {
	s.quorumGUID = guid
	return nil
}

func (s *fakeStore) MarkDVNWouldVerify(_ context.Context, guid common.Hash, _ string, quorumResult []byte) error {
	s.wouldVerifyGUID = guid
	s.quorumResult = append([]byte(nil), quorumResult...)
	return nil
}

func (s *fakeStore) MarkDVNQuorumConflict(_ context.Context, guid common.Hash, _, reason string, quorumResult []byte) error {
	s.conflictGUID = guid
	s.conflictReason = reason
	s.quorumResult = append([]byte(nil), quorumResult...)
	return nil
}

func (s *fakeStore) PauseChain(_ context.Context, eid uint32) error {
	s.pausedChainEID = eid
	return nil
}

func (s *fakeStore) PausePathwayForPacket(_ context.Context, guid common.Hash) error {
	s.pausedPathwayGUID = guid
	return nil
}

type fakeHead struct {
	head uint64
}

func (h fakeHead) BlockNumber(context.Context) (uint64, error) {
	return h.head, nil
}

func (h fakeHead) CheckHead(context.Context) (rpcquorum.HeadResult, error) {
	return rpcquorum.HeadResult{Number: new(big.Int).SetUint64(h.head), Hash: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Hex()}, nil
}

type fakeHeadConflict struct {
	eid uint32
}

func (h fakeHeadConflict) CheckHead(context.Context) (rpcquorum.HeadResult, error) {
	return rpcquorum.HeadResult{}, &rpcquorum.HeadConflictError{
		ChainName: fmt.Sprintf("eid-%d", h.eid),
		Number:    big.NewInt(42),
		Details:   []string{"provider a disagrees with provider b"},
	}
}

type fakeReceiptReader struct {
	receipt *gethtypes.Receipt
}

func (r fakeReceiptReader) TransactionReceipt(context.Context, common.Hash) (*gethtypes.Receipt, error) {
	return r.receipt, nil
}

type fakeReceiptConflictReader struct {
	txHash common.Hash
}

func (r fakeReceiptConflictReader) TransactionReceipt(context.Context, common.Hash) (*gethtypes.Receipt, error) {
	return nil, &rpcquorum.ReceiptConflictError{
		TxHash:  r.txHash,
		Details: []string{"provider a disagrees with provider b"},
	}
}

func testDVNPacket() db.PacketRecord {
	encodedPacket := testEncodedPacket()
	return db.PacketRecord{
		GUID:           common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		SrcEID:         40161,
		DstEID:         40245,
		Nonce:          big.NewInt(7),
		Sender:         common.HexToAddress("0x7777777777777777777777777777777777777777"),
		Receiver:       common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:        common.HexToAddress("0x9999999999999999999999999999999999999999"),
		SrcTxHash:      common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		SrcBlockNumber: 123,
		SrcLogIndex:    4,
		EncodedPacket:  encodedPacket,
		PacketHeader:   encodedPacket[:81],
		Message:        encodedPacket[81:],
		PayloadHash:    crypto.Keccak256Hash(encodedPacket[81:]),
		Options:        []byte{0x07, 0x08},
		Status:         string(packets.ExecutorNew),
	}
}

func testSourceReceipt(t *testing.T, packet db.PacketRecord) *gethtypes.Receipt {
	t.Helper()
	eventABI, err := abi.JSON(strings.NewReader(packetSentABIJSON))
	if err != nil {
		t.Fatalf("abi.JSON() error = %v", err)
	}
	data, err := eventABI.Events["PacketSent"].Inputs.Pack(packet.EncodedPacket, packet.Options, packet.SendLib)
	if err != nil {
		t.Fatalf("Pack PacketSent error = %v", err)
	}
	log := &gethtypes.Log{
		Address:     common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Topics:      []common.Hash{lzabi.PacketSentTopic()},
		Data:        data,
		TxHash:      packet.SrcTxHash,
		BlockNumber: packet.SrcBlockNumber,
		BlockHash:   common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
		Index:       packet.SrcLogIndex,
	}
	return &gethtypes.Receipt{
		TxHash: packet.SrcTxHash,
		Status: gethtypes.ReceiptStatusSuccessful,
		Logs:   []*gethtypes.Log{log},
	}
}

func testEncodedPacket() []byte {
	encoded := make([]byte, 0, 118)
	encoded = append(encoded, 1)
	encoded = binary.BigEndian.AppendUint64(encoded, 7)
	encoded = binary.BigEndian.AppendUint32(encoded, 40161)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x7777777777777777777777777777777777777777"))...)
	encoded = binary.BigEndian.AppendUint32(encoded, 40245)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x8888888888888888888888888888888888888888"))...)
	encoded = append(encoded, common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc").Bytes()...)
	encoded = append(encoded, []byte("hello")...)
	return encoded
}

func addressToBytes32(address common.Address) []byte {
	out := make([]byte, 32)
	copy(out[12:], address.Bytes())
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
