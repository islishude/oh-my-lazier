package indexer

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lz"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

const packetV1RouteLength = 81

type packetV1Route struct {
	SrcEID   uint32
	DstEID   uint32
	Sender   common.Address
	Receiver common.Address
}

// packetRouteFromSentLog reads only the fixed routing fields used to reject
// unrelated OApps before strict packet validation.
func packetRouteFromSentLog(log gethtypes.Log) (packetV1Route, lzabi.PacketSent, error) {
	event, err := lzabi.DecodePacketSent(log)
	if err != nil {
		return packetV1Route{}, lzabi.PacketSent{}, err
	}
	if len(event.EncodedPayload) < packetV1RouteLength {
		return packetV1Route{}, lzabi.PacketSent{}, fmt.Errorf("packet length %d is shorter than PacketV1 header", len(event.EncodedPayload))
	}
	return packetV1Route{
		SrcEID:   binary.BigEndian.Uint32(event.EncodedPayload[9:13]),
		Sender:   common.BytesToAddress(event.EncodedPayload[13:45]),
		DstEID:   binary.BigEndian.Uint32(event.EncodedPayload[45:49]),
		Receiver: common.BytesToAddress(event.EncodedPayload[49:81]),
	}, event, nil
}

// PacketRecordFromSentLog decodes an EndpointV2 PacketSent log into a database record.
func PacketRecordFromSentLog(log gethtypes.Log) (db.PacketRecord, error) {
	event, err := lzabi.DecodePacketSent(log)
	if err != nil {
		return db.PacketRecord{}, err
	}
	packet, err := lz.DecodePacketV1(event.EncodedPayload)
	if err != nil {
		return db.PacketRecord{}, err
	}
	return db.PacketRecord{
		GUID:           packet.GUID,
		SrcEID:         packet.SrcEID,
		DstEID:         packet.DstEID,
		Nonce:          new(big.Int).SetUint64(packet.Nonce),
		Sender:         packet.Sender,
		Receiver:       packet.Receiver,
		SendLib:        event.SendLibrary,
		SrcTxHash:      log.TxHash,
		SrcBlockNumber: log.BlockNumber,
		SrcLogIndex:    log.Index,
		EncodedPacket:  event.EncodedPayload,
		PacketHeader:   packet.Header,
		Message:        packet.Message,
		PayloadHash:    packet.PayloadHash,
		Options:        event.Options,
		Status:         string(packets.ExecutorNew),
	}, nil
}
