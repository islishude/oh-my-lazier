package lzabi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestDecodeExecutorJobAssigned(t *testing.T) {
	dstEID := uint32(40245)
	sender := common.HexToAddress("0x7777777777777777777777777777777777777777")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	data, err := openExecutorABI.Events["ExecutorJobAssigned"].Inputs.NonIndexed().Pack(
		big.NewInt(128),
		big.NewInt(42),
		[]byte{0x01, 0x02},
	)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodeExecutorJobAssigned(gethtypes.Log{
		Topics: []common.Hash{
			ExecutorJobAssignedTopic(),
			common.BigToHash(new(big.Int).SetUint64(uint64(dstEID))),
			common.BytesToHash(sender.Bytes()),
			common.BytesToHash(sendLib.Bytes()),
		},
		Data: data,
	})
	if err != nil {
		t.Fatalf("DecodeExecutorJobAssigned() error = %v", err)
	}
	if event.DstEID != dstEID {
		t.Fatalf("DstEID = %d, want %d", event.DstEID, dstEID)
	}
	if event.Sender != sender {
		t.Fatalf("Sender = %s, want %s", event.Sender, sender)
	}
	if event.SendLib != sendLib {
		t.Fatalf("SendLib = %s, want %s", event.SendLib, sendLib)
	}
	if event.CalldataSize.Cmp(big.NewInt(128)) != 0 {
		t.Fatalf("CalldataSize = %s", event.CalldataSize)
	}
	if event.Price.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("Price = %s", event.Price)
	}
	if string(event.Options) != string([]byte{0x01, 0x02}) {
		t.Fatalf("Options = %x", event.Options)
	}
}

func TestDecodeExecutorJobAssignedRejectsWrongTopic(t *testing.T) {
	if _, err := DecodeExecutorJobAssigned(gethtypes.Log{Topics: []common.Hash{common.HexToHash("0x01")}}); err == nil {
		t.Fatal("DecodeExecutorJobAssigned() error = nil, want topic error")
	}
}
