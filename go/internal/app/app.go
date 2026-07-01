package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/configcheck"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/dvn"
	"github.com/islishude/oh-my-lazier/go/internal/executor"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/metrics"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
	"github.com/islishude/oh-my-lazier/go/internal/signer"
	"github.com/islishude/oh-my-lazier/go/internal/signer/keystore"
	"github.com/islishude/oh-my-lazier/go/internal/signer/kms"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

var checkOnChainConfig = func(ctx context.Context, registry *chain.Registry) (configcheck.Report, error) {
	return configcheck.Check(ctx, registry)
}

const loopRestartDelay = 5 * time.Second

// App owns the configured worker process and its durable service loops.
type App struct {
	cfg    config.Config
	logger *slog.Logger
}

// New builds an App from already-validated configuration.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		return nil, errors.New("app logger is required")
	}
	return &App{cfg: cfg, logger: logger}, nil
}

// Run connects dependencies and runs all worker loops until cancellation.
func (a *App) Run(ctx context.Context) error {
	registry, err := chain.NewRegistry(a.cfg.Chains, a.cfg.Pathways)
	if err != nil {
		return err
	}
	defer registry.Close()
	if report, err := checkOnChainConfig(ctx, registry); err != nil {
		return err
	} else if !report.OK {
		return fmt.Errorf("on-chain config check failed: %s", configcheck.RenderText(report))
	}

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

	if err := store.SyncConfig(ctx, registry); err != nil {
		return err
	}

	runtimeMetrics := metrics.NewRegistry()
	pathways := registry.Pathways()
	txTargets, err := a.txTargets(ctx, registry)
	if err != nil {
		return err
	}
	executorWorker := executor.New(store, registry, a.cfg.Executor.Signer, a.logger)
	dvnWorker, err := a.dvnWorker(store, registry)
	if err != nil {
		return err
	}
	priceBot, err := a.priceBot(store, registry)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	start := func(name string, run func(context.Context) error) {
		wg.Go(func() {
			superviseLoop(ctx, name, loopRestartDelay, a.logger, run)
		})
	}

	start("metrics", metrics.New(a.cfg.Metrics.ListenAddress, store, a.logger, runtimeMetrics).Run)
	for _, c := range registry.All() {
		start("indexer."+c.Name, indexer.New(c, pathways, store, a.logger).WithMetrics(runtimeMetrics).Run)
	}
	start("txmgr", txmgr.NewWithTargets(store, txTargets, a.logger).Run)
	start("executor.committer", executorWorker.RunCommitter)
	start("executor.deliverer", executorWorker.RunDeliverer)
	start("dvn", dvnWorker.Run)
	start("pricing", priceBot.Run)

	<-ctx.Done()
	wg.Wait()
	return nil
}

func superviseLoop(ctx context.Context, name string, restartDelay time.Duration, logger *slog.Logger, run func(context.Context) error) {
	for {
		err := run(ctx)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			logger.Info("loop stopped", "name", name)
			return
		}
		if err != nil {
			logger.Error("loop failed; restarting", "name", name, "error", err)
		} else {
			logger.Warn("loop stopped unexpectedly; restarting", "name", name)
		}
		if !waitLoopRestart(ctx, restartDelay) {
			logger.Info("loop stopped", "name", name)
			return
		}
	}
}

