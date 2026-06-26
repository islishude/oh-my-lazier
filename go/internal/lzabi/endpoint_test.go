package lzabi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestDecodePacketSent(t *testing.T) {
	encodedPayload := []byte{0x01, 0x02, 0x03}
	options := []byte{0x04, 0x05}
	sendLibrary := common.HexToAddress("0x9999999999999999999999999999999999999999")
	data, err := endpointV2ABI.Events["PacketSent"].Inputs.Pack(encodedPayload, options, sendLibrary)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodePacketSent(gethtypes.Log{
		Topics: []common.Hash{PacketSentTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodePacketSent() error = %v", err)
	}
	if string(event.EncodedPayload) != string(encodedPayload) {
		t.Fatalf("EncodedPayload = %x, want %x", event.EncodedPayload, encodedPayload)
	}
	if string(event.Options) != string(options) {
		t.Fatalf("Options = %x, want %x", event.Options, options)
	}
	if event.SendLibrary != sendLibrary {
		t.Fatalf("SendLibrary = %s, want %s", event.SendLibrary, sendLibrary)
	}
}

func TestDecodePacketSentRejectsWrongTopic(t *testing.T) {
	_, err := DecodePacketSent(gethtypes.Log{
		Topics: []common.Hash{common.HexToHash("0x01")},
	})
	if err == nil {
		t.Fatal("DecodePacketSent() error = nil, want topic error")
	}
}

func TestDecodePacketVerified(t *testing.T) {
	origin := Origin{
		SrcEID: 40161,
		Sender: common.BytesToHash(common.HexToAddress("0x7777777777777777777777777777777777777777").Bytes()),
		Nonce:  7,
	}
	receiver := common.HexToAddress("0x8888888888888888888888888888888888888888")
	payloadHash := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	data, err := endpointV2ABI.Events["PacketVerified"].Inputs.Pack(origin, receiver, payloadHash)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodePacketVerified(gethtypes.Log{
		Topics: []common.Hash{PacketVerifiedTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodePacketVerified() error = %v", err)
	}
	if event.Origin != origin {
		t.Fatalf("origin = %+v, want %+v", event.Origin, origin)
	}
	if event.Receiver != receiver {
		t.Fatalf("receiver = %s, want %s", event.Receiver, receiver)
	}
	if event.PayloadHash != payloadHash {
		t.Fatalf("payload hash = %s, want %s", event.PayloadHash, payloadHash)
	}
}

func TestDecodePacketDelivered(t *testing.T) {
	origin := Origin{
		SrcEID: 40161,
		Sender: common.BytesToHash(common.HexToAddress("0x7777777777777777777777777777777777777777").Bytes()),
		Nonce:  7,
	}
	receiver := common.HexToAddress("0x8888888888888888888888888888888888888888")
	data, err := endpointV2ABI.Events["PacketDelivered"].Inputs.Pack(origin, receiver)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodePacketDelivered(gethtypes.Log{
		Topics: []common.Hash{PacketDeliveredTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodePacketDelivered() error = %v", err)
	}
	if event.Origin != origin {
		t.Fatalf("origin = %+v, want %+v", event.Origin, origin)
	}
	if event.Receiver != receiver {
		t.Fatalf("receiver = %s, want %s", event.Receiver, receiver)
	}
}

func TestDecodeLzReceiveAlert(t *testing.T) {
	receiver := common.HexToAddress("0x8888888888888888888888888888888888888888")
	executor := common.HexToAddress("0x9999999999999999999999999999999999999999")
	origin := Origin{
		SrcEID: 40161,
		Sender: common.BytesToHash(common.HexToAddress("0x7777777777777777777777777777777777777777").Bytes()),
		Nonce:  7,
	}
	guid := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	data, err := endpointV2ABI.Events["LzReceiveAlert"].Inputs.NonIndexed().Pack(
		origin,
		guid,
		big.NewInt(100_000),
		big.NewInt(0),
		[]byte{0x01, 0x02},
		[]byte{0x03},
		[]byte{0x04, 0x05},
	)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodeLzReceiveAlert(gethtypes.Log{
		Topics: []common.Hash{
			LzReceiveAlertTopic(),
			common.BytesToHash(receiver.Bytes()),
			common.BytesToHash(executor.Bytes()),
		},
		Data: data,
	})
	if err != nil {
		t.Fatalf("DecodeLzReceiveAlert() error = %v", err)
	}
	if event.Receiver != receiver {
		t.Fatalf("receiver = %s, want %s", event.Receiver, receiver)
	}
	if event.Executor != executor {
		t.Fatalf("executor = %s, want %s", event.Executor, executor)
	}
	if event.GUID != guid {
		t.Fatalf("guid = %s, want %s", event.GUID, guid)
	}
	if string(event.Reason) != string([]byte{0x04, 0x05}) {
		t.Fatalf("reason = %x, want 0405", event.Reason)
	}
}
