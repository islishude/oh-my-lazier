package indexer

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
)

// ExecutorSourceTxRecords are the durable records derived from one source-chain send.
type ExecutorSourceTxRecords struct {
	Packet      db.PacketRecord
	ExecutorJob db.ExecutorJobRecord
	Executor    common.Address
}

// ExecutorSourceTxRecordsFromLogs decodes and cross-checks source-chain executor logs from one transaction.
func ExecutorSourceTxRecordsFromLogs(logs []gethtypes.Log) ([]ExecutorSourceTxRecords, error) {
	ordered, err := orderedSourceTxLogs(logs)
	if err != nil {
		return nil, err
	}

	records := make([]ExecutorSourceTxRecords, 0)
	var segment executorSourceSegment
	for _, log := range ordered {
		switch {
		case logHasTopic(log, lzabi.ExecutorJobAssignedTopic()):
			if segment.assignment != nil {
				return nil, errors.New("source tx send segment contains multiple ExecutorJobAssigned logs")
			}
			event, err := lzabi.DecodeExecutorJobAssigned(log)
			if err != nil {
				return nil, err
			}
			segment.assignment = &event
			segment.assignmentLogAddress = log.Address
		case logHasTopic(log, lzabi.ExecutorFeePaidTopic()):
			event, err := lzabi.DecodeExecutorFeePaid(log)
			if err != nil {
				return nil, err
			}
			segment.feePaid = &event
			segment.feeLogAddress = log.Address
			segment.feePaidCount++
		case logHasTopic(log, lzabi.PacketSentTopic()):
			record, ok, err := segment.recordsFromPacket(log)
			if err != nil {
				return nil, err
			}
			if ok {
				records = append(records, record)
			}
			segment = executorSourceSegment{}
		}
	}
	if segment.assignment != nil {
		return nil, errors.New("source tx contains ExecutorJobAssigned without following PacketSent")
	}
	return records, nil
}

type executorSourceSegment struct {
	feePaid              *lzabi.ExecutorFeePaid
	feeLogAddress        common.Address
	feePaidCount         int
	assignment           *lzabi.ExecutorJobAssigned
	assignmentLogAddress common.Address
}

func (s executorSourceSegment) recordsFromPacket(log gethtypes.Log) (ExecutorSourceTxRecords, bool, error) {
	if s.assignment == nil {
		return ExecutorSourceTxRecords{}, false, nil
	}
	packet, err := PacketRecordFromSentLog(log)
	if err != nil {
		return ExecutorSourceTxRecords{}, false, err
	}
	if s.feePaidCount == 0 {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx send segment missing ExecutorFeePaid log")
	}
	if s.feePaidCount > 1 {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx send segment contains multiple ExecutorFeePaid logs")
	}
	if s.feePaid == nil {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx send segment missing ExecutorFeePaid log")
	}

	if s.feePaid.Executor != s.assignmentLogAddress {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("executor fee paid executor %s does not match assignment log address %s", s.feePaid.Executor, s.assignmentLogAddress)
	}
	if packet.SendLib != s.feeLogAddress {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("PacketSent send lib %s does not match ExecutorFeePaid log address %s", packet.SendLib, s.feeLogAddress)
	}
	if s.assignment.Price == nil || s.feePaid.Fee == nil || s.assignment.Price.Cmp(s.feePaid.Fee) != 0 {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("executor assignment price %s does not match fee paid %s", s.assignment.Price, s.feePaid.Fee)
	}

	job, err := ExecutorJobFromAssignment(packet, *s.assignment)
	if err != nil {
		return ExecutorSourceTxRecords{}, false, err
	}
	return ExecutorSourceTxRecords{Packet: packet, ExecutorJob: job, Executor: s.assignmentLogAddress}, true, nil
}
