package kms

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer signs Ethereum payloads through AWS KMS.
type Signer struct {
	address common.Address
}

// New creates a KMS signer placeholder for the expected Ethereum address.
func New(address common.Address) *Signer {
	return &Signer{address: address}
}

// Address returns the Ethereum address expected to recover from KMS signatures.
func (s *Signer) Address() common.Address {
	return s.address
}

// SignHash requests an ECDSA signature for a raw digest.
func (s *Signer) SignHash(context.Context, common.Hash) ([]byte, error) {
	return nil, errors.New("kms signer is not configured")
}

// SignTx signs an Ethereum transaction with AWS KMS.
func (s *Signer) SignTx(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error) {
	return nil, errors.New("kms signer is not configured")
}

// Type returns the signer backend name.
func (s *Signer) Type() string {
	return "kms"
}
