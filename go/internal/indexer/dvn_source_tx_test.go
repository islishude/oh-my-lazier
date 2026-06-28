package indexer

import (
	"math/big"
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

	records, err := DVNSourceTxRecordsFromLogs(logs, expectedDVN)
	if err != nil {
		t.Fatalf("DVNSourceTxRecordsFromLogs() error = %v", err)
	}
	if records.Packet.SendLib != sendLib {
		t.Fatalf("packet send lib = %s, want %s", records.Packet.SendLib, sendLib)
	}
	if records.DVNJob.GUID != records.Packet.GUID {
		t.Fatalf("dvn job guid = %s, want packet guid %s", records.DVNJob.GUID, records.Packet.GUID)
	}
	if records.DVNJob.ConfirmationsRequired != 12 {
		t.Fatalf("confirmations = %d, want 12", records.DVNJob.ConfirmationsRequired)
	}
	if records.DVNJob.Status != string(packets.DVNAssigned) {
		t.Fatalf("status = %q, want %q", records.DVNJob.Status, packets.DVNAssigned)
	}
}

func TestDVNSourceTxRecordsFromLogsRejectsUnexpectedDVN(t *testing.T) {
	logs := testDVNSourceLogs(
		t,
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
		common.HexToAddress("0x9999999999999999999999999999999999999999"),
		big.NewInt(42),
	)
	_, err := DVNSourceTxRecordsFromLogs(logs, common.HexToAddress("0x4444444444444444444444444444444444444444"))
	if err == nil {
		t.Fatal("DVNSourceTxRecordsFromLogs() error = nil, want dvn mismatch")
	}
}

func testDVNSourceLogs(t *testing.T, dvn, sendLib common.Address, fee *big.Int) []gethtypes.Log {
	t.Helper()
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	return []gethtypes.Log{
		testPacketSentLog(t, txHash, sendLib, 0),
		testDVNFeePaidLog(t, txHash, sendLib, dvn, fee, 1),
		testDVNJobAssignedLog(t, txHash, dvn, sendLib, fee, 2),
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
