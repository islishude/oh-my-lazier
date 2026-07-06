package lz

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
)

const (
	optionType3          = 3
	executorWorkerID     = 1
	optionTypeLzReceive  = 1
	optionTypeNativeDrop = 2
	optionTypeLzCompose  = 3
	optionTypeOrdered    = 4
	optionTypeLzRead     = 5
	lzReceiveGasBytes    = 16
	lzReceiveValueBytes  = 16
	lzReceiveOptionBytes = lzReceiveGasBytes + lzReceiveValueBytes
)

// ExecutorOptions is the strict first-phase executor option set supported by OpenExecutor.
type ExecutorOptions struct {
	LzReceiveGas *big.Int
}

// DecodeExecutorOptions decodes the only executor option shape supported in phase 1.
// It accepts either full LayerZero type-3 options or the executor worker-option
// stream that SendUln302 passes to worker contracts after splitting type-3 input.
func DecodeExecutorOptions(options []byte) (ExecutorOptions, error) {
	cursor := 0
	if len(options) >= 2 && binary.BigEndian.Uint16(options[:2]) == optionType3 {
		cursor = 2
	}

	var decoded ExecutorOptions
	hasLzReceive := false
	for cursor < len(options) {
		if len(options) < cursor+4 {
			return ExecutorOptions{}, errors.New("truncated executor option header")
		}
		workerID := options[cursor]
		size := int(binary.BigEndian.Uint16(options[cursor+1 : cursor+3]))
		if size == 0 || len(options) < cursor+3+size {
			return ExecutorOptions{}, errors.New("invalid executor option size")
		}
		optionType := options[cursor+3]
		payload := options[cursor+4 : cursor+3+size]
		cursor += 3 + size

		if workerID != executorWorkerID {
			return ExecutorOptions{}, fmt.Errorf("unsupported worker option id %d", workerID)
		}
		if optionType != optionTypeLzReceive {
			return ExecutorOptions{}, unsupportedExecutorOptionError(optionType)
		}
		if hasLzReceive {
			return ExecutorOptions{}, errors.New("duplicate lzReceive option")
		}
		if len(payload) != lzReceiveGasBytes && len(payload) != lzReceiveOptionBytes {
			return ExecutorOptions{}, fmt.Errorf("invalid lzReceive option payload length %d", len(payload))
		}
		if len(payload) == lzReceiveOptionBytes && new(big.Int).SetBytes(payload[lzReceiveGasBytes:]).Sign() != 0 {
			return ExecutorOptions{}, errors.New("non-zero lzReceive native value")
		}
		gas := new(big.Int).SetBytes(payload[:lzReceiveGasBytes])
		if gas.Sign() == 0 {
			return ExecutorOptions{}, errors.New("lzReceive gas is required")
		}
		decoded.LzReceiveGas = gas
		hasLzReceive = true
	}

	if !hasLzReceive {
		return ExecutorOptions{}, errors.New("missing lzReceive option")
	}
	return decoded, nil
}

func unsupportedExecutorOptionError(optionType byte) error {
	switch optionType {
	case optionTypeNativeDrop:
		return errors.New("unsupported native drop executor option")
	case optionTypeLzCompose:
		return errors.New("unsupported lzCompose executor option")
	case optionTypeOrdered:
		return errors.New("unsupported ordered execution executor option")
	case optionTypeLzRead:
		return errors.New("unsupported lzRead executor option")
	default:
		return fmt.Errorf("unsupported executor option type %d", optionType)
	}
}
