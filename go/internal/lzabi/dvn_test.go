package lzabi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestDecodeDVNJobAssigned(t *testing.T) {
	jobID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	dstEID := uint32(40449)
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

func TestDecodePayloadVerified(t *testing.T) {
	header := []byte{0x01, 0x02, 0x03}
	proofHash := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	data, err := receiveUln302ABI.Events["PayloadVerified"].Inputs.Pack(dvn, header, big.NewInt(12), proofHash)
	if err != nil {
		t.Fatalf("Pack PayloadVerified error = %v", err)
	}
	event, err := DecodePayloadVerified(gethtypes.Log{
		Topics: []common.Hash{PayloadVerifiedTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodePayloadVerified() error = %v", err)
	}
	if event.DVN != dvn {
		t.Fatalf("dvn = %s, want %s", event.DVN, dvn)
	}
	if string(event.Header) != string(header) {
		t.Fatalf("header = %x, want %x", event.Header, header)
	}
	if event.Confirmations.Cmp(big.NewInt(12)) != 0 {
		t.Fatalf("confirmations = %s, want 12", event.Confirmations)
	}
	if event.ProofHash != proofHash {
		t.Fatalf("proof hash = %s, want %s", event.ProofHash, proofHash)
	}
}

func TestPackOpenDVNSubmitVerification(t *testing.T) {
	receiveLib := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	packetHeader := []byte{0x01, 0x02, 0x03}
	payloadHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	data, err := PackOpenDVNSubmitVerification(receiveLib, packetHeader, payloadHash, 12)
	if err != nil {
		t.Fatalf("PackOpenDVNSubmitVerification() error = %v", err)
	}
	if string(data[:4]) != string(openDVNABI.Methods["submitVerification"].ID) {
		t.Fatalf("selector = %x", data[:4])
	}
}

func TestDecodeDVNJobAssignedRejectsWrongTopic(t *testing.T) {
	if _, err := DecodeDVNJobAssigned(gethtypes.Log{Topics: []common.Hash{common.HexToHash("0x01")}}); err == nil {
		t.Fatal("DecodeDVNJobAssigned() error = nil, want topic error")
	}
}
