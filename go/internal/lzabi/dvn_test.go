package lzabi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestDecodeDVNJobAssigned(t *testing.T) {
	jobID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	dstEID := uint32(40245)
	sender := common.HexToAddress("0x7777777777777777777777777777777777777777")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	data, err := openDVNABI.Events["DVNJobAssigned"].Inputs.NonIndexed().Pack(sendLib, uint64(12), big.NewInt(42))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodeDVNJobAssigned(gethtypes.Log{
		Topics: []common.Hash{
			DVNJobAssignedTopic(),
			jobID,
			common.BigToHash(new(big.Int).SetUint64(uint64(dstEID))),
			common.BytesToHash(sender.Bytes()),
		},
		Data: data,
	})
	if err != nil {
		t.Fatalf("DecodeDVNJobAssigned() error = %v", err)
	}
	if event.JobID != jobID {
		t.Fatalf("JobID = %s, want %s", event.JobID, jobID)
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
	if event.Confirmations != 12 {
		t.Fatalf("Confirmations = %d, want 12", event.Confirmations)
	}
	if event.Fee.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("Fee = %s, want 42", event.Fee)
	}
}

func TestDecodeDVNJobAssignedRejectsWrongTopic(t *testing.T) {
	if _, err := DecodeDVNJobAssigned(gethtypes.Log{Topics: []common.Hash{common.HexToHash("0x01")}}); err == nil {
		t.Fatal("DecodeDVNJobAssigned() error = nil, want topic error")
	}
}
