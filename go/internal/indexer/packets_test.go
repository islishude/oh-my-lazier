package indexer

import (
	"encoding/binary"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestPacketRecordFromSentLog(t *testing.T) {
	encodedPacket := testEncodedPacket()
	options := []byte{0x01, 0x02}
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	eventABI, err := abi.JSON(strings.NewReader(packetSentABIJSON))
	if err != nil {
		t.Fatalf("abi.JSON() error = %v", err)
	}
	data, err := eventABI.Events["PacketSent"].Inputs.Pack(encodedPacket, options, sendLib)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	record, err := PacketRecordFromSentLog(gethtypes.Log{
		Topics:      []common.Hash{lzabi.PacketSentTopic()},
		Data:        data,
		TxHash:      common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		BlockNumber: 123,
		Index:       4,
	})
	if err != nil {
		t.Fatalf("PacketRecordFromSentLog() error = %v", err)
	}

	if record.GUID != common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
		t.Fatalf("GUID = %s", record.GUID)
	}
	if record.Nonce.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("Nonce = %s", record.Nonce)
	}
	if record.SrcEID != 40161 || record.DstEID != 40245 {
		t.Fatalf("pathway = %d -> %d", record.SrcEID, record.DstEID)
	}
	if record.SendLib != sendLib {
		t.Fatalf("SendLib = %s, want %s", record.SendLib, sendLib)
	}
	if record.Status != string(packets.ExecutorNew) {
		t.Fatalf("Status = %q, want %q", record.Status, packets.ExecutorNew)
	}
	if record.PayloadHash != crypto.Keccak256Hash(encodedPacket[81:]) {
		t.Fatalf("PayloadHash = %s", record.PayloadHash)
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
