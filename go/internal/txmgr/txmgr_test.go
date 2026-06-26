package txmgr

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRunProcessesTargetsUntilQueueIsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, nil)
	manager.pollInterval = time.Hour
	manager.processReceipt = func(context.Context, Target) (int64, error) { return 0, ErrNoReceiptUpdate }
	calls := 0
	manager.processNext = func(context.Context, Target) (int64, error) {
		calls++
		if calls == 1 {
			return 42, nil
		}
		cancel()
		return 0, ErrNoQueuedTx
	}

	err := manager.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("process calls = %d, want 2", calls)
	}
}

func TestRunIgnoresNoQueuedTxUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, nil)
	manager.pollInterval = time.Millisecond
	manager.processReceipt = func(context.Context, Target) (int64, error) { return 0, ErrNoReceiptUpdate }
	calls := 0
	manager.processNext = func(context.Context, Target) (int64, error) {
		calls++
		cancel()
		return 0, ErrNoQueuedTx
	}

	err := manager.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	if calls == 0 {
		t.Fatal("process calls = 0, want at least one")
	}
}

func TestRunReturnsProcessingError(t *testing.T) {
	wantErr := errors.New("broadcast failed")
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, nil)
	manager.processReceipt = func(context.Context, Target) (int64, error) { return 0, ErrNoReceiptUpdate }
	manager.processNext = func(context.Context, Target) (int64, error) {
		return 0, wantErr
	}

	err := manager.Run(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
}

type fakeSigner struct{}

func (fakeSigner) Address() common.Address {
	return common.HexToAddress("0x9999999999999999999999999999999999999999")
}

func (fakeSigner) SignHash(context.Context, common.Hash) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (fakeSigner) SignTx(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error) {
	return nil, errors.New("not implemented")
}

func (fakeSigner) Type() string {
	return "fake"
}
