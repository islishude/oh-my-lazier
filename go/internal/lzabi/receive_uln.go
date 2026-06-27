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

//go:embed abis/receive_uln302.json
var receiveUln302ABIJSON string

var receiveUln302ABI = mustReceiveUln302ABI()

// ReceiveUln302ABI returns the pinned ReceiveUln302 ABI used by Go workers.
func ReceiveUln302ABI() abi.ABI {
	return receiveUln302ABI
}

// PayloadVerified is the decoded ReceiveUln302 PayloadVerified event.
type PayloadVerified struct {
	DVN           common.Address `abi:"dvn"`
	Header        []byte         `abi:"header"`
	Confirmations *big.Int       `abi:"confirmations"`
	ProofHash     common.Hash    `abi:"proofHash"`
}

// PayloadVerifiedTopic returns the ReceiveUln302 PayloadVerified event topic.
func PayloadVerifiedTopic() common.Hash {
	return receiveUln302ABI.Events["PayloadVerified"].ID
}

// DecodePayloadVerified decodes a ReceiveUln302 PayloadVerified log.
func DecodePayloadVerified(log gethtypes.Log) (PayloadVerified, error) {
	if len(log.Topics) == 0 || log.Topics[0] != PayloadVerifiedTopic() {
		return PayloadVerified{}, errors.New("log is not ReceiveUln302 PayloadVerified")
	}
	var event PayloadVerified
	if err := receiveUln302ABI.UnpackIntoInterface(&event, "PayloadVerified", log.Data); err != nil {
		return PayloadVerified{}, err
	}
	return event, nil
}

// PackReceiveUln302CommitVerification ABI-encodes ReceiveUln302.commitVerification.
func PackReceiveUln302CommitVerification(packetHeader []byte, payloadHash common.Hash) ([]byte, error) {
	return receiveUln302ABI.Pack("commitVerification", packetHeader, payloadHash)
}

// PackReceiveUln302Verify ABI-encodes ReceiveUln302.verify.
func PackReceiveUln302Verify(packetHeader []byte, payloadHash common.Hash, confirmations uint64) ([]byte, error) {
	return receiveUln302ABI.Pack("verify", packetHeader, payloadHash, confirmations)
}

func mustReceiveUln302ABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(receiveUln302ABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
