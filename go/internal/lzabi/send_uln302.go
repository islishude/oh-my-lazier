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

//go:embed abis/send_uln302.json
var sendUln302ABIJSON string

var sendUln302ABI = mustSendUln302ABI()

// SendUln302ABI returns the pinned LayerZero SendUln302 ABI used by Go workers.
func SendUln302ABI() abi.ABI {
	return sendUln302ABI
}

// ExecutorFeePaid is the decoded LayerZero SendUln302 executor fee event.
type ExecutorFeePaid struct {
	Executor common.Address
	Fee      *big.Int
}

// DVNFeePaid is the decoded LayerZero SendUln302 DVN fee event.
type DVNFeePaid struct {
	RequiredDVNs []common.Address
	OptionalDVNs []common.Address
	Fees         []*big.Int
}

// ExecutorFeePaidTopic returns the LayerZero SendUln302 ExecutorFeePaid event topic.
func ExecutorFeePaidTopic() common.Hash {
	return sendUln302ABI.Events["ExecutorFeePaid"].ID
}

// DVNFeePaidTopic returns the LayerZero SendUln302 DVNFeePaid event topic.
func DVNFeePaidTopic() common.Hash {
	return sendUln302ABI.Events["DVNFeePaid"].ID
}

// DecodeExecutorFeePaid decodes a LayerZero SendUln302 ExecutorFeePaid log.
func DecodeExecutorFeePaid(log gethtypes.Log) (ExecutorFeePaid, error) {
	if len(log.Topics) == 0 || log.Topics[0] != ExecutorFeePaidTopic() {
		return ExecutorFeePaid{}, errors.New("log is not SendUln302 ExecutorFeePaid")
	}
	var event ExecutorFeePaid
	if err := sendUln302ABI.UnpackIntoInterface(&event, "ExecutorFeePaid", log.Data); err != nil {
		return ExecutorFeePaid{}, err
	}
	return event, nil
}

// DecodeDVNFeePaid decodes a LayerZero SendUln302 DVNFeePaid log.
func DecodeDVNFeePaid(log gethtypes.Log) (DVNFeePaid, error) {
	if len(log.Topics) == 0 || log.Topics[0] != DVNFeePaidTopic() {
		return DVNFeePaid{}, errors.New("log is not SendUln302 DVNFeePaid")
	}
	var event DVNFeePaid
	if err := sendUln302ABI.UnpackIntoInterface(&event, "DVNFeePaid", log.Data); err != nil {
		return DVNFeePaid{}, err
	}
	return event, nil
}

func mustSendUln302ABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(sendUln302ABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
