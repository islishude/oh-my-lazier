package indexer

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

// DVNSourceTxRecords are durable records derived from one source-chain DVN assignment transaction.
type DVNSourceTxRecords struct {
	Packet db.PacketRecord
	DVNJob db.DVNJobRecord
}

// DVNSourceTxRecordsFromLogs decodes and cross-checks source-chain DVN logs from one transaction.
func DVNSourceTxRecordsFromLogs(logs []gethtypes.Log, expectedDVN common.Address) (DVNSourceTxRecords, error) {
	if expectedDVN == (common.Address{}) {
		return DVNSourceTxRecords{}, errors.New("expected dvn address is required")
	}
	var packet *db.PacketRecord
	var feePaid *lzabi.DVNFeePaid
	var assignment *lzabi.DVNJobAssigned

	for _, log := range logs {
		switch {
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.PacketSentTopic():
			if packet != nil {
				return DVNSourceTxRecords{}, errors.New("source tx contains multiple PacketSent logs")
			}
			record, err := PacketRecordFromSentLog(log)
			if err != nil {
				return DVNSourceTxRecords{}, err
			}
			packet = &record
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.DVNFeePaidTopic():
			if feePaid != nil {
				return DVNSourceTxRecords{}, errors.New("source tx contains multiple DVNFeePaid logs")
			}
			event, err := lzabi.DecodeDVNFeePaid(log)
			if err != nil {
				return DVNSourceTxRecords{}, err
			}
			feePaid = &event
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.DVNJobAssignedTopic():
			if assignment != nil {
				return DVNSourceTxRecords{}, errors.New("source tx contains multiple DVNJobAssigned logs")
			}
			event, err := lzabi.DecodeDVNJobAssigned(log)
			if err != nil {
				return DVNSourceTxRecords{}, err
			}
			assignment = &event
		}
	}
	if packet == nil {
		return DVNSourceTxRecords{}, errors.New("source tx missing PacketSent log")
	}
	if feePaid == nil {
		return DVNSourceTxRecords{}, errors.New("source tx missing DVNFeePaid log")
	}
	if assignment == nil {
		return DVNSourceTxRecords{}, errors.New("source tx missing DVNJobAssigned log")
	}
	if assignment.DstEID != packet.DstEID {
		return DVNSourceTxRecords{}, fmt.Errorf("dvn assignment dst eid %d does not match packet dst eid %d", assignment.DstEID, packet.DstEID)
	}
	if assignment.Sender != packet.Sender {
		return DVNSourceTxRecords{}, fmt.Errorf("dvn assignment sender %s does not match packet sender %s", assignment.Sender, packet.Sender)
	}
	if assignment.SendLib != packet.SendLib {
		return DVNSourceTxRecords{}, fmt.Errorf("dvn assignment send lib %s does not match packet send lib %s", assignment.SendLib, packet.SendLib)
	}
	if !dvnFeeMatches(*feePaid, expectedDVN, assignment.Fee) {
		return DVNSourceTxRecords{}, fmt.Errorf("dvn fee paid does not include expected dvn %s fee %s", expectedDVN, assignment.Fee)
	}
	return DVNSourceTxRecords{
		Packet: *packet,
		DVNJob: db.DVNJobRecord{
			GUID:                  packet.GUID,
			ConfirmationsRequired: assignment.Confirmations,
			Status:                string(packets.DVNAssigned),
		},
	}, nil
}

func dvnFeeMatches(event lzabi.DVNFeePaid, expectedDVN common.Address, expectedFee *big.Int) bool {
	if expectedFee == nil {
		return false
	}
	dvns := append(append([]common.Address{}, event.RequiredDVNs...), event.OptionalDVNs...)
	if len(dvns) != len(event.Fees) {
		return false
	}
	for i, dvn := range dvns {
		if dvn == expectedDVN && event.Fees[i] != nil && event.Fees[i].Cmp(expectedFee) == 0 {
			return true
		}
	}
	return false
}
