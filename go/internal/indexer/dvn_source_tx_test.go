package indexer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestDVNSourceTxRecordsFromLogs(t *testing.T) {
	expectedDVN := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	logs := testDVNSourceLogs(t, expectedDVN, sendLib, big.NewInt(42))

	records, err := DVNSourceTxRecordsFromLogs(logs)
	if err != nil {
		t.Fatalf("DVNSourceTxRecordsFromLogs() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("DVNSourceTxRecordsFromLogs() records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Packet.SendLib != sendLib {
		t.Fatalf("packet send lib = %s, want %s", record.Packet.SendLib, sendLib)
	}
	if record.DVN != expectedDVN {
		t.Fatalf("dvn = %s, want %s", record.DVN, expectedDVN)
	}
	if record.DVNJob.GUID != record.Packet.GUID {
		t.Fatalf("dvn job guid = %s, want packet guid %s", record.DVNJob.GUID, record.Packet.GUID)
	}
	if record.DVNJob.ConfirmationsRequired != 12 {
		t.Fatalf("confirmations = %d, want 12", record.DVNJob.ConfirmationsRequired)
	}
	if record.DVNJob.Status != string(packets.DVNAssigned) {
		t.Fatalf("status = %q, want %q", record.DVNJob.Status, packets.DVNAssigned)
	}
}

func TestDVNSourceTxRecordsFromLogsMatchesMultipleSendsByLogIndex(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	secondGUID := common.HexToHash("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	logs := []gethtypes.Log{
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, big.NewInt(41), 0),
		testDVNFeePaidLog(t, txHash, sendLib, dvn, big.NewInt(41), 1),
		testPacketSentLog(t, txHash, sendLib, 2),
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, big.NewInt(42), 3),
		testDVNFeePaidLog(t, txHash, sendLib, dvn, big.NewInt(42), 4),
		testPacketSentLogWithPacket(t, txHash, sendLib, testEncodedPacketWithNonceAndGUID(8, secondGUID), 5),
	}

	records, err := DVNSourceTxRecordsFromLogs(logs)
	if err != nil {
		t.Fatalf("DVNSourceTxRecordsFromLogs() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("DVNSourceTxRecordsFromLogs() records = %d, want 2", len(records))
	}
	if records[0].Packet.SrcLogIndex != 2 || records[1].Packet.SrcLogIndex != 5 {
		t.Fatalf("source log indexes = %d/%d, want 2/5", records[0].Packet.SrcLogIndex, records[1].Packet.SrcLogIndex)
	}
	if records[0].Packet.GUID == records[1].Packet.GUID || records[1].Packet.GUID != secondGUID {
		t.Fatalf("packet GUIDs = %s/%s, want distinct second %s", records[0].Packet.GUID, records[1].Packet.GUID, secondGUID)
	}
}

func TestDVNSourceTxRecordsFromLogsUsesLatestUnmatchedFeeForPacket(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	otherSendLib := common.HexToAddress("0x9898989898989898989898989898989898989898")
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	logs := []gethtypes.Log{
		testDVNFeePaidLog(t, txHash, otherSendLib, dvn, big.NewInt(99), 0),
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, big.NewInt(42), 1),
		testDVNFeePaidLog(t, txHash, sendLib, dvn, big.NewInt(42), 2),
		testPacketSentLog(t, txHash, sendLib, 3),
	}

	records, err := DVNSourceTxRecordsFromLogs(logs)
	if err != nil {
		t.Fatalf("DVNSourceTxRecordsFromLogs() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("DVNSourceTxRecordsFromLogs() records = %d, want 1", len(records))
	}
	if records[0].Packet.SrcLogIndex != 3 {
		t.Fatalf("packet log index = %d, want 3", records[0].Packet.SrcLogIndex)
	}
}

func TestDVNSourceTxRecordsFromLogsRejectsSendLibraryMismatch(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	otherSendLib := common.HexToAddress("0x9898989898989898989898989898989898989898")
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	logs := []gethtypes.Log{
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, big.NewInt(42), 0),
		testDVNFeePaidLog(t, txHash, otherSendLib, dvn, big.NewInt(42), 1),
		testPacketSentLog(t, txHash, sendLib, 2),
	}

	_, err := DVNSourceTxRecordsFromLogs(logs)
	if err == nil {
		t.Fatal("DVNSourceTxRecordsFromLogs() error = nil, want send-library mismatch")
	}
	if !strings.Contains(err.Error(), "missing matching DVNFeePaid") {
		t.Fatalf("DVNSourceTxRecordsFromLogs() error = %v, want missing matching DVNFeePaid", err)
	}
}

func TestDVNSourceTxRecordsFromLogsForEndpointRejectsWrongPacketSentAddress(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	logs := testDVNSourceLogs(t, dvn, sendLib, big.NewInt(42))

	_, err := DVNSourceTxRecordsFromLogsForEndpoint(logs, common.HexToAddress("0x1212121212121212121212121212121212121212"))
	if err == nil {
		t.Fatal("DVNSourceTxRecordsFromLogsForEndpoint() error = nil, want endpoint mismatch")
	}
	if !strings.Contains(err.Error(), "PacketSent address") {
		t.Fatalf("DVNSourceTxRecordsFromLogsForEndpoint() error = %v, want PacketSent address mismatch", err)
	}
}

func testDVNSourceLogs(t *testing.T, dvn, sendLib common.Address, fee *big.Int) []gethtypes.Log {
	t.Helper()
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	return []gethtypes.Log{
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, fee, 0),
		testDVNFeePaidLog(t, txHash, sendLib, dvn, fee, 1),
		testPacketSentLog(t, txHash, sendLib, 2),
	}
}

func testDVNFeePaidLog(t *testing.T, txHash common.Hash, sendLib, dvn common.Address, fee *big.Int, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.SendUln302ABI()
	data, err := eventABI.Events["DVNFeePaid"].Inputs.Pack([]common.Address{dvn}, []common.Address{}, []*big.Int{fee})
	if err != nil {
		t.Fatalf("Pack DVNFeePaid error = %v", err)
	}
	return gethtypes.Log{
		Address:     sendLib,
		Topics:      []common.Hash{lzabi.DVNFeePaidTopic()},
		Data:        data,
		TxHash:      txHash,
		BlockNumber: 123,
		Index:       index,
	}
}

func testDVNJobAssignedLog(t *testing.T, txHash common.Hash, dvn, sendLib common.Address, fee *big.Int, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.OpenDVNABI()
	data, err := eventABI.Events["DVNJobAssigned"].Inputs.NonIndexed().Pack(sendLib, uint64(12), fee)
	if err != nil {
		t.Fatalf("Pack DVNJobAssigned error = %v", err)
	}
	return gethtypes.Log{
		Address: dvn,
		Topics: []common.Hash{
			lzabi.DVNJobAssignedTopic(),
			common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
			common.BigToHash(new(big.Int).SetUint64(40245)),
			common.BytesToHash(common.HexToAddress("0x7777777777777777777777777777777777777777").Bytes()),
		},
		Data:        data,
		TxHash:      txHash,
		BlockNumber: 123,
		Index:       index,
	}
}
