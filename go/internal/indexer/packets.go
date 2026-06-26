package indexer

import (
	"math/big"

	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lz"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

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
