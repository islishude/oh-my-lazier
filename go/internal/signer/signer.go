package signer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Signer abstracts transaction and digest signing backends.
type Signer interface {
	Address() common.Address
	SignHash(ctx context.Context, digest common.Hash) ([]byte, error)
	SignTx(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
	Type() string
}
