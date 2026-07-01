package indexer

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
)

// ExecutorSourceTxRecords are the durable records derived from one source-chain send transaction.
type ExecutorSourceTxRecords struct {
	Packet      db.PacketRecord
	ExecutorJob db.ExecutorJobRecord
	Executor    common.Address
}

// ExecutorSourceTxRecordsFromLogs decodes and cross-checks source-chain executor logs from one transaction.
func ExecutorSourceTxRecordsFromLogs(logs []gethtypes.Log) (ExecutorSourceTxRecords, bool, error) {
	var packet *db.PacketRecord
	var feePaid *lzabi.ExecutorFeePaid
	var feeLogAddress common.Address
	var assignment *lzabi.ExecutorJobAssigned
	var assignmentLogAddress common.Address

	for _, log := range logs {
		switch {
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.PacketSentTopic():
			if packet != nil {
				return ExecutorSourceTxRecords{}, false, errors.New("source tx contains multiple PacketSent logs")
			}
			record, err := PacketRecordFromSentLog(log)
			if err != nil {
				return ExecutorSourceTxRecords{}, false, err
			}
			packet = &record
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.ExecutorFeePaidTopic():
			if feePaid != nil {
				return ExecutorSourceTxRecords{}, false, errors.New("source tx contains multiple ExecutorFeePaid logs")
			}
			event, err := lzabi.DecodeExecutorFeePaid(log)
			if err != nil {
				return ExecutorSourceTxRecords{}, false, err
			}
			feePaid = &event
			feeLogAddress = log.Address
		case len(log.Topics) > 0 && log.Topics[0] == lzabi.ExecutorJobAssignedTopic():
			if assignment != nil {
				return ExecutorSourceTxRecords{}, false, errors.New("source tx contains multiple ExecutorJobAssigned logs")
			}
			event, err := lzabi.DecodeExecutorJobAssigned(log)
			if err != nil {
				return ExecutorSourceTxRecords{}, false, err
			}
			assignment = &event
			assignmentLogAddress = log.Address
		}
	}

	if assignment == nil {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx missing ExecutorJobAssigned log")
	}
	if packet == nil {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx missing PacketSent log")
	}
	if feePaid == nil {
		return ExecutorSourceTxRecords{}, false, errors.New("source tx missing ExecutorFeePaid log")
	}
	if feePaid.Executor != assignmentLogAddress {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("executor fee paid executor %s does not match assignment log address %s", feePaid.Executor, assignmentLogAddress)
	}
	if packet.SendLib != feeLogAddress {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("PacketSent send lib %s does not match ExecutorFeePaid log address %s", packet.SendLib, feeLogAddress)
	}
	if assignment.Price == nil || feePaid.Fee == nil || assignment.Price.Cmp(feePaid.Fee) != 0 {
		return ExecutorSourceTxRecords{}, false, fmt.Errorf("executor assignment price %s does not match fee paid %s", assignment.Price, feePaid.Fee)
	}

	job, err := ExecutorJobFromAssignment(*packet, *assignment)
	if err != nil {
		return ExecutorSourceTxRecords{}, false, err
	}
	return ExecutorSourceTxRecords{Packet: *packet, ExecutorJob: job, Executor: assignmentLogAddress}, true, nil
}
