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
		ConfiguredMaxFeePerGas:         big.NewInt(3_000_000_000),
		ConfiguredMaxPriorityFeePerGas: big.NewInt(500_000_000),
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

func TestQuoteFeeSelectsTypeFromHeader(t *testing.T) {
	for _, tt := range []struct {
		name        string
		baseFee     *big.Int
		wantDynamic bool
	}{
		{name: "no base fee"},
		{name: "zero base fee", baseFee: big.NewInt(0), wantDynamic: true},
		{name: "positive base fee", baseFee: big.NewInt(500_000_000), wantDynamic: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			header := legacyHeader()
			header.BaseFee = tt.baseFee
			client := &fakeChainClient{header: header, suggestedGasPrice: big.NewInt(1_000_000_000), suggestedGasTipCap: big.NewInt(500_000_000)}
			policy := FeePolicy{ConfiguredMaxFeePerGas: big.NewInt(3_000_000_000)}
			if tt.wantDynamic {
				policy.ConfiguredMaxPriorityFeePerGas = big.NewInt(500_000_000)
			}
			quote, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1}, policy, client)
			if err != nil {
				t.Fatalf("quoteFee() error = %v", err)
			}
			if quote.Dynamic != tt.wantDynamic {
				t.Fatalf("quote.Dynamic = %v, want %v", quote.Dynamic, tt.wantDynamic)
			}
			wantFee := big.NewInt(1_000_000_000)
			if tt.wantDynamic {
				wantFee.Add(new(big.Int).Mul(tt.baseFee, big.NewInt(2)), policy.ConfiguredMaxPriorityFeePerGas)
			}
			if quote.MaxFeePerGas.Cmp(wantFee) != 0 {
				t.Fatalf("fee = %s, want %s", quote.MaxFeePerGas, wantFee)
			}
		})
	}
}

func TestQuoteFeeDefersOverCap(t *testing.T) {
	client := &fakeChainClient{
		header:             dynamicHeader(),
		suggestedGasTipCap: big.NewInt(1_000_000_000),
	}
	_, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1}, FeePolicy{
		ConfiguredMaxFeePerGas:         big.NewInt(1_500_000_000),
		ConfiguredMaxPriorityFeePerGas: big.NewInt(1_000_000_000),
	}, client)
	if !errors.Is(err, ErrTxDeferred) {
		t.Fatalf("quoteFee() error = %v, want ErrTxDeferred", err)
	}
}

func TestQuoteFeeAssignedNonceWithoutPreviousFeesUsesFreshQuote(t *testing.T) {
	nonce := uint64(7)
	t.Run("dynamic", func(t *testing.T) {
		client := &fakeChainClient{
			header:             dynamicHeader(),
			suggestedGasTipCap: big.NewInt(1_000_000_000),
		}
		quote, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1, Nonce: &nonce}, defaultFeePolicy(), client)
		if err != nil {
			t.Fatalf("quoteFee() error = %v", err)
		}
		if quote.MaxFeePerGas.Cmp(big.NewInt(2_000_000_000)) != 0 {
			t.Fatalf("max fee = %s, want fresh quote", quote.MaxFeePerGas)
		}
		if quote.MaxPriorityFeePerGas.Cmp(big.NewInt(1_000_000_000)) != 0 {
			t.Fatalf("priority fee = %s, want fresh quote", quote.MaxPriorityFeePerGas)
		}
	})
	t.Run("legacy", func(t *testing.T) {
		client := &fakeChainClient{
			header:            legacyHeader(),
			suggestedGasPrice: big.NewInt(5_000_000_000),
		}
		quote, err := quoteFee(context.Background(), db.QueuedOutboxTx{ID: 1, Nonce: &nonce}, defaultFeePolicy(), client)
		if err != nil {
			t.Fatalf("quoteFee() error = %v", err)
		}
		if quote.MaxFeePerGas.Cmp(big.NewInt(5_000_000_000)) != 0 {
			t.Fatalf("gas price = %s, want fresh quote", quote.MaxFeePerGas)
		}
	})
}
