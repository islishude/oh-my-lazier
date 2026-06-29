package lz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	packetV1Version = 1

	packetV1HeaderLength  = 81
	packetV1PayloadOffset = packetV1HeaderLength
	packetV1GUIDOffset    = packetV1HeaderLength
	packetV1MessageOffset = 113
)

// PacketV1 is a decoded LayerZero PacketV1Codec packet.
type PacketV1 struct {
	Version     uint8
	Nonce       uint64
	SrcEID      uint32
	Sender      common.Address
	DstEID      uint32
	Receiver    common.Address
	GUID        common.Hash
	Message     []byte
	Header      []byte
	PayloadHash common.Hash
}

// DecodePacketV1 decodes bytes emitted as EndpointV2 PacketSent.encodedPayload.
func DecodePacketV1(encoded []byte) (PacketV1, error) {
	if len(encoded) < packetV1MessageOffset {
		return PacketV1{}, fmt.Errorf("packet length %d is shorter than PacketV1 header+guid", len(encoded))
	}
	version := encoded[0]
	if version != packetV1Version {
		return PacketV1{}, fmt.Errorf("unsupported packet version %d", version)
	}
	packet := PacketV1{
		Version:     version,
		Nonce:       binary.BigEndian.Uint64(encoded[1:9]),
		SrcEID:      binary.BigEndian.Uint32(encoded[9:13]),
		Sender:      common.BytesToAddress(encoded[13:45]),
		DstEID:      binary.BigEndian.Uint32(encoded[45:49]),
		Receiver:    common.BytesToAddress(encoded[49:81]),
		GUID:        common.BytesToHash(encoded[packetV1GUIDOffset:packetV1MessageOffset]),
		Message:     bytes.Clone(encoded[packetV1MessageOffset:]),
		Header:      bytes.Clone(encoded[:packetV1HeaderLength]),
		PayloadHash: crypto.Keccak256Hash(encoded[packetV1PayloadOffset:]),
	}
	if packet.Nonce == 0 {
		return PacketV1{}, errors.New("packet nonce is zero")
	}
	if packet.SrcEID == 0 || packet.DstEID == 0 {
		return PacketV1{}, errors.New("packet source and destination eids are required")
	}
	if packet.SrcEID == packet.DstEID {
		return PacketV1{}, errors.New("packet source and destination eids must differ")
	}
	if packet.Sender == (common.Address{}) || packet.Receiver == (common.Address{}) {
		return PacketV1{}, errors.New("packet sender and receiver are required")
	}
	if packet.GUID == (common.Hash{}) {
		return PacketV1{}, errors.New("packet guid is required")
	}
	return packet, nil
}
