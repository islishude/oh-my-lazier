package indexer

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestExecutorJobFromAssignment(t *testing.T) {
	packet := testPacketRecord()
	assignment := lzabi.ExecutorJobAssigned{
		DstEID:  packet.DstEID,
		Sender:  packet.Sender,
		SendLib: packet.SendLib,
		Price:   big.NewInt(42),
		Options: validExecutorOptions(),
	}
	job, err := ExecutorJobFromAssignment(packet, assignment)
	if err != nil {
		t.Fatalf("ExecutorJobFromAssignment() error = %v", err)
	}
	if job.GUID != packet.GUID {
		t.Fatalf("GUID = %s, want %s", job.GUID, packet.GUID)
	}
	if job.AssignedFee.Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("AssignedFee = %s", job.AssignedFee)
	}
	if job.Status != string(packets.ExecutorAssigned) {
		t.Fatalf("Status = %q, want %q", job.Status, packets.ExecutorAssigned)
	}
	if job.LastError != "" {
		t.Fatalf("LastError = %q, want empty", job.LastError)
	}
}

func TestExecutorJobFromAssignmentMarksUnsupportedOptionsManualReview(t *testing.T) {
	packet := testPacketRecord()
	assignment := lzabi.ExecutorJobAssigned{
		DstEID:  packet.DstEID,
		Sender:  packet.Sender,
		SendLib: packet.SendLib,
		Price:   big.NewInt(42),
		Options: unsupportedExecutorOptions(),
	}
	job, err := ExecutorJobFromAssignment(packet, assignment)
	if err != nil {
		t.Fatalf("ExecutorJobFromAssignment() error = %v", err)
	}
	if job.Status != string(packets.ExecutorManualReview) {
		t.Fatalf("Status = %q, want %q", job.Status, packets.ExecutorManualReview)
	}
	if !strings.Contains(job.LastError, "unsupported executor options") {
		t.Fatalf("LastError = %q, want unsupported options detail", job.LastError)
	}
	if !strings.Contains(job.LastError, "unsupported native drop executor option") {
		t.Fatalf("LastError = %q, want native drop detail", job.LastError)
	}
}

func TestExecutorJobFromAssignmentRejectsMismatchedSender(t *testing.T) {
	packet := testPacketRecord()
	assignment := lzabi.ExecutorJobAssigned{
		DstEID:  packet.DstEID,
		Sender:  common.HexToAddress("0x1111111111111111111111111111111111111111"),
		SendLib: packet.SendLib,
		Price:   big.NewInt(42),
		Options: validExecutorOptions(),
	}
	if _, err := ExecutorJobFromAssignment(packet, assignment); err == nil {
		t.Fatal("ExecutorJobFromAssignment() error = nil, want sender mismatch")
	}
}

func testPacketRecord() db.PacketRecord {
	return db.PacketRecord{
		GUID:    common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		DstEID:  40449,
		Sender:  common.HexToAddress("0x7777777777777777777777777777777777777777"),
		SendLib: common.HexToAddress("0x9999999999999999999999999999999999999999"),
	}
}

func validExecutorOptions() []byte {
	payload := make([]byte, 32)
	new(big.Int).SetUint64(100_000).FillBytes(payload[:16])

	options := make([]byte, 0, 38)
	options = append(options, 0x00, 0x03)
	options = append(options, 0x01)
	options = append(options, 0x00, 0x21)
	options = append(options, 0x01)
	options = append(options, payload...)
	return options
}

func unsupportedExecutorOptions() []byte {
	options := validExecutorOptions()
	options[5] = 2
	return options
}
