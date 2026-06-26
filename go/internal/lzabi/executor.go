package lzabi

import (
	_ "embed"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

//go:embed abis/open_executor.json
var openExecutorABIJSON string

var openExecutorABI = mustOpenExecutorABI()

// ExecutorJobAssigned is the decoded OpenExecutor assignment event.
type ExecutorJobAssigned struct {
	DstEID       uint32
	Sender       common.Address
	SendLib      common.Address
	CalldataSize *big.Int
	Price        *big.Int
	Options      []byte
}

// ExecutorJobAssignedTopic returns the OpenExecutor assignment event topic.
func ExecutorJobAssignedTopic() common.Hash {
	return openExecutorABI.Events["ExecutorJobAssigned"].ID
}

// DecodeExecutorJobAssigned decodes an OpenExecutor ExecutorJobAssigned log.
func DecodeExecutorJobAssigned(log gethtypes.Log) (ExecutorJobAssigned, error) {
	if len(log.Topics) != 4 || log.Topics[0] != ExecutorJobAssignedTopic() {
		return ExecutorJobAssigned{}, errors.New("log is not OpenExecutor ExecutorJobAssigned")
	}
	var event ExecutorJobAssigned
	if err := openExecutorABI.UnpackIntoInterface(&event, "ExecutorJobAssigned", log.Data); err != nil {
		return ExecutorJobAssigned{}, err
	}
	event.DstEID = uint32(log.Topics[1].Big().Uint64())
	event.Sender = common.BytesToAddress(log.Topics[2].Bytes()[12:])
	event.SendLib = common.BytesToAddress(log.Topics[3].Bytes()[12:])
	return event, nil
}

func mustOpenExecutorABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(openExecutorABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
