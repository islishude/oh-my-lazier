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

//go:embed abis/open_dvn.json
var openDVNABIJSON string

var openDVNABI = mustOpenDVNABI()

// OpenDVNABI returns the pinned OpenDVN ABI used by Go workers.
func OpenDVNABI() abi.ABI {
	return openDVNABI
}

// DVNJobAssigned is the decoded OpenDVN assignment event.
type DVNJobAssigned struct {
	JobID         common.Hash
	DstEID        uint32
	Sender        common.Address
	SendLib       common.Address
	Confirmations uint64
	Fee           *big.Int
}

// DVNJobAssignedTopic returns the OpenDVN assignment event topic.
func DVNJobAssignedTopic() common.Hash {
	return openDVNABI.Events["DVNJobAssigned"].ID
}

// DecodeDVNJobAssigned decodes an OpenDVN DVNJobAssigned log.
func DecodeDVNJobAssigned(log gethtypes.Log) (DVNJobAssigned, error) {
	if len(log.Topics) != 4 || log.Topics[0] != DVNJobAssignedTopic() {
		return DVNJobAssigned{}, errors.New("log is not OpenDVN DVNJobAssigned")
	}
	var event DVNJobAssigned
	if err := openDVNABI.UnpackIntoInterface(&event, "DVNJobAssigned", log.Data); err != nil {
		return DVNJobAssigned{}, err
	}
	event.JobID = log.Topics[1]
	event.DstEID = uint32(log.Topics[2].Big().Uint64())
	event.Sender = common.BytesToAddress(log.Topics[3].Bytes()[12:])
	return event, nil
}

func mustOpenDVNABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(openDVNABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
