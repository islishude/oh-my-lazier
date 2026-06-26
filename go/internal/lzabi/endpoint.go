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

//go:embed abis/endpoint_v2.json
var endpointV2ABIJSON string

var endpointV2ABI = mustEndpointV2ABI()

// PacketSent is the decoded EndpointV2 PacketSent event.
type PacketSent struct {
	EncodedPayload []byte
	Options        []byte
	SendLibrary    common.Address
}

// Origin is the LayerZero EndpointV2 origin tuple.
type Origin struct {
	SrcEID uint32      `abi:"srcEid"`
	Sender common.Hash `abi:"sender"`
	Nonce  uint64      `abi:"nonce"`
}

// PacketVerified is the decoded EndpointV2 PacketVerified event.
type PacketVerified struct {
	Origin      Origin         `abi:"origin"`
	Receiver    common.Address `abi:"receiver"`
	PayloadHash common.Hash    `abi:"payloadHash"`
}

// PacketDelivered is the decoded EndpointV2 PacketDelivered event.
type PacketDelivered struct {
	Origin   Origin         `abi:"origin"`
	Receiver common.Address `abi:"receiver"`
}

// LzReceiveAlert is the decoded EndpointV2 LzReceiveAlert event.
type LzReceiveAlert struct {
	Receiver  common.Address
	Executor  common.Address
	Origin    Origin      `abi:"origin"`
	GUID      common.Hash `abi:"guid"`
	Gas       *big.Int    `abi:"gas"`
	Value     *big.Int    `abi:"value"`
	Message   []byte      `abi:"message"`
	ExtraData []byte      `abi:"extraData"`
	Reason    []byte      `abi:"reason"`
}

// PacketSentTopic returns the EndpointV2 PacketSent event topic.
func PacketSentTopic() common.Hash {
	return endpointV2ABI.Events["PacketSent"].ID
}

// PacketVerifiedTopic returns the EndpointV2 PacketVerified event topic.
func PacketVerifiedTopic() common.Hash {
	return endpointV2ABI.Events["PacketVerified"].ID
}

// PacketDeliveredTopic returns the EndpointV2 PacketDelivered event topic.
func PacketDeliveredTopic() common.Hash {
	return endpointV2ABI.Events["PacketDelivered"].ID
}

// LzReceiveAlertTopic returns the EndpointV2 LzReceiveAlert event topic.
func LzReceiveAlertTopic() common.Hash {
	return endpointV2ABI.Events["LzReceiveAlert"].ID
}

// DecodePacketSent decodes an EndpointV2 PacketSent log.
func DecodePacketSent(log gethtypes.Log) (PacketSent, error) {
	if len(log.Topics) == 0 || log.Topics[0] != PacketSentTopic() {
		return PacketSent{}, errors.New("log is not EndpointV2 PacketSent")
	}
	var event PacketSent
	if err := endpointV2ABI.UnpackIntoInterface(&event, "PacketSent", log.Data); err != nil {
		return PacketSent{}, err
	}
	return event, nil
}

// DecodePacketVerified decodes an EndpointV2 PacketVerified log.
func DecodePacketVerified(log gethtypes.Log) (PacketVerified, error) {
	if len(log.Topics) == 0 || log.Topics[0] != PacketVerifiedTopic() {
		return PacketVerified{}, errors.New("log is not EndpointV2 PacketVerified")
	}
	var event PacketVerified
	if err := endpointV2ABI.UnpackIntoInterface(&event, "PacketVerified", log.Data); err != nil {
		return PacketVerified{}, err
	}
	return event, nil
}

// DecodePacketDelivered decodes an EndpointV2 PacketDelivered log.
func DecodePacketDelivered(log gethtypes.Log) (PacketDelivered, error) {
	if len(log.Topics) == 0 || log.Topics[0] != PacketDeliveredTopic() {
		return PacketDelivered{}, errors.New("log is not EndpointV2 PacketDelivered")
	}
	var event PacketDelivered
	if err := endpointV2ABI.UnpackIntoInterface(&event, "PacketDelivered", log.Data); err != nil {
		return PacketDelivered{}, err
	}
	return event, nil
}

// DecodeLzReceiveAlert decodes an EndpointV2 LzReceiveAlert log.
func DecodeLzReceiveAlert(log gethtypes.Log) (LzReceiveAlert, error) {
	if len(log.Topics) == 0 || log.Topics[0] != LzReceiveAlertTopic() {
		return LzReceiveAlert{}, errors.New("log is not EndpointV2 LzReceiveAlert")
	}
	if len(log.Topics) != 3 {
		return LzReceiveAlert{}, errors.New("EndpointV2 LzReceiveAlert must have receiver and executor topics")
	}
	var decoded struct {
		Origin    Origin      `abi:"origin"`
		GUID      common.Hash `abi:"guid"`
		Gas       *big.Int    `abi:"gas"`
		Value     *big.Int    `abi:"value"`
		Message   []byte      `abi:"message"`
		ExtraData []byte      `abi:"extraData"`
		Reason    []byte      `abi:"reason"`
	}
	if err := endpointV2ABI.UnpackIntoInterface(&decoded, "LzReceiveAlert", log.Data); err != nil {
		return LzReceiveAlert{}, err
	}
	return LzReceiveAlert{
		Receiver:  common.BytesToAddress(log.Topics[1].Bytes()[12:]),
		Executor:  common.BytesToAddress(log.Topics[2].Bytes()[12:]),
		Origin:    decoded.Origin,
		GUID:      decoded.GUID,
		Gas:       decoded.Gas,
		Value:     decoded.Value,
		Message:   decoded.Message,
		ExtraData: decoded.ExtraData,
		Reason:    decoded.Reason,
	}, nil
}

func mustEndpointV2ABI() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(endpointV2ABIJSON))
	if err != nil {
		panic(err)
	}
	return parsed
}
