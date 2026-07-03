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

// DVNSourceTxRecords are durable records derived from one source-chain DVN assignment.
type DVNSourceTxRecords struct {
	Packet db.PacketRecord
	DVNJob db.DVNJobRecord
	DVN    common.Address
}

// DVNSourceTxRecordsFromLogs decodes and cross-checks source-chain DVN logs from one transaction.
func DVNSourceTxRecordsFromLogs(logs []gethtypes.Log) ([]DVNSourceTxRecords, error) {
	return DVNSourceTxRecordsFromLogsForEndpoint(logs, common.Address{})
}

// DVNSourceTxRecordsFromLogsForEndpoint decodes source-chain DVN logs and requires PacketSent from endpoint when set.
func DVNSourceTxRecordsFromLogsForEndpoint(logs []gethtypes.Log, endpoint common.Address) ([]DVNSourceTxRecords, error) {
	ordered, err := orderedSourceTxLogs(logs)
	if err != nil {
		return nil, err
	}

	records := make([]DVNSourceTxRecords, 0)
	feeEvents := make([]dvnFeeEvent, 0)
	assignments := make([]dvnAssignmentEvent, 0)
	for _, log := range ordered {
		switch {
		case logHasTopic(log, lzabi.DVNJobAssignedTopic()):
			event, err := lzabi.DecodeDVNJobAssigned(log)
			if err != nil {
				return nil, err
			}
			assignments = append(assignments, dvnAssignmentEvent{Log: log, Event: event})
		case logHasTopic(log, lzabi.DVNFeePaidTopic()):
			event, err := lzabi.DecodeDVNFeePaid(log)
			if err != nil {
				return nil, err
			}
			feeEvents = append(feeEvents, dvnFeeEvent{Log: log, Event: event})
		case logHasTopic(log, lzabi.PacketSentTopic()):
			if endpoint != (common.Address{}) && log.Address != endpoint {
				return nil, fmt.Errorf("source tx PacketSent address %s does not match endpoint %s", log.Address, endpoint)
			}
			packet, err := PacketRecordFromSentLog(log)
			if err != nil {
				return nil, err
			}
			feeIndex := latestUnmatchedDVNFee(feeEvents, packet.SendLib)
			if feeIndex < 0 {
				if hasUnmatchedDVNAssignmentForPacket(assignments, packet) {
					return nil, errors.New("source tx PacketSent missing matching DVNFeePaid log")
				}
				continue
			}
			assignmentIndex, err := matchingDVNAssignment(assignments, packet, feeEvents[feeIndex].Event)
			if err != nil {
				return nil, err
			}
			feeEvents[feeIndex].Matched = true
			if assignmentIndex >= 0 {
				assignments[assignmentIndex].Matched = true
				record := dvnRecordFromMatchedLogs(packet, assignments[assignmentIndex])
				records = append(records, record)
			}
		}
	}
	if hasUnmatchedDVNAssignment(assignments) {
		return nil, errors.New("source tx contains DVNJobAssigned without following PacketSent")
	}
	return records, nil
}

type dvnFeeEvent struct {
	Log     gethtypes.Log
	Event   lzabi.DVNFeePaid
	Matched bool
}

type dvnAssignmentEvent struct {
	Log     gethtypes.Log
	Event   lzabi.DVNJobAssigned
	Matched bool
}

func latestUnmatchedDVNFee(fees []dvnFeeEvent, sendLib common.Address) int {
	for i := len(fees) - 1; i >= 0; i-- {
		if !fees[i].Matched && fees[i].Log.Address == sendLib {
			return i
		}
	}
	return -1
}

func matchingDVNAssignment(assignments []dvnAssignmentEvent, packet db.PacketRecord, fee lzabi.DVNFeePaid) (int, error) {
	candidates := make([]int, 0, 1)
	matches := make([]int, 0, 1)
	for i, assignment := range assignments {
		if assignment.Matched || !dvnAssignmentMatchesPacket(assignment.Event, packet) {
			continue
		}
		candidates = append(candidates, i)
		if dvnFeeMatches(fee, assignment.Log.Address, assignment.Event.Fee) {
			matches = append(matches, i)
		}
	}
	if len(matches) > 1 {
		return -1, fmt.Errorf("packet %s matches multiple DVNJobAssigned logs", packet.GUID)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(candidates) > 0 {
		assignment := assignments[candidates[0]]
		return -1, fmt.Errorf("dvn fee paid does not include assigned dvn %s fee %s", assignment.Log.Address, assignment.Event.Fee)
	}
	return -1, nil
}

func hasUnmatchedDVNAssignmentForPacket(assignments []dvnAssignmentEvent, packet db.PacketRecord) bool {
	for _, assignment := range assignments {
		if !assignment.Matched && dvnAssignmentMatchesPacket(assignment.Event, packet) {
			return true
		}
	}
	return false
}

func hasUnmatchedDVNAssignment(assignments []dvnAssignmentEvent) bool {
	for _, assignment := range assignments {
		if !assignment.Matched {
			return true
		}
	}
	return false
}

func dvnAssignmentMatchesPacket(assignment lzabi.DVNJobAssigned, packet db.PacketRecord) bool {
	return assignment.DstEID == packet.DstEID && assignment.Sender == packet.Sender && assignment.SendLib == packet.SendLib
}

func dvnRecordFromMatchedLogs(packet db.PacketRecord, assignment dvnAssignmentEvent) DVNSourceTxRecords {
	return DVNSourceTxRecords{
		Packet: packet,
		DVNJob: db.DVNJobRecord{
			GUID:                  packet.GUID,
			ConfirmationsRequired: assignment.Event.Confirmations,
			Status:                string(packets.DVNAssigned),
		},
		DVN: assignment.Log.Address,
	}
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
