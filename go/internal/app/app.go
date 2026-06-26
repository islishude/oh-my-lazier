package app

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/dvn"
	"github.com/islishude/oh-my-lazier/go/internal/executor"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/metrics"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

// App owns the configured worker process and its durable service loops.
type App struct {
	cfg    config.Config
	logger *slog.Logger
}

// New builds an App from already-validated configuration.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	return &App{cfg: cfg, logger: logger}, nil
}

// Run connects dependencies and runs all worker loops until cancellation or a loop failure.
func (a *App) Run(ctx context.Context) error {
	store, err := db.Connect(ctx, a.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		return err
	}
	if err := store.Migrate(ctx); err != nil {
		return err
	}

	registry, err := chain.NewRegistry(a.cfg.Chains, a.cfg.Pathways)
	if err != nil {
		return err
	}
	if err := store.SyncConfig(ctx, registry); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 8+len(a.cfg.Chains))
	start := func(name string, run func(context.Context) error) {
		wg.Go(func() {
			// Any durable loop failure cancels the whole worker; partial worker operation can
			// otherwise advance packet state with missing indexers or senders.
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
				cancel()
			}
			a.logger.Info("loop stopped", "name", name)
		})
	}

	start("metrics", metrics.New(a.cfg.Metrics.ListenAddress, store, a.logger).Run)
	pathways := registry.Pathways()
	for _, c := range registry.All() {
		start("indexer."+c.Name, indexer.New(c, pathways, store, a.logger).Run)
	}
	start("txmgr", txmgr.New(store, a.logger).Run)

	executorWorker := executor.New(store, registry, a.cfg.Executor.Signer, a.logger)
	start("executor.committer", executorWorker.RunCommitter)
	start("executor.deliverer", executorWorker.RunDeliverer)
	start("dvn", dvn.New(a.cfg.DVN.Mode, store, registry, a.logger).Run)
	priceBot, err := a.priceBot(store, registry)
	if err != nil {
		return err
	}
	start("pricing", priceBot.Run)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
	wg.Wait()
	return nil
}

func (a *App) priceBot(store *db.Store, registry *chain.Registry) (*pricing.Bot, error) {
	if !a.cfg.Pricing.Enabled {
		return pricing.New(a.logger), nil
	}
	baseFee, err := parseBigInt(a.cfg.Pricing.BaseFeeWei)
	if err != nil {
		return nil, err
	}
	maxFeePerGas, err := parseBigInt(a.cfg.Pricing.MaxFeePerGasWei)
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := parseBigInt(a.cfg.Pricing.MaxPriorityFeePerGasWei)
	if err != nil {
		return nil, err
	}
	settings := pricing.Settings{
		Enabled:       true,
		SignerID:      a.cfg.Pricing.Signer,
		Interval:      time.Duration(a.cfg.Pricing.IntervalSeconds) * time.Second,
		BaseFee:       baseFee,
		BufferBps:     a.cfg.Pricing.BufferBps,
		StaleAfter:    time.Duration(a.cfg.Pricing.StaleAfterSeconds) * time.Second,
		MaxDeviation:  a.cfg.Pricing.MaxDeviationBps,
		AllowFallback: a.cfg.Pricing.AllowUniswapFallback,
		TxFees: pricing.TxFees{
			GasLimit:             new(big.Int).SetUint64(a.cfg.Pricing.TxGasLimit),
			MaxFeePerGas:         maxFeePerGas,
			MaxPriorityFeePerGas: maxPriorityFeePerGas,
		},
	}
	binanceClient := pricing.NewBinanceClient(a.cfg.Pricing.BinanceBaseURL, http.DefaultClient)
	sources := make(map[uint32]pricing.ChainSources, len(a.cfg.Pricing.Chains))
	for _, cfg := range a.cfg.Pricing.Chains {
		configuredChain, err := registry.Get(cfg.EID)
		if err != nil {
			return nil, err
		}
		primary, err := pricing.NewBinancePriceReader(binanceClient, cfg.BinanceSymbol)
		if err != nil {
			return nil, err
		}
		amountIn, err := parseBigInt(cfg.Uniswap.AmountInWei)
		if err != nil {
			return nil, err
		}
		sanity, err := pricing.NewUniswapV3Client(configuredChain.RPC, pricing.UniswapV3Config{
			QuoterAddress:    common.HexToAddress(cfg.Uniswap.QuoterAddress),
			TokenIn:          common.HexToAddress(cfg.Uniswap.TokenIn),
			TokenOut:         common.HexToAddress(cfg.Uniswap.TokenOut),
			Fee:              cfg.Uniswap.Fee,
			AmountIn:         amountIn,
			TokenOutDecimals: cfg.Uniswap.TokenOutDecimals,
		})
		if err != nil {
			return nil, err
		}
		sources[cfg.EID] = pricing.ChainSources{Primary: primary, Sanity: sanity, Gas: configuredChain.RPC}
	}
	return pricing.NewWithDependencies(store, registry, settings, sources, a.logger)
}

func parseBigInt(value string) (*big.Int, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, errors.New("invalid integer")
	}
	return parsed, nil
}
