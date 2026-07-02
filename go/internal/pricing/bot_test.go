package pricing

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

func TestBotEnqueueOnceQueuesExecutorAndDVNPriceUpdates(t *testing.T) {
	registry := testRegistry(t)
	store := &fakeStore{}
	bot, err := NewWithDependencies(store, registry, testSettings(), map[uint32]ChainSources{
		40161: {Primary: fixedPrice{price: big.NewRat(2000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(2000, 1)}}, Gas: fixedGas{price: big.NewInt(1_000_000_000)}},
		40245: {Primary: fixedPrice{price: big.NewRat(1000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(1000, 1)}}, Gas: fixedGas{price: big.NewInt(2_000_000_000)}},
	}, discardLogger())
	if err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	bot.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if err := bot.EnqueueOnce(context.Background()); err != nil {
		t.Fatalf("EnqueueOnce() error = %v", err)
	}
	if len(store.requests) != 4 {
		t.Fatalf("enqueued requests = %d, want 4", len(store.requests))
	}
	wantPurposes := map[string]int{
		TxPurposeSetExecutorPriceConfig: 2,
		TxPurposeSetDVNPriceConfig:      2,
	}
	for _, request := range store.requests {
		wantPurposes[request.Purpose]--
		if len(request.Calldata) == 0 {
			t.Fatal("request calldata is empty")
		}
		if request.SignerID != "0x9999999999999999999999999999999999999999" {
			t.Fatalf("signer = %q", request.SignerID)
		}
	}
	for purpose, remaining := range wantPurposes {
		if remaining != 0 {
			t.Fatalf("purpose %s remaining count = %d", purpose, remaining)
		}
	}
}

func TestBotEnqueueOnceRejectsDeviationWithoutEnqueue(t *testing.T) {
	registry := testRegistry(t)
	store := &fakeStore{}
	bot, err := NewWithDependencies(store, registry, testSettings(), map[uint32]ChainSources{
		40161: {Primary: fixedPrice{price: big.NewRat(2000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(2300, 1)}}, Gas: fixedGas{price: big.NewInt(1_000_000_000)}},
		40245: {Primary: fixedPrice{price: big.NewRat(1000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(1000, 1)}}, Gas: fixedGas{price: big.NewInt(2_000_000_000)}},
	}, discardLogger())
	if err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}

	if err := bot.EnqueueOnce(context.Background()); err == nil {
		t.Fatal("EnqueueOnce() error = nil, want deviation error")
	}
	if len(store.requests) != 0 {
		t.Fatalf("enqueued requests = %d, want 0", len(store.requests))
	}
}

func TestBotEnqueueOnGasSpikeQueuesOnlyAboveThreshold(t *testing.T) {
	registry := testRegistry(t)
	store := &fakeStore{}
	sourceGas := &mutableGas{price: big.NewInt(1_000_000_000)}
	destinationGas := &mutableGas{price: big.NewInt(2_000_000_000)}
	bot, err := NewWithDependencies(store, registry, testSettings(), map[uint32]ChainSources{
		40161: {Primary: fixedPrice{price: big.NewRat(2000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(2000, 1)}}, Gas: sourceGas},
		40245: {Primary: fixedPrice{price: big.NewRat(1000, 1)}, Sanity: []PriceReader{fixedPrice{price: big.NewRat(1000, 1)}}, Gas: destinationGas},
	}, discardLogger())
	if err != nil {
		t.Fatalf("NewWithDependencies() error = %v", err)
	}
	bot.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if err := bot.EnqueueOnce(context.Background()); err != nil {
		t.Fatalf("EnqueueOnce() error = %v", err)
	}
	if len(store.requests) != 4 {
		t.Fatalf("initial enqueued requests = %d, want 4", len(store.requests))
	}

	destinationGas.price = big.NewInt(2_100_000_000)
	if err := bot.EnqueueOnGasSpike(context.Background()); err != nil {
		t.Fatalf("EnqueueOnGasSpike() below threshold error = %v", err)
	}
	if len(store.requests) != 4 {
		t.Fatalf("below-threshold enqueued requests = %d, want 4", len(store.requests))
	}

	destinationGas.price = big.NewInt(2_300_000_000)
	if err := bot.EnqueueOnGasSpike(context.Background()); err != nil {
		t.Fatalf("EnqueueOnGasSpike() above threshold error = %v", err)
	}
	if len(store.requests) != 6 {
		t.Fatalf("above-threshold enqueued requests = %d, want 6", len(store.requests))
	}
}

type fakeStore struct {
	requests []db.TxRequest
}

func (s *fakeStore) EnqueueTx(_ context.Context, request db.TxRequest) (int64, error) {
	s.requests = append(s.requests, request)
	return int64(len(s.requests)), nil
}

type fixedPrice struct {
	price *big.Rat
}

func (p fixedPrice) PriceUSD(context.Context) (SourcePrice, error) {
	return SourcePrice{Source: "fixed", USD: p.price}, nil
}

type fixedGas struct {
	price *big.Int
}

func (g fixedGas) SuggestGasPrice(context.Context) (*big.Int, error) {
	return new(big.Int).Set(g.price), nil
}

type mutableGas struct {
	price *big.Int
}

func (g *mutableGas) SuggestGasPrice(context.Context) (*big.Int, error) {
	return new(big.Int).Set(g.price), nil
}

func testSettings() Settings {
	return Settings{
		Enabled:       true,
		SignerID:      "0x9999999999999999999999999999999999999999",
		Interval:      time.Minute,
		BaseFee:       big.NewInt(1000),
		BufferBps:     100,
		StaleAfter:    30 * time.Minute,
		MaxDeviation:  500,
		GasSpikeBps:   1000,
		AllowFallback: true,
		TxFees: TxFees{
			GasLimit:             big.NewInt(100_000),
			MaxFeePerGas:         big.NewInt(2_000_000_000),
			MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		},
	}
}

func testRegistry(t *testing.T) *chain.Registry {
	t.Helper()
	registry, err := chain.NewRegistry([]config.ChainConfig{
		{
			EID:             40161,
			Name:            "ethereum-sepolia",
			Family:          config.ChainFamilyEVM,
			ChainID:         11155111,
			EndpointAddress: config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8545"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
			},
		},
		{
			EID:             40245,
			Name:            "base-sepolia",
			Family:          config.ChainFamilyEVM,
			ChainID:         84532,
			EndpointAddress: config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8546"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
			},
		},
	}, []config.PathwayConfig{
		{
			SrcEID:     40161,
			DstEID:     40245,
			SrcOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
			DstOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
			SendLib:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
			ReceiveLib: config.MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			SourceWorkers: config.WorkerContractsConfig{
				OpenExecutor: config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
				OpenDVN:      config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
			},
			DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
			Enabled:        true,
			MaxMessageSize: 10000,
		},
		{
			SrcEID:     40245,
			DstEID:     40161,
			SrcOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
			DstOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
			SendLib:    config.MustEVMAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			ReceiveLib: config.MustEVMAddress("0xcccccccccccccccccccccccccccccccccccccccc"),
			SourceWorkers: config.WorkerContractsConfig{
				OpenExecutor: config.MustEVMAddress("0x5555555555555555555555555555555555555555"),
				OpenDVN:      config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
			},
			DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
			Enabled:        true,
			MaxMessageSize: 10000,
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
