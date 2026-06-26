package indexer

import (
	"math/big"
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
}

func TestExecutorJobFromAssignmentRejectsMismatchedSender(t *testing.T) {
	packet := testPacketRecord()
	assignment := lzabi.ExecutorJobAssigned{
		DstEID:  packet.DstEID,
		Sender:  common.HexToAddress("0x1111111111111111111111111111111111111111"),
		SendLib: packet.SendLib,
		Price:   big.NewInt(42),
	}
	if _, err := ExecutorJobFromAssignment(packet, assignment); err == nil {
		t.Fatal("ExecutorJobFromAssignment() error = nil, want sender mismatch")
	}
}

func testPacketRecord() db.PacketRecord {
	return db.PacketRecord{
		GUID:    common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		DstEID:  40245,
		Sender:  common.HexToAddress("0x7777777777777777777777777777777777777777"),
		SendLib: common.HexToAddress("0x9999999999999999999999999999999999999999"),
	}
}
