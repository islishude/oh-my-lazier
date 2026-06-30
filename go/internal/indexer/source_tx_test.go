package indexer

import (
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

	records, ok, err := ExecutorSourceTxRecordsFromLogs(logs, expectedExecutor)
	if err != nil {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() error = %v", err)
	}
	if !ok {
		t.Fatal("ExecutorSourceTxRecordsFromLogs() ok = false, want true")
	}
	if records.Packet.SendLib != sendLib {
		t.Fatalf("packet send lib = %s, want %s", records.Packet.SendLib, sendLib)
	}
	if records.ExecutorJob.GUID != records.Packet.GUID {
		t.Fatalf("job guid = %s, want packet guid %s", records.ExecutorJob.GUID, records.Packet.GUID)
	}
	if records.ExecutorJob.Status != string(packets.ExecutorAssigned) {
		t.Fatalf("job status = %q, want %q", records.ExecutorJob.Status, packets.ExecutorAssigned)
	}
	if records.ExecutorJob.AssignedFee.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("assigned fee = %s, want 42", records.ExecutorJob.AssignedFee)
	}
}

func TestExecutorSourceTxRecordsFromLogsFiltersUnexpectedExecutor(t *testing.T) {
	logs := testExecutorSourceLogs(
		t,
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		big.NewInt(42),
	)
	_, ok, err := ExecutorSourceTxRecordsFromLogs(logs, common.HexToAddress("0x3333333333333333333333333333333333333333"))
	if err != nil {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("ExecutorSourceTxRecordsFromLogs() ok = true, want false")
	}
}

func TestExecutorSourceTxRecordsFromLogsFiltersUnexpectedExecutorBeforeContextValidation(t *testing.T) {
	otherExecutor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	logs := []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, txHash, otherExecutor, sendLib, big.NewInt(42), validExecutorOptions(), 0),
	}

	_, ok, err := ExecutorSourceTxRecordsFromLogs(logs, common.HexToAddress("0x3333333333333333333333333333333333333333"))
	if err != nil {
		t.Fatalf("ExecutorSourceTxRecordsFromLogs() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("ExecutorSourceTxRecordsFromLogs() ok = true, want false")
	}
}

func testExecutorSourceLogs(t *testing.T, executor, sendLib common.Address, fee *big.Int) []gethtypes.Log {
	t.Helper()
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	return []gethtypes.Log{
		testPacketSentLog(t, txHash, sendLib, 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, fee, 1),
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, fee, validExecutorOptions(), 2),
	}
}

func testPacketSentLog(t *testing.T, txHash common.Hash, sendLib common.Address, index uint) gethtypes.Log {
	t.Helper()
	eventABI := lzabi.EndpointV2ABI()
	data, err := eventABI.Events["PacketSent"].Inputs.Pack(testEncodedPacket(), []byte{0x01, 0x02}, sendLib)
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
