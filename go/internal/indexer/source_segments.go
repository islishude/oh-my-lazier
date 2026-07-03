package indexer

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func orderedSourceTxLogs(logs []gethtypes.Log) ([]gethtypes.Log, error) {
	if len(logs) == 0 {
		return nil, nil
	}
	ordered := append([]gethtypes.Log(nil), logs...)
	sort.SliceStable(ordered, func(a, b int) bool {
		return ordered[a].Index < ordered[b].Index
	})
	txHash := ordered[0].TxHash
	for _, log := range ordered[1:] {
		if log.TxHash != txHash {
			return nil, fmt.Errorf("source tx logs contain multiple transaction hashes %s and %s", txHash, log.TxHash)
		}
	}
	return ordered, nil
}

func logHasTopic(log gethtypes.Log, topic common.Hash) bool {
	return len(log.Topics) > 0 && log.Topics[0] == topic
}
