package indexer

import (
	"fmt"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

// ExecutorJobFromAssignment validates an assignment event against a decoded packet.
func ExecutorJobFromAssignment(packet db.PacketRecord, assignment lzabi.ExecutorJobAssigned) (db.ExecutorJobRecord, error) {
	if packet.DstEID != assignment.DstEID {
		return db.ExecutorJobRecord{}, fmt.Errorf("executor assignment dst eid %d does not match packet dst eid %d", assignment.DstEID, packet.DstEID)
	}
	if packet.Sender != assignment.Sender {
		return db.ExecutorJobRecord{}, fmt.Errorf("executor assignment sender %s does not match packet sender %s", assignment.Sender, packet.Sender)
	}
	if packet.SendLib != assignment.SendLib {
		return db.ExecutorJobRecord{}, fmt.Errorf("executor assignment send lib %s does not match packet send lib %s", assignment.SendLib, packet.SendLib)
	}
	return db.ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: assignment.Price,
		Status:      string(packets.ExecutorAssigned),
	}, nil
}
