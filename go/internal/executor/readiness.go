package executor

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
)

var (
	endpointViewABI   = lzabi.EndpointV2ABI()
	receiveUlnViewABI = lzabi.ReceiveUln302ABI()
)

var (
	emptyPayloadHash common.Hash
	nilPayloadHash   = common.HexToHash("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
)

// ContractCaller is the eth_call surface used by executor readiness checks.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// IsCommitVerifiable checks the EndpointV2 and ReceiveUln302 readiness gates before commit enqueue.
func IsCommitVerifiable(ctx context.Context, caller ContractCaller, endpoint, receiveLib common.Address, packet db.PacketRecord) (bool, error) {
	if err := packet.Validate(); err != nil {
		return false, err
	}
	if caller == nil {
		return false, errors.New("contract caller is required")
	}
	if endpoint == (common.Address{}) {
		return false, errors.New("endpoint address is required")
	}
	if receiveLib == (common.Address{}) {
		return false, errors.New("receive lib address is required")
	}
	if packet.PayloadHash == emptyPayloadHash {
		return false, nil
	}
	origin := originFromPacket(packet)
	validReceiveLib, err := callBool(ctx, caller, endpointViewABI, endpoint, "isValidReceiveLibrary", packet.Receiver, packet.SrcEID, receiveLib)
	if err != nil {
		return false, err
	}
	if !validReceiveLib {
		return false, nil
	}
	endpointVerifiable, err := callBool(ctx, caller, endpointViewABI, endpoint, "verifiable", origin, packet.Receiver)
	if err != nil {
		return false, err
	}
	if !endpointVerifiable {
		return false, nil
	}
	config, err := callUlnConfig(ctx, caller, receiveLib, packet.Receiver, packet.SrcEID)
	if err != nil {
		return false, err
	}
	return callBool(ctx, caller, receiveUlnViewABI, receiveLib, "verifiable", config, crypto.Keccak256Hash(packet.PacketHeader), packet.PayloadHash)
}

// IsLzReceiveExecutable checks whether EndpointV2 state allows this packet to be delivered.
func IsLzReceiveExecutable(ctx context.Context, caller ContractCaller, endpoint common.Address, packet db.PacketRecord) (bool, error) {
	if err := packet.Validate(); err != nil {
		return false, err
	}
	if caller == nil {
		return false, errors.New("contract caller is required")
	}
	if endpoint == (common.Address{}) {
		return false, errors.New("endpoint address is required")
	}
	origin := originFromPacket(packet)
	payloadHash, err := callHash(ctx, caller, endpoint, "inboundPayloadHash", packet.Receiver, packet.SrcEID, origin.Sender, origin.Nonce)
	if err != nil {
		return false, err
	}
	if payloadHash != packet.PayloadHash {
		return false, nil
	}
	if payloadHash == emptyPayloadHash || payloadHash == nilPayloadHash {
		return false, nil
	}
	inboundNonce, err := callUint64(ctx, caller, endpoint, "inboundNonce", packet.Receiver, packet.SrcEID, origin.Sender)
	if err != nil {
		return false, err
	}
	if origin.Nonce > inboundNonce {
		return false, nil
	}
	lazyInboundNonce, err := callUint64(ctx, caller, endpoint, "lazyInboundNonce", packet.Receiver, packet.SrcEID, origin.Sender)
	if err != nil {
		return false, err
	}
	return origin.Nonce > lazyInboundNonce, nil
}

type ulnConfig struct {
	Confirmations        uint64           `abi:"confirmations"`
	RequiredDVNCount     uint8            `abi:"requiredDVNCount"`
	OptionalDVNCount     uint8            `abi:"optionalDVNCount"`
	OptionalDVNThreshold uint8            `abi:"optionalDVNThreshold"`
	RequiredDVNs         []common.Address `abi:"requiredDVNs"`
	OptionalDVNs         []common.Address `abi:"optionalDVNs"`
}

func originFromPacket(packet db.PacketRecord) endpointOrigin {
	return endpointOrigin{
		SrcEID: packet.SrcEID,
		Sender: common.BytesToHash(
			packet.Sender.Bytes(),
		),
		Nonce: packet.Nonce.Uint64(),
	}
}

func callUlnConfig(ctx context.Context, caller ContractCaller, receiveLib, receiver common.Address, srcEID uint32) (ulnConfig, error) {
	data, err := receiveUlnViewABI.Pack("getUlnConfig", receiver, srcEID)
	if err != nil {
		return ulnConfig{}, err
	}
	result, err := caller.CallContract(ctx, ethereum.CallMsg{To: &receiveLib, Data: data}, nil)
	if err != nil {
		return ulnConfig{}, err
	}
	values, err := receiveUlnViewABI.Unpack("getUlnConfig", result)
	if err != nil {
		return ulnConfig{}, err
	}
	if len(values) != 1 {
		return ulnConfig{}, fmt.Errorf("getUlnConfig returned %d values, want 1", len(values))
	}
	return ulnConfigFromABI(values[0])
}

func ulnConfigFromABI(value any) (ulnConfig, error) {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Struct {
		return ulnConfig{}, fmt.Errorf("getUlnConfig returned %T, want tuple struct", value)
	}
	config := ulnConfig{
		Confirmations:        uint64Field(reflected, "Confirmations"),
		RequiredDVNCount:     uint8Field(reflected, "RequiredDVNCount"),
		OptionalDVNCount:     uint8Field(reflected, "OptionalDVNCount"),
		OptionalDVNThreshold: uint8Field(reflected, "OptionalDVNThreshold"),
	}
	requiredDVNs, ok := reflected.FieldByName("RequiredDVNs").Interface().([]common.Address)
	if !ok {
		return ulnConfig{}, fmt.Errorf("getUlnConfig requiredDVNs has type %T", reflected.FieldByName("RequiredDVNs").Interface())
	}
	optionalDVNs, ok := reflected.FieldByName("OptionalDVNs").Interface().([]common.Address)
	if !ok {
		return ulnConfig{}, fmt.Errorf("getUlnConfig optionalDVNs has type %T", reflected.FieldByName("OptionalDVNs").Interface())
	}
	config.RequiredDVNs = append([]common.Address(nil), requiredDVNs...)
	config.OptionalDVNs = append([]common.Address(nil), optionalDVNs...)
	return config, nil
}

func uint64Field(value reflect.Value, name string) uint64 {
	return value.FieldByName(name).Uint()
}

func uint8Field(value reflect.Value, name string) uint8 {
	return uint8(value.FieldByName(name).Uint())
}

func callBool(ctx context.Context, caller ContractCaller, contractABI abiLike, to common.Address, method string, args ...any) (bool, error) {
	values, err := callAndUnpack(ctx, caller, contractABI, to, method, args...)
	if err != nil {
		return false, err
	}
	value, ok := values[0].(bool)
	if !ok {
		return false, fmt.Errorf("%s returned %T, want bool", method, values[0])
	}
	return value, nil
}

func callHash(ctx context.Context, caller ContractCaller, to common.Address, method string, args ...any) (common.Hash, error) {
	values, err := callAndUnpack(ctx, caller, endpointViewABI, to, method, args...)
	if err != nil {
		return common.Hash{}, err
	}
	value, ok := values[0].([32]byte)
	if !ok {
		return common.Hash{}, fmt.Errorf("%s returned %T, want bytes32", method, values[0])
	}
	return common.BytesToHash(value[:]), nil
}

func callUint64(ctx context.Context, caller ContractCaller, to common.Address, method string, args ...any) (uint64, error) {
	values, err := callAndUnpack(ctx, caller, endpointViewABI, to, method, args...)
	if err != nil {
		return 0, err
	}
	value, ok := values[0].(uint64)
	if !ok {
		return 0, fmt.Errorf("%s returned %T, want uint64", method, values[0])
	}
	return value, nil
}

type abiLike interface {
	Pack(name string, args ...any) ([]byte, error)
	Unpack(name string, data []byte) ([]any, error)
}

func callAndUnpack(ctx context.Context, caller ContractCaller, contractABI abiLike, to common.Address, method string, args ...any) ([]any, error) {
	data, err := contractABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	result, err := caller.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	values, err := contractABI.Unpack(method, result)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("%s returned %d values, want 1", method, len(values))
	}
	return values, nil
}