func waitLoopRestart(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// RunPriceOnce computes current price configs and enqueues one update batch.
func (a *App) RunPriceOnce(ctx context.Context) error {
	if !a.cfg.Pricing.Enabled {
		return errors.New("pricing is disabled")
	}
	registry, err := chain.NewRegistry(a.cfg.Chains, a.cfg.Pathways)
	if err != nil {
		return err
	}
	defer registry.Close()
	if report, err := checkOnChainConfig(ctx, registry); err != nil {
		return err
	} else if !report.OK {
		return fmt.Errorf("on-chain config check failed: %s", configcheck.RenderText(report))
	}

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

	if err := store.SyncConfig(ctx, registry); err != nil {
		return err
	}

	priceBot, err := a.priceBot(store, registry)
	if err != nil {
		return err
	}
	return priceBot.EnqueueOnce(ctx)
}

func (a *App) priceBot(store *db.Store, registry *chain.Registry) (*pricing.Bot, error) {
	if !a.cfg.Pricing.Enabled {
		return pricing.New(a.logger), nil
	}
	baseFee, err := parseBigInt(a.cfg.Pricing.BaseFeeWei)
	if err != nil {
		return nil, err
	}
	dynamicFeeCapsRequired := a.requiresDynamicFeeCaps()
	maxFeePerGas, err := parseOptionalBigInt(a.cfg.Pricing.MaxFeePerGasWei, dynamicFeeCapsRequired)
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := parseOptionalBigInt(a.cfg.Pricing.MaxPriorityFeePerGasWei, dynamicFeeCapsRequired)
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
		GasSpikeBps:   a.cfg.Pricing.GasSpikeBps,
		AllowFallback: a.cfg.Pricing.AllowUniswapFallback,
		TxFees: pricing.TxFees{
			GasLimit:             new(big.Int).SetUint64(a.cfg.Pricing.TxGasLimit),
			MaxFeePerGas:         maxFeePerGas,
			MaxPriorityFeePerGas: maxPriorityFeePerGas,
		},
	}
	binanceClient := pricing.NewBinanceClient(a.cfg.Pricing.BinanceBaseURL, http.DefaultClient)
	coinMarketCapClient, err := pricing.NewCoinMarketCapClient(a.cfg.Pricing.CoinMarketCapBaseURL, a.cfg.Pricing.CoinMarketCapAPIKeyEnv, http.DefaultClient)
	if err != nil {
		return nil, err
	}
	coinGeckoClient := pricing.NewCoinGeckoClient(a.cfg.Pricing.CoinGeckoBaseURL, http.DefaultClient)
	primarySource := a.cfg.Pricing.PrimarySource
	if primarySource == "" {
		primarySource = "binance"
	}
	sources := make(map[uint32]pricing.ChainSources, len(a.cfg.Pricing.Chains))
	for _, cfg := range a.cfg.Pricing.Chains {
		configuredChain, err := registry.Get(cfg.EID)
		if err != nil {
			return nil, err
		}
		readers := make(map[string]pricing.PriceReader)
		if cfg.BinanceSymbol != "" {
			reader, err := pricing.NewBinancePriceReader(binanceClient, cfg.BinanceSymbol)
			if err != nil {
				return nil, err
			}
			readers["binance"] = reader
		}
		if cfg.CoinMarketCapSymbol != "" {
			reader, err := pricing.NewCoinMarketCapPriceReader(coinMarketCapClient, cfg.CoinMarketCapSymbol)
			if err != nil {
				return nil, err
			}
			readers["coinmarketcap"] = reader
		}
		if cfg.CoinGeckoID != "" {
			reader, err := pricing.NewCoinGeckoPriceReader(coinGeckoClient, cfg.CoinGeckoID)
			if err != nil {
				return nil, err
			}
			readers["coingecko"] = reader
		}
		primary := readers[primarySource]
		if primary == nil {
			return nil, fmt.Errorf("pricing chain %d primary source %s is not configured", cfg.EID, primarySource)
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
		sanityReaders := []pricing.PriceReader{sanity}
		for source, reader := range readers {
			if source != primarySource {
				sanityReaders = append(sanityReaders, reader)
			}
		}
		sources[cfg.EID] = pricing.ChainSources{Primary: primary, Sanity: sanityReaders, Gas: configuredChain.RPC}
	}
	return pricing.NewWithDependencies(store, registry, settings, sources, a.logger)
}

func (a *App) dvnWorker(store *db.Store, registry *chain.Registry) (*dvn.Worker, error) {
	if a.cfg.DVN.Mode != string(dvn.ModeActive) {
		return dvn.New(a.cfg.DVN.Mode, store, registry, a.logger), nil
	}
	dynamicFeeCapsRequired := a.requiresDynamicFeeCaps()
	maxFeePerGas, err := parseOptionalBigInt(a.cfg.DVN.MaxFeePerGasWei, dynamicFeeCapsRequired)
	if err != nil {
		return nil, err
	}
	maxPriorityFeePerGas, err := parseOptionalBigInt(a.cfg.DVN.MaxPriorityFeePerGasWei, dynamicFeeCapsRequired)
	if err != nil {
		return nil, err
	}
	settings := dvn.Settings{
		SignerID: a.cfg.DVN.Signer,
		TxFees: dvn.TxFees{
			GasLimit:             new(big.Int).SetUint64(a.cfg.DVN.TxGasLimit),
			MaxFeePerGas:         maxFeePerGas,
			MaxPriorityFeePerGas: maxPriorityFeePerGas,
		},
	}
	return dvn.NewWithSettings(a.cfg.DVN.Mode, store, registry, settings, a.logger), nil
}

func (a *App) txTargets(ctx context.Context, registry *chain.Registry) ([]txmgr.Target, error) {
	signers, err := a.loadSigners(ctx)
	if err != nil {
		return nil, err
	}
	required := map[string]struct{}{common.HexToAddress(a.cfg.Executor.Signer).Hex(): {}}
	if a.cfg.DVN.Mode == string(dvn.ModeActive) {
		required[common.HexToAddress(a.cfg.DVN.Signer).Hex()] = struct{}{}
	}
	if a.cfg.Pricing.Enabled {
		required[common.HexToAddress(a.cfg.Pricing.Signer).Hex()] = struct{}{}
	}
	targets := make([]txmgr.Target, 0, len(required)*len(a.cfg.Chains))
	for _, configuredChain := range registry.All() {
		for signerID := range required {
			configuredSigner, ok := signers[signerID]
			if !ok {
				return nil, errors.New("configured signer was not loaded")
			}
			targets = append(targets, txmgr.Target{
				ChainEID: configuredChain.EID,
				ChainID:  new(big.Int).Set(configuredChain.ChainID),
				TxType:   configuredChain.TxType,
				Signer:   configuredSigner,
				Client:   configuredChain.RPC,
			})
		}
	}
	return targets, nil
}

func (a *App) loadSigners(ctx context.Context) (map[string]signer.Signer, error) {
	signers := make(map[string]signer.Signer, len(a.cfg.Signers))
	for _, cfg := range a.cfg.Signers {
		id := common.HexToAddress(cfg.ID).Hex()
		switch cfg.Type {
		case "keystore":
			loaded, err := keystore.LoadWithPasswordSource(cfg.Keystore.Path, keystore.PasswordSource{
				Env:  cfg.Keystore.PasswordEnv,
				File: cfg.Keystore.PasswordFile,
			})
			if err != nil {
				return nil, err
			}
			if loaded.Address().Hex() != id {
				return nil, errors.New("keystore signer address does not match configured signer id")
			}
			signers[id] = loaded
		case "kms":
			accessKeyID, err := envValue(cfg.KMS.AccessKeyIDEnv)
			if err != nil {
				return nil, err
			}
			secretAccessKey, err := envValue(cfg.KMS.SecretAccessKeyEnv)
			if err != nil {
				return nil, err
			}
			sessionToken := ""
			if cfg.KMS.SessionTokenEnv != "" {
				sessionToken, err = envValue(cfg.KMS.SessionTokenEnv)
				if err != nil {
					return nil, err
				}
			}
			awsConfig := aws.Config{
				Region:      cfg.KMS.Region,
				Credentials: credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken),
			}
			client := kms.NewSDKClient(awsConfig)
			if cfg.KMS.Endpoint != "" {
				client = kms.NewSDKClientWithEndpoint(awsConfig, cfg.KMS.Endpoint)
			}
			loaded := kms.New(client, cfg.KMS.KeyID, common.HexToAddress(cfg.KMS.Address))
			if err := loaded.ValidateKey(ctx); err != nil {
				return nil, err
			}
			signers[id] = loaded
		default:
			return nil, errors.New("unsupported signer type")
		}
	}
	return signers, nil
}

func (a *App) requiresDynamicFeeCaps() bool {
	for _, chain := range a.cfg.Chains {
		if config.NormalizeTxType(chain.TxType) == config.TxTypeDynamicFee {
			return true
		}
	}
	return false
}

func envValue(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is empty", name)
	}
	return value, nil
}

func parseBigInt(value string) (*big.Int, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, errors.New("invalid integer")
	}
	return parsed, nil
}

func parseOptionalBigInt(value string, required bool) (*big.Int, error) {
	if value == "" && !required {
		return nil, nil
	}
	return parseBigInt(value)
}
