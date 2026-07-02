package txmgr

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/db"
)

func TestQuoteDynamicFeeClampsPriorityTip(t *testing.T) {
	client := &fakeChainClient{
		header:             dynamicHeader(),
		suggestedGasTipCap: big.NewInt(1_500_000_000),
	}
	quote, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1}, FeePolicy{
		MaxFeePerGas:         big.NewInt(3_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(500_000_000),
	}, client)
	if err != nil {
		t.Fatalf("quoteFee() error = %v", err)
	}
	if !quote.Dynamic {
		t.Fatal("quote.Dynamic = false, want true")
	}
	if quote.MaxPriorityFeePerGas.Cmp(big.NewInt(500_000_000)) != 0 {
		t.Fatalf("priority fee = %s, want clamp to 500000000", quote.MaxPriorityFeePerGas)
	}
	if quote.MaxFeePerGas.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("max fee = %s, want 1500000000", quote.MaxFeePerGas)
	}
}

func TestQuoteFeeDefersOverCap(t *testing.T) {
	client := &fakeChainClient{
		header:             dynamicHeader(),
		suggestedGasTipCap: big.NewInt(1_000_000_000),
	}
	_, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1}, FeePolicy{
		MaxFeePerGas:         big.NewInt(1_500_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
	}, client)
	if !errors.Is(err, ErrTxDeferred) {
		t.Fatalf("quoteFee() error = %v, want ErrTxDeferred", err)
	}
}
