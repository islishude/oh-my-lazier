package feeaccounting

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
)

func TestProcessOncePricesReceiptCosts(t *testing.T) {
	tests := []struct {
		name    string
		sources map[uint32]pricing.ChainSources
		cost    *big.Int
		want    *big.Int
	}{
		{
			name: "same native asset",
			sources: map[uint32]pricing.ChainSources{
				40161: {NativeAssetID: "eth"},
				40449: {NativeAssetID: "eth"},
			},
			cost: big.NewInt(123),
			want: big.NewInt(123),
		},
		{
			name: "cross asset rounds up",
			sources: map[uint32]pricing.ChainSources{
				40161: {Primary: fixedPrice{usd: big.NewRat(3, 1)}},
				40449: {Primary: fixedPrice{usd: big.NewRat(2, 1)}},
			},
			cost: big.NewInt(10),
			want: big.NewInt(7),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{costs: []db.UnpricedWorkerReceiptCost{testCost(tt.cost)}}
			reconciler := testReconciler(t, store, tt.sources)

			processed, err := reconciler.ProcessOnce(t.Context())
			if err != nil {
				t.Fatalf("ProcessOnce() error = %v", err)
			}
			if processed != 1 {
				t.Fatalf("processed = %d, want 1", processed)
			}
			if len(store.priced) != 1 || store.priced[1].Cmp(tt.want) != 0 {
				t.Fatalf("priced = %v, want tx 1 => %s", store.priced, tt.want)
			}
		})
	}
}

func TestProcessOnceLeavesReceiptPendingOnPricingError(t *testing.T) {
	store := &fakeStore{costs: []db.UnpricedWorkerReceiptCost{testCost(big.NewInt(10))}}
	reconciler := testReconciler(t, store, map[uint32]pricing.ChainSources{
		40161: {},
		40449: {Primary: fixedPrice{usd: big.NewRat(2, 1)}},
	})

	processed, err := reconciler.ProcessOnce(t.Context())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if processed != 0 {
		t.Fatalf("processed = %d, want 0", processed)
	}
	if len(store.priced) != 0 {
		t.Fatalf("priced = %v, want none", store.priced)
	}
}

func testReconciler(t *testing.T, store *fakeStore, sources map[uint32]pricing.ChainSources) *Reconciler {
	t.Helper()
	reconciler, err := New(store, sources, Settings{
		PriceSelection: pricing.PriceSelectionPolicy{MaxDeviationBps: 500},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return reconciler
}

func testCost(cost *big.Int) db.UnpricedWorkerReceiptCost {
	return db.UnpricedWorkerReceiptCost{
		ID:            1,
		Role:          "executor",
		SrcEID:        40161,
		DstEID:        40449,
		ChainEID:      40449,
		Purpose:       "executor_lz_receive",
		GUID:          common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		GasCostDstWei: cost,
	}
}

type fakeStore struct {
	costs  []db.UnpricedWorkerReceiptCost
	priced map[int64]*big.Int
}

func (s *fakeStore) ListUnpricedWorkerReceiptCosts(context.Context, int) ([]db.UnpricedWorkerReceiptCost, error) {
	return append([]db.UnpricedWorkerReceiptCost{}, s.costs...), nil
}

func (s *fakeStore) MarkTxReceiptCostPriced(_ context.Context, id int64, gasCostSrcWei *big.Int) error {
	if s.priced == nil {
		s.priced = make(map[int64]*big.Int)
	}
	s.priced[id] = new(big.Int).Set(gasCostSrcWei)
	return nil
}

type fixedPrice struct {
	usd *big.Rat
}

func (p fixedPrice) PriceUSD(context.Context) (pricing.SourcePrice, error) {
	return pricing.SourcePrice{Source: "fixed", USD: p.usd}, nil
}
