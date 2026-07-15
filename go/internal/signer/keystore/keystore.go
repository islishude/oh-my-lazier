package keystore

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// PasswordSource describes where the keystore password should be loaded from.
type PasswordSource struct {
	Value string
	Env   string
	File  string
}

type passwordFileReadError struct {
	cause error
}

func (e *passwordFileReadError) Error() string {
	return "read keystore password file failed"
}

func (e *passwordFileReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

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

// LoadWithPasswordSource decrypts a geth keystore file using a configured password source.
func LoadWithPasswordSource(path string, source PasswordSource) (*Signer, error) {
	password, err := ResolvePassword(source)
	if err != nil {
		return nil, err
	}
	return Load(path, password)
}

// ResolvePassword loads a keystore password from an explicit value, environment variable, or file.
func ResolvePassword(source PasswordSource) (string, error) {
	switch {
	case source.Value != "":
		return source.Value, nil
	case source.Env != "":
		password := os.Getenv(source.Env)
		if password == "" {
			return "", errors.New("keystore password environment variable is empty")
		}
		return password, nil
	case source.File != "":
		raw, err := os.ReadFile(source.File)
		if err != nil {
			return "", &passwordFileReadError{cause: err}
		}
		password := strings.TrimRight(string(raw), "\r\n")
		if password == "" {
			return "", errors.New("keystore password file is empty")
		}
		return password, nil
	default:
		return "", errors.New("keystore password source is required")
	}
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
