package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/bigutil"
	"github.com/jackc/pgx/v5"
)

// PacketRecord is the normalized packet data persisted from a PacketSent log.
type PacketRecord struct {
	GUID           common.Hash
	SrcEID         uint32
	DstEID         uint32
	Nonce          *big.Int
	Sender         common.Address
	Receiver       common.Address
	SendLib        common.Address
	SrcTxHash      common.Hash
	SrcBlockNumber uint64
	SrcLogIndex    uint
	EncodedPacket  []byte
	PacketHeader   []byte
	Message        []byte
	PayloadHash    common.Hash
	Options        []byte
	Status         string
}

// UpsertPacket persists packet data from source-chain indexing.
func (s *Store) UpsertPacket(ctx context.Context, packet PacketRecord) error {
	if err := packet.Validate(); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO packets (
			guid, src_eid, dst_eid, nonce, sender, receiver, send_lib,
			src_tx_hash, src_block_number, src_log_index, encoded_packet,
			packet_header, message, payload_hash, options, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (guid) DO UPDATE SET
			src_eid = EXCLUDED.src_eid,
			dst_eid = EXCLUDED.dst_eid,
			nonce = EXCLUDED.nonce,
			sender = EXCLUDED.sender,
			receiver = EXCLUDED.receiver,
			send_lib = EXCLUDED.send_lib,
			src_tx_hash = EXCLUDED.src_tx_hash,
			src_block_number = EXCLUDED.src_block_number,
			src_log_index = EXCLUDED.src_log_index,
			encoded_packet = EXCLUDED.encoded_packet,
			packet_header = EXCLUDED.packet_header,
			message = EXCLUDED.message,
			payload_hash = EXCLUDED.payload_hash,
			options = EXCLUDED.options,
			status = EXCLUDED.status,
			updated_at = now()
	`, packet.GUID.Bytes(), packet.SrcEID, packet.DstEID, packet.Nonce.String(), addressBytes(packet.Sender), addressBytes(packet.Receiver), addressBytes(packet.SendLib), packet.SrcTxHash.Bytes(), packet.SrcBlockNumber, packet.SrcLogIndex, bytes.Clone(packet.EncodedPacket), bytes.Clone(packet.PacketHeader), bytes.Clone(packet.Message), packet.PayloadHash.Bytes(), bytes.Clone(packet.Options), packet.Status)
	return err
}

// GetPacket returns one packet by GUID.
func (s *Store) GetPacket(ctx context.Context, guid common.Hash) (PacketRecord, error) {
	if guid == (common.Hash{}) {
		return PacketRecord{}, errors.New("packet guid is required")
	}
	return s.scanPacket(ctx, `
		SELECT
			guid, src_eid, dst_eid, nonce::text, sender, receiver, send_lib,
			src_tx_hash, src_block_number, src_log_index, encoded_packet,
			packet_header, message, payload_hash, options, status
		FROM packets
		WHERE guid = $1
	`, guid.Bytes())
}

// GetPacketByDestination returns one packet by the EndpointV2 destination event identity.
func (s *Store) GetPacketByDestination(ctx context.Context, dstEID, srcEID uint32, sender, receiver common.Address, nonce uint64) (PacketRecord, error) {
	if dstEID == 0 || srcEID == 0 {
		return PacketRecord{}, errors.New("packet source and destination eids are required")
	}
	if sender == (common.Address{}) || receiver == (common.Address{}) {
		return PacketRecord{}, errors.New("packet sender and receiver are required")
	}
	if nonce == 0 {
		return PacketRecord{}, errors.New("packet nonce is required")
	}
	return s.scanPacket(ctx, `
		SELECT
			guid, src_eid, dst_eid, nonce::text, sender, receiver, send_lib,
			src_tx_hash, src_block_number, src_log_index, encoded_packet,
			packet_header, message, payload_hash, options, status
		FROM packets
		WHERE dst_eid = $1 AND src_eid = $2 AND sender = $3 AND receiver = $4 AND nonce = $5
	`, dstEID, srcEID, addressBytes(sender), addressBytes(receiver), new(big.Int).SetUint64(nonce).String())
}

// GetPacketByVerification returns one packet by ReceiveUln302 PayloadVerified identity.
func (s *Store) GetPacketByVerification(ctx context.Context, dstEID uint32, packetHeader []byte, payloadHash common.Hash) (PacketRecord, error) {
	if dstEID == 0 {
		return PacketRecord{}, errors.New("packet destination eid is required")
	}
	if len(packetHeader) == 0 {
		return PacketRecord{}, errors.New("packet header is required")
	}
	if payloadHash == (common.Hash{}) {
		return PacketRecord{}, errors.New("packet payload hash is required")
	}
	return s.scanPacket(ctx, `
		SELECT
			guid, src_eid, dst_eid, nonce::text, sender, receiver, send_lib,
			src_tx_hash, src_block_number, src_log_index, encoded_packet,
			packet_header, message, payload_hash, options, status
		FROM packets
		WHERE dst_eid = $1 AND packet_header = $2 AND payload_hash = $3
	`, dstEID, bytes.Clone(packetHeader), payloadHash.Bytes())
}

// Validate checks packet persistence invariants before writing to Postgres.
func (p PacketRecord) Validate() error {
	if p.GUID == (common.Hash{}) {
		return errors.New("packet guid is required")
	}
	if p.SrcEID == 0 || p.DstEID == 0 {
		return errors.New("packet source and destination eids are required")
	}
	if p.SrcEID == p.DstEID {
		return errors.New("packet source and destination eids must differ")
	}
	if p.Nonce == nil || p.Nonce.Sign() < 0 {
		return errors.New("packet nonce must be non-negative")
	}
	for label, address := range map[string]common.Address{
		"sender":   p.Sender,
		"receiver": p.Receiver,
		"send_lib": p.SendLib,
	} {
		if address == (common.Address{}) {
			return fmt.Errorf("packet %s is required", label)
		}
	}
	if p.SrcTxHash == (common.Hash{}) {
		return errors.New("packet source tx hash is required")
	}
	if p.PayloadHash == (common.Hash{}) {
		return errors.New("packet payload hash is required")
	}
	if len(p.EncodedPacket) == 0 {
		return errors.New("packet encoded bytes are required")
	}
	if len(p.PacketHeader) == 0 {
		return errors.New("packet header is required")
	}
	if p.Status == "" {
		return errors.New("packet status is required")
	}
	return nil
}

func (s *Store) scanPacket(ctx context.Context, sql string, args ...any) (PacketRecord, error) {
	var row packetRow
	err := s.pool.QueryRow(ctx, sql, args...).Scan(
		&row.GUID,
		&row.SrcEID,
		&row.DstEID,
		&row.Nonce,
		&row.Sender,
		&row.Receiver,
		&row.SendLib,
		&row.SrcTxHash,
		&row.SrcBlockNumber,
		&row.SrcLogIndex,
		&row.EncodedPacket,
		&row.PacketHeader,
		&row.Message,
		&row.PayloadHash,
		&row.Options,
		&row.Status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PacketRecord{}, pgx.ErrNoRows
	}
	if err != nil {
		return PacketRecord{}, err
	}
	return row.toPacketRecord()
}

type packetRow struct {
	GUID           []byte
	SrcEID         uint32
	DstEID         uint32
	Nonce          string
	Sender         []byte
	Receiver       []byte
	SendLib        []byte
	SrcTxHash      []byte
	SrcBlockNumber uint64
	SrcLogIndex    uint
	EncodedPacket  []byte
	PacketHeader   []byte
	Message        []byte
	PayloadHash    []byte
	Options        []byte
	Status         string
}

func (r packetRow) toPacketRecord() (PacketRecord, error) {
	if len(r.GUID) != common.HashLength {
		return PacketRecord{}, fmt.Errorf("packet guid has length %d", len(r.GUID))
	}
	if len(r.Sender) != common.AddressLength {
		return PacketRecord{}, fmt.Errorf("packet sender has length %d", len(r.Sender))
	}
	if len(r.Receiver) != common.AddressLength {
		return PacketRecord{}, fmt.Errorf("packet receiver has length %d", len(r.Receiver))
	}
	if len(r.SendLib) != common.AddressLength {
		return PacketRecord{}, fmt.Errorf("packet send_lib has length %d", len(r.SendLib))
	}
	if len(r.SrcTxHash) != common.HashLength {
		return PacketRecord{}, fmt.Errorf("packet src_tx_hash has length %d", len(r.SrcTxHash))
	}
	if len(r.PayloadHash) != common.HashLength {
		return PacketRecord{}, fmt.Errorf("packet payload_hash has length %d", len(r.PayloadHash))
	}
	nonce, err := bigutil.ParseDecimal("packet nonce", r.Nonce)
	if err != nil {
		return PacketRecord{}, fmt.Errorf("packet nonce %q is not a valid integer", r.Nonce)
	}
	return PacketRecord{
		GUID:           common.BytesToHash(r.GUID),
		SrcEID:         r.SrcEID,
		DstEID:         r.DstEID,
		Nonce:          nonce,
		Sender:         common.BytesToAddress(r.Sender),
		Receiver:       common.BytesToAddress(r.Receiver),
		SendLib:        common.BytesToAddress(r.SendLib),
		SrcTxHash:      common.BytesToHash(r.SrcTxHash),
		SrcBlockNumber: r.SrcBlockNumber,
		SrcLogIndex:    r.SrcLogIndex,
		EncodedPacket:  bytes.Clone(r.EncodedPacket),
		PacketHeader:   bytes.Clone(r.PacketHeader),
		Message:        bytes.Clone(r.Message),
		PayloadHash:    common.BytesToHash(r.PayloadHash),
		Options:        bytes.Clone(r.Options),
		Status:         r.Status,
	}, nil
}
