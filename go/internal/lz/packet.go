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

// PacketV1Header is the decoded routing identity carried in a PacketV1 header.
type PacketV1Header struct {
	Version  uint8
	Nonce    uint64
	SrcEID   uint32
	Sender   common.Address
	DstEID   uint32
	Receiver common.Address
	Header   []byte
}

// DecodePacketV1Header decodes the fixed PacketV1 header without requiring payload bytes.
func DecodePacketV1Header(header []byte) (PacketV1Header, error) {
	if len(header) != packetV1HeaderLength {
		return PacketV1Header{}, fmt.Errorf("packet header length %d is not %d", len(header), packetV1HeaderLength)
	}
	decoded := PacketV1Header{
		Version:  header[0],
		Nonce:    binary.BigEndian.Uint64(header[1:9]),
		SrcEID:   binary.BigEndian.Uint32(header[9:13]),
		Sender:   common.BytesToAddress(header[13:45]),
		DstEID:   binary.BigEndian.Uint32(header[45:49]),
		Receiver: common.BytesToAddress(header[49:81]),
		Header:   bytes.Clone(header),
	}
	if err := validatePacketV1Header(decoded); err != nil {
		return PacketV1Header{}, err
	}
	return decoded, nil
}

// DecodePacketV1 decodes bytes emitted as EndpointV2 PacketSent.encodedPayload.
func DecodePacketV1(encoded []byte) (PacketV1, error) {
	if len(encoded) < packetV1MessageOffset {
		return PacketV1{}, fmt.Errorf("packet length %d is shorter than PacketV1 header+guid", len(encoded))
	}
	header, err := DecodePacketV1Header(encoded[:packetV1HeaderLength])
	if err != nil {
		return PacketV1{}, err
	}
	packet := PacketV1{
		Version:     header.Version,
		Nonce:       header.Nonce,
		SrcEID:      header.SrcEID,
		Sender:      header.Sender,
		DstEID:      header.DstEID,
		Receiver:    header.Receiver,
		GUID:        common.BytesToHash(encoded[packetV1GUIDOffset:packetV1MessageOffset]),
		Message:     bytes.Clone(encoded[packetV1MessageOffset:]),
		Header:      header.Header,
		PayloadHash: crypto.Keccak256Hash(encoded[packetV1PayloadOffset:]),
	}
	if packet.GUID == (common.Hash{}) {
		return PacketV1{}, errors.New("packet guid is required")
	}
	return packet, nil
}

func validatePacketV1Header(header PacketV1Header) error {
	if header.Version != packetV1Version {
		return fmt.Errorf("unsupported packet version %d", header.Version)
	}
	if header.Nonce == 0 {
		return errors.New("packet nonce is zero")
	}
	if header.SrcEID == 0 || header.DstEID == 0 {
		return errors.New("packet source and destination eids are required")
	}
	if header.SrcEID == header.DstEID {
		return errors.New("packet source and destination eids must differ")
	}
	if header.Sender == (common.Address{}) || header.Receiver == (common.Address{}) {
		return errors.New("packet sender and receiver are required")
	}
	return nil
}
