package txmgr

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRunProcessesTargetsUntilQueueIsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, discardLogger())
	manager.pollInterval = time.Hour
	calls := 0
	processOnce := func(context.Context) (bool, error) {
		calls++
		if calls == 1 {
			return true, nil
		}
		cancel()
		return false, nil
	}

	err := manager.runLoop(ctx, processOnce)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	if calls != 2 {
		t.Fatalf("process calls = %d, want 2", calls)
	}
}

func TestRunIgnoresNoQueuedTxUntilCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, discardLogger())
	manager.pollInterval = time.Millisecond
	calls := 0
	processOnce := func(context.Context) (bool, error) {
		calls++
		cancel()
		return false, nil
	}

	err := manager.runLoop(ctx, processOnce)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context canceled", err)
	}
	if calls == 0 {
		t.Fatal("process calls = 0, want at least one")
	}
}

func TestRunReturnsProcessingError(t *testing.T) {
	wantErr := errors.New("broadcast failed")
	manager := NewWithTargets(nil, []Target{{ChainEID: 40161, ChainID: big.NewInt(11155111), Signer: fakeSigner{}, Client: &fakeChainClient{}}}, discardLogger())
	processOnce := func(context.Context) (bool, error) {
		return false, wantErr
	}

	err := manager.runLoop(t.Context(), processOnce)
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
