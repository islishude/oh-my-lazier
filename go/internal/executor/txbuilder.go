package executor

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lz"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
)

const (
	// TxPurposeCommitVerification identifies ReceiveUln302.commitVerification outbox requests.
	TxPurposeCommitVerification = "executor_commit_verification"
	// TxPurposeLzReceive identifies EndpointV2.lzReceive outbox requests.
	TxPurposeLzReceive = "executor_lz_receive"
)

var (
	endpointABI = lzabi.EndpointV2ABI()
)

// TxFees carries optional dynamic-fee transaction fee caps for an outbox request.
type TxFees struct {
	GasLimit             *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
}

// BuildCommitVerificationCalldata ABI-encodes ReceiveUln302.commitVerification.
func BuildCommitVerificationCalldata(packet db.PacketRecord) ([]byte, error) {
	if err := packet.Validate(); err != nil {
		return nil, err
	}
	return lzabi.PackReceiveUln302CommitVerification(packet.PacketHeader, packet.PayloadHash)
}

// BuildLzReceiveCalldata ABI-encodes EndpointV2.lzReceive for phase-1 delivery.
func BuildLzReceiveCalldata(packet db.PacketRecord, extraData []byte) ([]byte, error) {
	if err := packet.Validate(); err != nil {
		return nil, err
	}
	origin := endpointOrigin{
		SrcEID: packet.SrcEID,
		Sender: common.BytesToHash(
			packet.Sender.Bytes(),
		),
		Nonce: packet.Nonce.Uint64(),
	}
	return endpointABI.Pack("lzReceive", origin, packet.Receiver, packet.GUID, packet.Message, cloneBytes(extraData))
}

// BuildCommitVerificationTx creates the outbox request for ReceiveUln302.commitVerification.
func BuildCommitVerificationTx(packet db.PacketRecord, receiveLib common.Address, signerID string, fees TxFees) (db.TxRequest, error) {
	if receiveLib == (common.Address{}) {
		return db.TxRequest{}, errors.New("receive lib address is required")
	}
	if signerID == "" {
		return db.TxRequest{}, errors.New("signer id is required")
	}
	calldata, err := BuildCommitVerificationCalldata(packet)
	if err != nil {
		return db.TxRequest{}, err
	}
	return db.TxRequest{
		ChainEID:             packet.DstEID,
		Purpose:              TxPurposeCommitVerification,
		GUID:                 packet.GUID.Bytes(),
		To:                   receiveLib,
		Calldata:             calldata,
		Value:                new(big.Int),
		GasLimit:             cloneBigInt(fees.GasLimit),
		MaxFeePerGas:         cloneBigInt(fees.MaxFeePerGas),
		MaxPriorityFeePerGas: cloneBigInt(fees.MaxPriorityFeePerGas),
		SignerID:             signerID,
	}, nil
}

// BuildLzReceiveTx creates the outbox request for EndpointV2.lzReceive.
func BuildLzReceiveTx(packet db.PacketRecord, endpoint common.Address, signerID string, fees TxFees) (db.TxRequest, error) {
	if endpoint == (common.Address{}) {
		return db.TxRequest{}, errors.New("endpoint address is required")
	}
	if signerID == "" {
		return db.TxRequest{}, errors.New("signer id is required")
	}
	options, err := lz.DecodeExecutorOptions(packet.Options)
	if err != nil {
		return db.TxRequest{}, err
	}
	calldata, err := BuildLzReceiveCalldata(packet, nil)
	if err != nil {
		return db.TxRequest{}, err
	}
	return db.TxRequest{
		ChainEID:             packet.DstEID,
		Purpose:              TxPurposeLzReceive,
		GUID:                 packet.GUID.Bytes(),
		To:                   endpoint,
		Calldata:             calldata,
		Value:                new(big.Int),
		GasLimit:             options.LzReceiveGas,
		MaxFeePerGas:         cloneBigInt(fees.MaxFeePerGas),
		MaxPriorityFeePerGas: cloneBigInt(fees.MaxPriorityFeePerGas),
		SignerID:             signerID,
	}, nil
}

type endpointOrigin struct {
	SrcEID uint32      `abi:"srcEid"`
	Sender common.Hash `abi:"sender"`
	Nonce  uint64      `abi:"nonce"`
}

func cloneBigInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)
	return copied
}
