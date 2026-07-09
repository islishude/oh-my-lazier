package lz

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestDecodePacketV1(t *testing.T) {
	encoded := testEncodedPacket()
	packet, err := DecodePacketV1(encoded)
	if err != nil {
		t.Fatalf("DecodePacketV1() error = %v", err)
	}

	if packet.Version != 1 {
		t.Fatalf("Version = %d, want 1", packet.Version)
	}
	if packet.Nonce != 7 {
		t.Fatalf("Nonce = %d, want 7", packet.Nonce)
	}
	if packet.SrcEID != 40161 || packet.DstEID != 40449 {
		t.Fatalf("pathway = %d -> %d", packet.SrcEID, packet.DstEID)
	}
	if packet.Sender != common.HexToAddress("0x7777777777777777777777777777777777777777") {
		t.Fatalf("Sender = %s", packet.Sender)
	}
	if packet.Receiver != common.HexToAddress("0x8888888888888888888888888888888888888888") {
		t.Fatalf("Receiver = %s", packet.Receiver)
	}
	if packet.GUID != common.HexToHash("0x9999999999999999999999999999999999999999999999999999999999999999") {
		t.Fatalf("GUID = %s", packet.GUID)
	}
	if string(packet.Message) != "hello" {
		t.Fatalf("Message = %q", packet.Message)
	}
	if len(packet.Header) != packetV1HeaderLength {
		t.Fatalf("Header length = %d", len(packet.Header))
	}
	wantPayloadHash := crypto.Keccak256Hash(encoded[packetV1PayloadOffset:])
	if packet.PayloadHash != wantPayloadHash {
		t.Fatalf("PayloadHash = %s, want %s", packet.PayloadHash, wantPayloadHash)
	}
}

func TestDecodePacketV1Header(t *testing.T) {
	encoded := testEncodedPacket()
	header, err := DecodePacketV1Header(encoded[:packetV1HeaderLength])
	if err != nil {
		t.Fatalf("DecodePacketV1Header() error = %v", err)
	}
	if header.Nonce != 7 {
		t.Fatalf("Nonce = %d, want 7", header.Nonce)
	}
	if header.SrcEID != 40161 || header.DstEID != 40449 {
		t.Fatalf("pathway = %d -> %d", header.SrcEID, header.DstEID)
	}
	if header.Sender != common.HexToAddress("0x7777777777777777777777777777777777777777") {
		t.Fatalf("Sender = %s", header.Sender)
	}
	if header.Receiver != common.HexToAddress("0x8888888888888888888888888888888888888888") {
		t.Fatalf("Receiver = %s", header.Receiver)
	}
	if string(header.Header) != string(encoded[:packetV1HeaderLength]) {
		t.Fatal("Header bytes were not preserved")
	}
}

func TestDecodePacketV1RejectsUnsupportedVersion(t *testing.T) {
	encoded := testEncodedPacket()
	encoded[0] = 2
	if _, err := DecodePacketV1(encoded); err == nil {
		t.Fatal("DecodePacketV1() error = nil, want unsupported version")
	}
}

func testEncodedPacket() []byte {
	encoded := make([]byte, 0, packetV1MessageOffset+5)
	encoded = append(encoded, 1)
	encoded = binary.BigEndian.AppendUint64(encoded, 7)
	encoded = binary.BigEndian.AppendUint32(encoded, 40161)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x7777777777777777777777777777777777777777"))...)
	encoded = binary.BigEndian.AppendUint32(encoded, 40449)
	encoded = append(encoded, addressToBytes32(common.HexToAddress("0x8888888888888888888888888888888888888888"))...)
	encoded = append(encoded, common.HexToHash("0x9999999999999999999999999999999999999999999999999999999999999999").Bytes()...)
	encoded = append(encoded, []byte("hello")...)
	return encoded
}

func addressToBytes32(address common.Address) []byte {
	out := make([]byte, 32)
	copy(out[12:], address.Bytes())
	return out
}
