package keystore

import (
	"context"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Signer signs Ethereum transactions with an encrypted geth keystore key.
type Signer struct {
	key     *keystore.Key
	address common.Address
}

// Load decrypts a geth keystore file and returns a signer for its account.
func Load(path string, password string) (*Signer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(raw, password)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, address: key.Address}, nil
}

// Address returns the Ethereum address controlled by the keystore key.
func (s *Signer) Address() common.Address {
	return s.address
}

// SignHash signs a raw digest with the keystore key.
func (s *Signer) SignHash(_ context.Context, digest common.Hash) ([]byte, error) {
	return crypto.Sign(digest.Bytes(), s.key.PrivateKey)
}

// SignTx signs an Ethereum transaction for the given chain ID.
func (s *Signer) SignTx(_ context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	return types.SignTx(tx, types.LatestSignerForChainID(chainID), s.key.PrivateKey)
}

// Type returns the signer backend name.
func (s *Signer) Type() string {
	return "keystore"
}
