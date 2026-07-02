package config

import (
	"errors"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

var _ yaml.Unmarshaler = (*EVMAddress)(nil)

// EVMAddress is a 20-byte Ethereum-compatible address decoded at the config boundary.
type EVMAddress common.Address

// ParseEVMAddress parses a required Ethereum-compatible hex address.
func ParseEVMAddress(raw string) (EVMAddress, error) {
	if raw == "" {
		return EVMAddress{}, errors.New("evm address is required")
	}
	if !common.IsHexAddress(raw) {
		return EVMAddress{}, errors.New("evm address must be a 20-byte hex address")
	}
	return EVMAddress(common.HexToAddress(raw)), nil
}

// MustEVMAddress parses an Ethereum-compatible hex address and panics on invalid input.
func MustEVMAddress(raw string) EVMAddress {
	address, err := ParseEVMAddress(raw)
	if err != nil {
		panic(err)
	}
	return address
}

// EVMAddressFromCommon converts a go-ethereum address into a config address value.
func EVMAddressFromCommon(address common.Address) EVMAddress {
	return EVMAddress(address)
}

// UnmarshalYAML decodes an Ethereum-compatible hex address from a YAML scalar.
func (a *EVMAddress) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return errors.New("evm address must be a scalar")
	}
	address, err := ParseEVMAddress(value.Value)
	if err != nil {
		return err
	}
	*a = address
	return nil
}

// Common returns the go-ethereum address value.
func (a EVMAddress) Common() common.Address {
	return common.Address(a)
}

// Hex returns the EIP-55 formatted hex address.
func (a EVMAddress) Hex() string {
	return a.Common().Hex()
}

// String returns the EIP-55 formatted hex address.
func (a EVMAddress) String() string {
	return a.Hex()
}

// IsZero reports whether the address is unset or the zero address.
func (a EVMAddress) IsZero() bool {
	return a.Common() == (common.Address{})
}

// MarshalJSON renders config diffs and reports with hex addresses.
func (a EVMAddress) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(a.Hex())), nil
}

// MarshalText renders the address as EIP-55 formatted hex text.
func (a EVMAddress) MarshalText() ([]byte, error) {
	return []byte(a.Hex()), nil
}
