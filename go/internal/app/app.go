package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/bigutil"
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
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
)

var checkOnChainConfig = func(ctx context.Context, registry *chain.Registry) (configcheck.Report, error) {
	return configcheck.Check(ctx, registry)
}

var readTxHeader = func(ctx context.Context, client txmgr.ChainClient) (*gethtypes.Header, error) {
	return client.HeaderByNumber(ctx, nil)
}

const loopRestartDelay = 5 * time.Second

type loopRetryRecorder interface {
	RecordLoopRetry(name string)
}

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
	executorWorker := executor.New(store, registry, a.logger)
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
	errCh := make(chan error, 1)
	start := func(name string, run func(context.Context) error) {
		wg.Go(func() {
			if err := superviseLoop(ctx, name, loopRestartDelay, a.logger, runtimeMetrics, run); err != nil {
				select {
				case errCh <- fmt.Errorf("%s loop failed fatally: %w", name, err):
				default:
				}
			}
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

	select {
	case <-ctx.Done():
		wg.Wait()
		return nil
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
}

func superviseLoop(ctx context.Context, name string, restartDelay time.Duration, logger *slog.Logger, retryMetrics loopRetryRecorder, run func(context.Context) error) error {
	for {
		err := run(ctx)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			logger.Info("loop stopped", "name", name)
			return nil
		}
		if workerloop.IsFatal(err) {
			logger.Error("loop failed fatally", "name", name, "error", err)
			return err
		}
		if err != nil {
			logger.Error("loop failed; restarting", "name", name, "error", err)
			if retryMetrics != nil {
				retryMetrics.RecordLoopRetry(name)
			}
		} else {
			logger.Warn("loop stopped unexpectedly; restarting", "name", name)
		}
		if !waitLoopRestart(ctx, restartDelay) {
			logger.Info("loop stopped", "name", name)
			return nil
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
	settings := pricing.Settings{
		Enabled:       true,
		SignerID:      a.cfg.Pricing.Signer.Hex(),
		Interval:      time.Duration(a.cfg.Pricing.IntervalSeconds) * time.Second,
		BaseFee:       baseFee,
		BufferBps:     a.cfg.Pricing.BufferBps,
		StaleAfter:    time.Duration(a.cfg.Pricing.StaleAfterSeconds) * time.Second,
		MaxDeviation:  a.cfg.Pricing.MaxDeviationBps,
		GasSpikeBps:   a.cfg.Pricing.GasSpikeBps,
		AllowFallback: a.cfg.Pricing.AllowUniswapFallback,
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
			QuoterAddress:    cfg.Uniswap.QuoterAddress.Common(),
			TokenIn:          cfg.Uniswap.TokenIn.Common(),
			TokenOut:         cfg.Uniswap.TokenOut.Common(),
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
	settings := make(map[uint32]dvn.Settings)
	for _, pathway := range registry.Pathways() {
		if pathway.DVNMode != config.DVNModeActive {
			continue
		}
		if _, ok := settings[pathway.DstEID]; ok {
			continue
		}
		dstChain, err := registry.Get(pathway.DstEID)
		if err != nil {
			return nil, err
		}
		settings[pathway.DstEID] = dvn.Settings{
			SignerID: dstChain.TxRoles.DVN.SignerID,
		}
	}
	return dvn.NewWithSettings(store, registry, settings, a.logger), nil
}

func (a *App) txTargets(ctx context.Context, registry *chain.Registry) ([]txmgr.Target, error) {
	signers, err := a.loadSigners(ctx)
	if err != nil {
		return nil, err
	}
	type targetKey struct {
		chainEID uint32
		signerID string
	}
	required := make(map[targetKey]map[string]txmgr.FeePolicy)
	addPolicy := func(chainEID uint32, signerID, purpose string, policy txmgr.FeePolicy) {
		key := targetKey{chainEID: chainEID, signerID: signerID}
		if required[key] == nil {
			required[key] = make(map[string]txmgr.FeePolicy)
		}
		required[key][purpose] = policy
	}
	for _, configuredChain := range registry.All() {
		executorPolicy, err := feePolicy(configuredChain.TxRoles.Executor.MaxFeePerGasWei, configuredChain.TxRoles.Executor.MaxPriorityFeePerGasWei)
		if err != nil {
			return nil, fmt.Errorf("chain %s executor fee policy: %w", configuredChain.Name, err)
		}
		addPolicy(configuredChain.EID, configuredChain.TxRoles.Executor.SignerID, executor.TxPurposeCommitVerification, executorPolicy)
		addPolicy(configuredChain.EID, configuredChain.TxRoles.Executor.SignerID, executor.TxPurposeLzReceive, executorPolicy)
		if a.cfg.Pricing.Enabled {
			pricingPolicy, err := feePolicy(a.cfg.Pricing.MaxFeePerGasWei, a.cfg.Pricing.MaxPriorityFeePerGasWei)
			if err != nil {
				return nil, fmt.Errorf("pricing fee policy: %w", err)
			}
			addPolicy(configuredChain.EID, a.cfg.Pricing.Signer.Hex(), pricing.TxPurposeSetExecutorPriceConfig, pricingPolicy)
			addPolicy(configuredChain.EID, a.cfg.Pricing.Signer.Hex(), pricing.TxPurposeSetDVNPriceConfig, pricingPolicy)
		}
	}
	for _, pathway := range registry.Pathways() {
		if pathway.DVNMode != config.DVNModeActive {
			continue
		}
		dstChain, err := registry.Get(pathway.DstEID)
		if err != nil {
			return nil, err
		}
		dvnPolicy, err := feePolicy(dstChain.TxRoles.DVN.MaxFeePerGasWei, dstChain.TxRoles.DVN.MaxPriorityFeePerGasWei)
		if err != nil {
			return nil, fmt.Errorf("chain %s dvn fee policy: %w", dstChain.Name, err)
		}
		addPolicy(dstChain.EID, dstChain.TxRoles.DVN.SignerID, dvn.TxPurposeVerify, dvnPolicy)
	}
	targets := make([]txmgr.Target, 0, len(required))
	for key, policies := range required {
		configuredChain, err := registry.Get(key.chainEID)
		if err != nil {
			return nil, err
		}
		if err := validateRuntimeFeePolicies(ctx, configuredChain, policies); err != nil {
			return nil, err
		}
		configuredSigner, ok := signers[key.signerID]
		if !ok {
			return nil, errors.New("configured signer was not loaded")
		}
		targets = append(targets, txmgr.Target{
			ChainEID:    configuredChain.EID,
			ChainID:     new(big.Int).Set(configuredChain.ChainID),
			Signer:      configuredSigner,
			Client:      configuredChain.RPC,
			FeePolicies: cloneFeePolicies(policies),
		})
	}
	return targets, nil
}

func (a *App) loadSigners(ctx context.Context) (map[string]signer.Signer, error) {
	signers := make(map[string]signer.Signer, len(a.cfg.Signers))
	for _, cfg := range a.cfg.Signers {
		id := cfg.ID.Hex()
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
			awsConfig, err := loadKMSAWSConfig(ctx, cfg.KMS)
			if err != nil {
				return nil, err
			}
			client := kms.NewSDKClient(awsConfig)
			if cfg.KMS.Endpoint != "" {
				client = kms.NewSDKClientWithEndpoint(awsConfig, cfg.KMS.Endpoint)
			}
			loaded := kms.New(client, cfg.KMS.KeyID, cfg.KMS.Address.Common())
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

func loadKMSAWSConfig(ctx context.Context, cfg config.KMSSignerConfig) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
}

func parseBigInt(value string) (*big.Int, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, errors.New("invalid integer")
	}
	return parsed, nil
}

func feePolicy(maxFeePerGasWei, maxPriorityFeePerGasWei string) (txmgr.FeePolicy, error) {
	maxFeePerGas, err := parseBigInt(maxFeePerGasWei)
	if err != nil {
		return txmgr.FeePolicy{}, err
	}
	var maxPriorityFeePerGas *big.Int
	if maxPriorityFeePerGasWei != "" {
		maxPriorityFeePerGas, err = parseBigInt(maxPriorityFeePerGasWei)
		if err != nil {
			return txmgr.FeePolicy{}, err
		}
	}
	return txmgr.FeePolicy{MaxFeePerGas: maxFeePerGas, MaxPriorityFeePerGas: maxPriorityFeePerGas}, nil
}

func validateRuntimeFeePolicies(ctx context.Context, configuredChain chain.Chain, policies map[string]txmgr.FeePolicy) error {
	header, err := readTxHeader(ctx, configuredChain.RPC)
	if err != nil {
		return fmt.Errorf("read latest header for chain %s: %w", configuredChain.Name, err)
	}
	if header == nil || header.BaseFee == nil {
		return nil
	}
	for purpose, policy := range policies {
		if policy.MaxPriorityFeePerGas == nil || policy.MaxPriorityFeePerGas.Sign() <= 0 {
			return fmt.Errorf("chain %s purpose %s max_priority_fee_per_gas_wei is required because latest header has base fee", configuredChain.Name, purpose)
		}
	}
	return nil
}

func cloneFeePolicies(policies map[string]txmgr.FeePolicy) map[string]txmgr.FeePolicy {
	out := make(map[string]txmgr.FeePolicy, len(policies))
	for purpose, policy := range policies {
		out[purpose] = txmgr.FeePolicy{
			MaxFeePerGas:         bigutil.Clone(policy.MaxFeePerGas),
			MaxPriorityFeePerGas: bigutil.Clone(policy.MaxPriorityFeePerGas),
		}
	}
	return out
}
