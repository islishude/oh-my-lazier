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

//go:embed abis/sendlib_base.json
var sendLibBaseABIJSON string

var sendLibBaseABI = mustSendLibBaseABI()

// SendLibBaseABI returns the pinned LayerZero SendLib base ABI used by Go workers.
func SendLibBaseABI() abi.ABI {
	return sendLibBaseABI
}

// ExecutorFeePaid is the decoded LayerZero SendLib executor fee event.
type ExecutorFeePaid struct {
	Executor common.Address
	Fee      *big.Int
}

// DVNFeePaid is the decoded LayerZero SendLib DVN fee event.
type DVNFeePaid struct {
	RequiredDVNs []common.Address
	OptionalDVNs []common.Address
	Fees         []*big.Int
}

// ExecutorFeePaidTopic returns the LayerZero SendLib ExecutorFeePaid event topic.
func ExecutorFeePaidTopic() common.Hash {
	return sendLibBaseABI.Events["ExecutorFeePaid"].ID
}

// DVNFeePaidTopic returns the LayerZero SendLib DVNFeePaid event topic.
func DVNFeePaidTopic() common.Hash {
	return sendLibBaseABI.Events["DVNFeePaid"].ID
}

// DecodeExecutorFeePaid decodes a LayerZero SendLib ExecutorFeePaid log.
func DecodeExecutorFeePaid(log gethtypes.Log) (ExecutorFeePaid, error) {
	if len(log.Topics) == 0 || log.Topics[0] != ExecutorFeePaidTopic() {
		return ExecutorFeePaid{}, errors.New("log is not SendLib ExecutorFeePaid")
	}
	var event ExecutorFeePaid
	if err := sendLibBaseABI.UnpackIntoInterface(&event, "ExecutorFeePaid", log.Data); err != nil {
		return ExecutorFeePaid{}, err
	}
	return event, nil
}

// DecodeDVNFeePaid decodes a LayerZero SendLib DVNFeePaid log.
func DecodeDVNFeePaid(log gethtypes.Log) (DVNFeePaid, error) {
	if len(log.Topics) == 0 || log.Topics[0] != DVNFeePaidTopic() {
		return DVNFeePaid{}, errors.New("log is not SendLib DVNFeePaid")
	}
	var event DVNFeePaid
	if err := sendLibBaseABI.UnpackIntoInterface(&event, "DVNFeePaid", log.Data); err != nil {
		return DVNFeePaid{}, err
	}
	return event, nil
}

func mustSendLibBaseABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(sendLibBaseABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
