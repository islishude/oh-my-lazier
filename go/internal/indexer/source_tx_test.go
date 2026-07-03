package indexer

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestExecutorSourceTxRecordsFromLogs(t *testing.T) {
	expectedExecutor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	logs := testExecutorSourceLogs(t, expectedExecutor, sendLib, big.NewInt(42))

	records, err := ExecutorSourceTxRecordsFromLogs(logs)
	if err != nil {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Packet.SendLib != sendLib {
		t.Fatalf("packet send lib = %s, want %s", record.Packet.SendLib, sendLib)
	}
	if record.Executor != expectedExecutor {
		t.Fatalf("executor = %s, want %s", record.Executor, expectedExecutor)
	}
	if record.ExecutorJob.GUID != record.Packet.GUID {
		t.Fatalf("job guid = %s, want packet guid %s", record.ExecutorJob.GUID, record.Packet.GUID)
	}
	if record.ExecutorJob.Status != string(packets.ExecutorAssigned) {
		t.Fatalf("job status = %q, want %q", record.ExecutorJob.Status, packets.ExecutorAssigned)
	}
	if record.ExecutorJob.AssignedFee.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("assigned fee = %s, want 42", record.ExecutorJob.AssignedFee)
	}
}

func TestExecutorSourceTxRecordsFromLogsMatchesMultipleSendsByLogIndex(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secondGUID := common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	logs := []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(41), validExecutorOptions(), 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(41), 1),
		testPacketSentLog(t, txHash, sendLib, 2),
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(42), validExecutorOptions(), 3),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(42), 4),
		testPacketSentLogWithPacket(t, txHash, sendLib, testEncodedPacketWithNonceAndGUID(8, secondGUID), 5),
	}

	records, err := ExecutorSourceTxRecordsFromLogs(logs)
	if err != nil {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() records = %d, want 2", len(records))
	}
	if records[0].Packet.SrcLogIndex != 2 || records[1].Packet.SrcLogIndex != 5 {
		t.Fatalf("source log indexes = %d/%d, want 2/5", records[0].Packet.SrcLogIndex, records[1].Packet.SrcLogIndex)
	}
	if records[0].ExecutorJob.AssignedFee.Cmp(big.NewInt(41)) != 0 || records[1].ExecutorJob.AssignedFee.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("assigned fees = %s/%s, want 41/42", records[0].ExecutorJob.AssignedFee, records[1].ExecutorJob.AssignedFee)
	}
	if records[0].Packet.GUID == records[1].Packet.GUID || records[1].Packet.GUID != secondGUID {
		t.Fatalf("packet GUIDs = %s/%s, want distinct second %s", records[0].Packet.GUID, records[1].Packet.GUID, secondGUID)
	}
}

func testExecutorSourceLogs(t *testing.T, executor, sendLib common.Address, fee *big.Int) []gethtypes.Log {
	t.Helper()
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	return []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, fee, validExecutorOptions(), 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, fee, 1),
		testPacketSentLog(t, txHash, sendLib, 2),
	}
}

func testPacketSentLog(t *testing.T, txHash common.Hash, sendLib common.Address, index uint) gethtypes.Log {
	t.Helper()
	return testPacketSentLogWithPacket(t, txHash, sendLib, testEncodedPacket(), index)
}

func testPacketSentLogWithPacket(t *testing.T, txHash common.Hash, sendLib common.Address, encodedPacket []byte, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.EndpointV2ABI()
	data, err := eventABI.Events["PacketSent"].Inputs.Pack(encodedPacket, []byte{0x01, 0x02}, sendLib)
	if err != nil {
		t.Fatalf("Pack PacketSent error = %v", err)
	}
	return gethtypes.Log{
		Address:     common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Topics:      []common.Hash{lzabi.PacketSentTopic()},
		Data:        data,
		TxHash:      txHash,
		BlockNumber: 123,
		Index:       index,
	}
}

func testEncodedPacketWithNonceAndGUID(nonce uint64, guid common.Hash) []byte {
	encoded := make([]byte, 0, 118)
	encoded = append(encoded, 1)
	encoded = binary.BigEndian.AppendUint64(encoded, nonce)
	encoded = binary.BigEndian.AppendUint32(encoded, 40161)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x7777777777777777777777777777777777777777"))...)
	encoded = binary.BigEndian.AppendUint32(encoded, 40245)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x8888888888888888888888888888888888888888"))...)
	encoded = append(encoded, guid.Bytes()...)
	encoded = append(encoded, []byte("hello")...)
	return encoded
}

func testExecutorFeePaidLog(t *testing.T, txHash common.Hash, sendLib, executor common.Address, fee *big.Int, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.SendUln302ABI()
	data, err := eventABI.Events["ExecutorFeePaid"].Inputs.Pack(executor, fee)
	if err != nil {
		t.Fatalf("Pack ExecutorFeePaid error = %v", err)
	}
	return gethtypes.Log{
		Address:     sendLib,
		Topics:      []common.Hash{lzabi.ExecutorFeePaidTopic()},
		Data:        data,
		TxHash:      txHash,
		BlockNumber: 123,
		Index:       index,
	}
}

func testExecutorJobAssignedLogWithOptions(t *testing.T, txHash common.Hash, executor, sendLib common.Address, fee *big.Int, options []byte, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.OpenExecutorABI()
	data, err := eventABI.Events["ExecutorJobAssigned"].Inputs.NonIndexed().Pack(big.NewInt(5), fee, options)
	if err != nil {
		t.Fatalf("Pack ExecutorJobAssigned error = %v", err)
	}
	return gethtypes.Log{
		Address: executor,
		Topics: []common.Hash{
			lzabi.ExecutorJobAssignedTopic(),
			common.BigToHash(new(big.Int).SetUint64(40245)),
			common.BytesToHash(common.HexToAddress("0x7777777777777777777777777777777777777777").Bytes()),
			common.BytesToHash(sendLib.Bytes()),
		},
		Data:        data,
		TxHash:      txHash,
		BlockNumber: 123,
		Index:       index,
	}
}
