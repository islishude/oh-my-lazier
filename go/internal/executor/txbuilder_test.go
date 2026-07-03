package executor

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
)

func TestBuildCommitVerificationTx(t *testing.T) {
	packet := testPacketRecord()
	receiveLib := common.HexToAddress("0x3333333333333333333333333333333333333333")

	request, err := BuildCommitVerificationTx(packet, receiveLib, "executor")
	if err != nil {
		t.Fatalf("BuildCommitVerificationTx() error = %v", err)
	}
	if request.ChainEID != packet.DstEID {
		t.Fatalf("chain eid = %d, want %d", request.ChainEID, packet.DstEID)
	}
	if request.Purpose != TxPurposeCommitVerification {
		t.Fatalf("purpose = %q, want %q", request.Purpose, TxPurposeCommitVerification)
	}
	if request.To != receiveLib {
		t.Fatalf("to = %s, want %s", request.To, receiveLib)
	}
	if !bytes.Equal(request.GUID, packet.GUID.Bytes()) {
		t.Fatalf("guid bytes mismatch")
	}
	receiveUlnABI := lzabi.ReceiveUln302ABI()
	if len(request.Calldata) < 4 || !bytes.Equal(request.Calldata[:4], receiveUlnABI.Methods["commitVerification"].ID) {
		t.Fatalf("calldata selector = %x, want commitVerification selector", request.Calldata[:4])
	}
}

func TestBuildLzReceiveTx(t *testing.T) {
	packet := testPacketRecord()
	endpoint := common.HexToAddress("0x4444444444444444444444444444444444444444")

	request, err := BuildLzReceiveTx(packet, endpoint, "executor")
	if err != nil {
		t.Fatalf("BuildLzReceiveTx() error = %v", err)
	}
	if request.ChainEID != packet.DstEID {
		t.Fatalf("chain eid = %d, want %d", request.ChainEID, packet.DstEID)
	}
	if request.Purpose != TxPurposeLzReceive {
		t.Fatalf("purpose = %q, want %q", request.Purpose, TxPurposeLzReceive)
	}
	if request.To != endpoint {
		t.Fatalf("to = %s, want %s", request.To, endpoint)
	}
	if len(request.Calldata) < 4 || !bytes.Equal(request.Calldata[:4], endpointABI.Methods["lzReceive"].ID) {
		t.Fatalf("calldata selector = %x, want lzReceive selector", request.Calldata[:4])
	}
	if request.Value.Sign() != 0 {
		t.Fatalf("tx value = %s, want 0", request.Value)
	}
}

func TestBuildLzReceiveTxRejectsUnsupportedOptions(t *testing.T) {
	packet := testPacketRecord()
	packet.Options[5] = 2

	_, err := BuildLzReceiveTx(packet, common.HexToAddress("0x4444444444444444444444444444444444444444"), "executor")
	if err == nil {
		t.Fatal("BuildLzReceiveTx() error = nil, want unsupported option error")
	}
}

func testPacketRecord() db.PacketRecord {
	return db.PacketRecord{
		GUID:           common.HexToHash("0x0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"),
		SrcEID:         40161,
		DstEID:         40449,
		Nonce:          big.NewInt(7),
		Sender:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Receiver:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		SendLib:        common.HexToAddress("0x9999999999999999999999999999999999999999"),
		SrcTxHash:      common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SrcBlockNumber: 123,
		SrcLogIndex:    4,
		EncodedPacket:  []byte{0x01, 0x02},
		PacketHeader:   []byte{0x01, 0x02, 0x03},
		Message:        []byte{0x04, 0x05},
		PayloadHash:    common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Options:        testLzReceiveOptions(100_000, 0),
		Status:         "ASSIGNED",
	}
}

func testLzReceiveOptions(gasLimit, value uint64) []byte {
	payload := make([]byte, 32)
	new(big.Int).SetUint64(gasLimit).FillBytes(payload[:16])
	new(big.Int).SetUint64(value).FillBytes(payload[16:])

	options := make([]byte, 0, 38)
	options = append(options, 0x00, 0x03)
	options = append(options, 0x01)
	options = append(options, 0x00, 0x21)
	options = append(options, 0x01)
	options = append(options, payload...)
	return options
}
