package pricing

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/abiutil"
	"github.com/islishude/oh-my-lazier/go/internal/bigutil"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

const (
	// TxPurposeSetPriceSnapshot identifies OpenPriceFeed.setPriceSnapshot updates.
	TxPurposeSetPriceSnapshot = "pricing_set_price_snapshot"
)

var (
	//go:embed abis/price_snapshot.json
	priceSnapshotABIJSON string

	priceSnapshotABI = abiutil.MustParse(priceSnapshotABIJSON)
)

// Bot updates shared worker price snapshots.
type Bot struct {
	store         Store
	registry      *chain.Registry
	settings      Settings
	sources       map[uint32]ChainSources
	lastGasPrices map[string]*big.Int
	now           func() time.Time
	logger        *slog.Logger
}

// New creates a price bot.
func New(logger *slog.Logger) *Bot {
	return &Bot{logger: logger, now: time.Now}
}

// NewWithDependencies creates an enabled price bot with explicit sources.
func NewWithDependencies(store Store, registry *chain.Registry, settings Settings, sources map[uint32]ChainSources, logger *slog.Logger) (*Bot, error) {
	if !settings.Enabled {
		return New(logger), nil
	}
	if store == nil {
		return nil, errors.New("pricing store is required")
	}
	if registry == nil {
		return nil, errors.New("pricing registry is required")
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	copied := make(map[uint32]ChainSources, len(sources))
	maps.Copy(copied, sources)
	return &Bot{store: store, registry: registry, settings: settings, sources: copied, lastGasPrices: make(map[string]*big.Int), now: time.Now, logger: logger}, nil
}

// Run starts the price update loop until the context is canceled.
func (b *Bot) Run(ctx context.Context) error {
	if b == nil || !b.settings.Enabled {
		b.logger.Info("price bot disabled")
		<-ctx.Done()
		return ctx.Err()
	}
	b.logger.Info("price bot loop started")
	if err := b.EnqueueOnce(ctx); err != nil {
		return err
	}
	interval := time.NewTicker(b.settings.Interval)
	defer interval.Stop()
	gasCheckInterval := min(b.settings.Interval, 15*time.Second)
	gasCheck := time.NewTicker(gasCheckInterval)
	defer gasCheck.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-interval.C:
			if err := b.EnqueueOnce(ctx); err != nil {
				return err
			}
		case <-gasCheck.C:
			if err := b.EnqueueOnGasSpike(ctx); err != nil {
				return err
			}
		}
	}
}

// Store persists price update transactions.
type Store interface {
	EnqueueTx(ctx context.Context, request db.TxRequest) (int64, error)
}

// GasPriceReader reads a destination-chain gas price.
type GasPriceReader interface {
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
}

// PriceReader reads USD/native prices.
type PriceReader interface {
	PriceUSD(ctx context.Context) (SourcePrice, error)
}

// Settings controls price update generation.
type Settings struct {
	Enabled             bool
	SignerID            string
	Interval            time.Duration
	StaleAfter          time.Duration
	MaxDeviation        uint64
	GasSpikeBps         uint64
	AllowSanityFallback bool
}

// FeeModel controls one worker role's source-chain quote inputs.
type FeeModel struct {
	FixedFee              *big.Int
	DstGasOverhead        uint64
	DataSizeOverheadBytes uint64
	MarginBps             uint16
}

// Validate checks settings required for enabled price updates.
func (s Settings) Validate() error {
	if !s.Enabled {
		return nil
	}
	if s.SignerID == "" {
		return errors.New("pricing signer id is required")
	}
	if s.Interval <= 0 {
		return errors.New("pricing interval must be positive")
	}
	if s.StaleAfter <= 0 {
		return errors.New("pricing stale_after must be positive")
	}
	if s.MaxDeviation == 0 {
		return errors.New("pricing max deviation bps is required")
	}
	if s.GasSpikeBps == 0 {
		return errors.New("pricing gas spike bps is required")
	}
	return nil
}

// Validate checks one worker fee model.
func (m FeeModel) Validate(prefix string) error {
	if m.FixedFee == nil || m.FixedFee.Sign() < 0 {
		return fmt.Errorf("%s fixed fee must be non-negative", prefix)
	}
	if m.MarginBps > 10_000 {
		return fmt.Errorf("%s margin bps exceeds 10000", prefix)
	}
	return nil
}

// ChainSources are the price and gas inputs for one configured chain.
type ChainSources struct {
	Primary           PriceReader
	Sanity            []PriceReader
	Gas               GasPriceReader
	DataFeePerByteWei *big.Int
	NativeAssetID     string
}

// EnqueueOnce computes current price snapshots and enqueues shared price-feed update batches.
func (b *Bot) EnqueueOnce(ctx context.Context) error {
	if b == nil || !b.settings.Enabled {
		return nil
	}
	if b.now == nil {
		b.now = time.Now
	}
	updates, err := b.uniquePriceUpdates()
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		b.logger.Debug("skipped price update batch", "reason", "no_pathways")
		return nil
	}
	for _, batch := range priceUpdateBatches(updates) {
		if _, err := b.enqueuePriceUpdateBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

// EnqueueOnGasSpike enqueues updates for pathways whose destination gas price rose past the configured threshold.
func (b *Bot) EnqueueOnGasSpike(ctx context.Context) error {
	if b == nil || !b.settings.Enabled {
		return nil
	}
	if b.lastGasPrices == nil {
		b.lastGasPrices = make(map[string]*big.Int)
	}
	updates, err := b.uniquePriceUpdates()
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		b.logger.Debug("skipped price gas-spike check", "reason", "no_pathways")
		return nil
	}
	spikes := make([]pricedGasSpike, 0, len(updates))
	for _, update := range updates {
		key := priceUpdateKey(update)
		current, err := b.currentDstGasPrice(ctx, update.DstEID)
		if err != nil {
			return err
		}
		previous := b.lastGasPrices[key]
		if previous == nil {
			b.lastGasPrices[key] = bigutil.Clone(current)
			continue
		}
		if GasIncreaseBps(previous, current) < b.settings.GasSpikeBps {
			continue
		}
		spikes = append(spikes, pricedGasSpike{
			update:   update,
			previous: bigutil.Clone(previous),
			current:  bigutil.Clone(current),
		})
	}
	for _, batch := range priceUpdateBatches(spikeUpdates(spikes)) {
		txOutboxID, err := b.enqueuePriceUpdateBatch(ctx, batch)
		if err != nil {
			return err
		}
		for _, selected := range spikes {
			if selected.update.SrcEID != batch.SrcEID || selected.update.PriceFeed != batch.PriceFeed {
				continue
			}
			b.logger.Warn("price bot enqueued gas-spike update", "src_eid", selected.update.SrcEID, "dst_eid", selected.update.DstEID, "price_feed", selected.update.PriceFeed, "previous_gas_wei", selected.previous, "current_gas_wei", selected.current, "tx_outbox_id", txOutboxID)
		}
	}
	return nil
}

type pricedUpdate struct {
	SrcEID    uint32
	DstEID    uint32
	PriceFeed common.Address
}

type pricedGasSpike struct {
	update   pricedUpdate
	previous *big.Int
	current  *big.Int
}

type pricedUpdateBatch struct {
	SrcEID    uint32
	PriceFeed common.Address
	Targets   []pricedUpdate
}

func (b *Bot) enqueuePriceUpdateBatch(ctx context.Context, batch pricedUpdateBatch) (int64, error) {
	srcChain, err := b.registry.Get(batch.SrcEID)
	if err != nil {
		return 0, err
	}
	updates := make([]PriceSnapshotUpdate, 0, len(batch.Targets))
	gasByKey := make(map[string]*big.Int, len(batch.Targets))
	dstEIDs := make([]uint32, 0, len(batch.Targets))
	for _, target := range batch.Targets {
		dstChain, err := b.registry.Get(target.DstEID)
		if err != nil {
			return 0, err
		}
		srcPrice, dstPrice, err := b.pathwayPrices(ctx, target.SrcEID, target.DstEID)
		if err != nil {
			return 0, err
		}
		dstGasPrice, err := b.currentDstGasPrice(ctx, target.DstEID)
		if err != nil {
			return 0, err
		}
		dstDataFeePerByte, err := b.currentDstDataFeePerByte(target.DstEID)
		if err != nil {
			return 0, err
		}
		snapshot, err := BuildPriceSnapshot(PriceInputs{
			SrcNativeUSD:         srcPrice,
			DstNativeUSD:         dstPrice,
			DstGasPriceWei:       dstGasPrice,
			DstDataFeePerByteWei: dstDataFeePerByte,
			UpdatedAtUnix:        uint64(b.now().Unix()),
			StaleAfterSeconds:    uint64(b.settings.StaleAfter.Seconds()),
		})
		if err != nil {
			return 0, err
		}
		updates = append(updates, PriceSnapshotUpdate{DstEid: dstChain.EID, Snapshot: snapshot})
		gasByKey[priceUpdateKey(target)] = bigutil.Clone(dstGasPrice)
		dstEIDs = append(dstEIDs, dstChain.EID)
	}
	tx, err := BuildSetPriceSnapshotTx(srcChain.EID, batch.PriceFeed, b.settings.SignerID, updates)
	if err != nil {
		return 0, err
	}
	id, err := b.store.EnqueueTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if b.lastGasPrices == nil {
		b.lastGasPrices = make(map[string]*big.Int)
	}
	for key, gas := range gasByKey {
		b.lastGasPrices[key] = bigutil.Clone(gas)
	}
	b.logger.Info("price update tx enqueued", "tx_outbox_id", id, "purpose", TxPurposeSetPriceSnapshot, "src_eid", srcChain.EID, "dst_count", len(dstEIDs), "dst_eids", dstEIDs, "price_feed", batch.PriceFeed)
	return id, nil
}

func (b *Bot) chainPrice(ctx context.Context, eid uint32) (*big.Rat, error) {
	sources, ok := b.sources[eid]
	if !ok {
		return nil, fmt.Errorf("pricing sources for chain %d are not configured", eid)
	}
	var primary SourcePrice
	if sources.Primary != nil {
		price, err := sources.Primary.PriceUSD(ctx)
		if err == nil {
			primary = price
		}
	}
	sanityPrices := make([]SourcePrice, 0, len(sources.Sanity))
	for _, reader := range sources.Sanity {
		if reader == nil {
			continue
		}
		price, err := reader.PriceUSD(ctx)
		if err == nil {
			sanityPrices = append(sanityPrices, price)
		}
	}
	return SelectPriceWithSanity(primary, sanityPrices, b.settings.MaxDeviation, b.settings.AllowSanityFallback)
}

func (b *Bot) pathwayPrices(ctx context.Context, srcEID, dstEID uint32) (*big.Rat, *big.Rat, error) {
	srcSources, ok := b.sources[srcEID]
	if !ok {
		return nil, nil, fmt.Errorf("pricing sources for chain %d are not configured", srcEID)
	}
	dstSources, ok := b.sources[dstEID]
	if !ok {
		return nil, nil, fmt.Errorf("pricing sources for chain %d are not configured", dstEID)
	}
	if srcSources.NativeAssetID != "" && srcSources.NativeAssetID == dstSources.NativeAssetID {
		return big.NewRat(1, 1), big.NewRat(1, 1), nil
	}
	srcPrice, err := b.chainPrice(ctx, srcEID)
	if err != nil {
		return nil, nil, err
	}
	dstPrice, err := b.chainPrice(ctx, dstEID)
	if err != nil {
		return nil, nil, err
	}
	return srcPrice, dstPrice, nil
}

func (b *Bot) currentDstGasPrice(ctx context.Context, dstEID uint32) (*big.Int, error) {
	dstSources, ok := b.sources[dstEID]
	if !ok || dstSources.Gas == nil {
		return nil, fmt.Errorf("pricing gas source for chain %d is not configured", dstEID)
	}
	dstGasPrice, err := dstSources.Gas.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	if dstGasPrice == nil || dstGasPrice.Sign() <= 0 {
		return nil, fmt.Errorf("pricing gas source for chain %d returned non-positive gas price", dstEID)
	}
	return dstGasPrice, nil
}

func (b *Bot) currentDstDataFeePerByte(dstEID uint32) (*big.Int, error) {
	dstSources, ok := b.sources[dstEID]
	if !ok || dstSources.DataFeePerByteWei == nil {
		return nil, fmt.Errorf("pricing data fee source for chain %d is not configured", dstEID)
	}
	if dstSources.DataFeePerByteWei.Sign() < 0 {
		return nil, fmt.Errorf("pricing data fee source for chain %d returned negative data fee", dstEID)
	}
	return bigutil.Clone(dstSources.DataFeePerByteWei), nil
}

func (b *Bot) uniquePriceUpdates() ([]pricedUpdate, error) {
	if b == nil || b.registry == nil {
		return nil, nil
	}
	seen := make(map[string]pricedUpdate)
	seenFeeModels := make(map[string]FeeModel)
	for _, pathway := range b.registry.Pathways() {
		executorFee, err := feeModelFromConfig(pathway.Pricing.ExecutorFee)
		if err != nil {
			return nil, fmt.Errorf("pathway %d -> %d pricing.executor_fee: %w", pathway.SrcEID, pathway.DstEID, err)
		}
		dvnFee, err := feeModelFromConfig(pathway.Pricing.DVNFee)
		if err != nil {
			return nil, fmt.Errorf("pathway %d -> %d pricing.dvn_fee: %w", pathway.SrcEID, pathway.DstEID, err)
		}
		if err := rememberFeeModel(seenFeeModels, pathway.SrcEID, pathway.DstEID, "executor", pathway.SourceWorkers.OpenExecutor, executorFee); err != nil {
			return nil, err
		}
		if err := rememberFeeModel(seenFeeModels, pathway.SrcEID, pathway.DstEID, "dvn", pathway.SourceWorkers.OpenDVN, dvnFee); err != nil {
			return nil, err
		}
		update := pricedUpdate{SrcEID: pathway.SrcEID, DstEID: pathway.DstEID, PriceFeed: pathway.SourceWorkers.PriceFeed}
		key := priceUpdateKey(update)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = update
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	updates := make([]pricedUpdate, 0, len(keys))
	for _, key := range keys {
		updates = append(updates, seen[key])
	}
	return updates, nil
}

func priceUpdateKey(update pricedUpdate) string {
	return fmt.Sprintf("%d:%d:%s", update.SrcEID, update.DstEID, update.PriceFeed)
}

func priceUpdateBatchKey(update pricedUpdate) string {
	return fmt.Sprintf("%d:%s", update.SrcEID, update.PriceFeed)
}

func priceUpdateBatches(updates []pricedUpdate) []pricedUpdateBatch {
	seen := make(map[string]pricedUpdateBatch)
	for _, update := range updates {
		key := priceUpdateBatchKey(update)
		batch := seen[key]
		if batch.Targets == nil {
			batch.SrcEID = update.SrcEID
			batch.PriceFeed = update.PriceFeed
		}
		batch.Targets = append(batch.Targets, update)
		seen[key] = batch
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	batches := make([]pricedUpdateBatch, 0, len(keys))
	for _, key := range keys {
		batch := seen[key]
		sort.Slice(batch.Targets, func(i, j int) bool {
			if batch.Targets[i].DstEID != batch.Targets[j].DstEID {
				return batch.Targets[i].DstEID < batch.Targets[j].DstEID
			}
			return priceUpdateKey(batch.Targets[i]) < priceUpdateKey(batch.Targets[j])
		})
		batches = append(batches, batch)
	}
	return batches
}

func spikeUpdates(spikes []pricedGasSpike) []pricedUpdate {
	updates := make([]pricedUpdate, 0, len(spikes))
	for _, spike := range spikes {
		updates = append(updates, spike.update)
	}
	return updates
}

func rememberFeeModel(seen map[string]FeeModel, srcEID, dstEID uint32, role string, worker common.Address, fee FeeModel) error {
	key := fmt.Sprintf("%d:%d:%s:%s", srcEID, dstEID, role, worker)
	if existing, ok := seen[key]; ok {
		if !feeModelsEqual(existing, fee) {
			return fmt.Errorf("conflicting %s fee model for %d -> %d worker %s", role, srcEID, dstEID, worker)
		}
		return nil
	}
	seen[key] = fee
	return nil
}

func feeModelFromConfig(cfg config.WorkerFeeModelConfig) (FeeModel, error) {
	if cfg.FixedFeeWei == "" {
		return FeeModel{}, errors.New("fixed_fee_wei is required")
	}
	fixedFee, err := bigutil.ParseNonNegativeDecimal("fixed_fee_wei", cfg.FixedFeeWei)
	if err != nil {
		return FeeModel{}, errors.New("fixed_fee_wei must be a non-negative integer")
	}
	if cfg.DataSizeOverheadBytes == nil {
		return FeeModel{}, errors.New("data_size_overhead_bytes is required")
	}
	model := FeeModel{
		FixedFee:              fixedFee,
		DstGasOverhead:        cfg.DstGasOverhead,
		DataSizeOverheadBytes: *cfg.DataSizeOverheadBytes,
		MarginBps:             cfg.MarginBps,
	}
	if err := model.Validate("fee model"); err != nil {
		return FeeModel{}, err
	}
	return model, nil
}

func feeModelsEqual(left, right FeeModel) bool {
	if left.FixedFee == nil || right.FixedFee == nil {
		return left.FixedFee == right.FixedFee &&
			left.DstGasOverhead == right.DstGasOverhead &&
			left.DataSizeOverheadBytes == right.DataSizeOverheadBytes &&
			left.MarginBps == right.MarginBps
	}
	return left.FixedFee.Cmp(right.FixedFee) == 0 &&
		left.DstGasOverhead == right.DstGasOverhead &&
		left.DataSizeOverheadBytes == right.DataSizeOverheadBytes &&
		left.MarginBps == right.MarginBps
}

// SourcePrice is one USD/native price observed from a configured data source.
type SourcePrice struct {
	Source string
	USD    *big.Rat
}

// SelectPrice applies the phase-1 primary/sanity policy for one sanity source.
func SelectPrice(primary, sanity SourcePrice, maxDeviationBps uint64, allowFallback bool) (*big.Rat, error) {
	return SelectPriceWithSanity(primary, []SourcePrice{sanity}, maxDeviationBps, allowFallback)
}

// SelectPriceWithSanity applies the phase-1 primary/sanity policy.
func SelectPriceWithSanity(primary SourcePrice, sanityPrices []SourcePrice, maxDeviationBps uint64, allowFallback bool) (*big.Rat, error) {
	if primary.USD != nil && primary.USD.Sign() > 0 {
		for _, sanity := range sanityPrices {
			if sanity.USD == nil || sanity.USD.Sign() <= 0 {
				continue
			}
			deviation := DeviationBps(primary.USD, sanity.USD)
			if deviation > maxDeviationBps {
				return nil, fmt.Errorf("price deviation between %s and %s is %d bps, exceeds limit %d bps", primary.Source, sanity.Source, deviation, maxDeviationBps)
			}
		}
		return bigutil.CloneRat(primary.USD), nil
	}
	if allowFallback {
		for _, sanity := range sanityPrices {
			if sanity.USD != nil && sanity.USD.Sign() > 0 {
				return bigutil.CloneRat(sanity.USD), nil
			}
		}
	}
	return nil, errors.New("no healthy price source")
}

// GasIncreaseBps returns max((current-previous)/previous, 0) in basis points.
func GasIncreaseBps(previous, current *big.Int) uint64 {
	if previous == nil || current == nil || previous.Sign() <= 0 || current.Sign() <= 0 {
		return ^uint64(0)
	}
	if current.Cmp(previous) <= 0 {
		return 0
	}
	diff := new(big.Int).Sub(current, previous)
	ratio := new(big.Rat).SetFrac(diff, previous)
	ratio.Mul(ratio, big.NewRat(10_000, 1))
	return bigutil.CeilRat(ratio).Uint64()
}

// DeviationBps returns abs(left-right)/left in basis points.
func DeviationBps(left, right *big.Rat) uint64 {
	if left == nil || right == nil || left.Sign() <= 0 || right.Sign() <= 0 {
		return ^uint64(0)
	}
	diff := new(big.Rat).Sub(left, right)
	if diff.Sign() < 0 {
		diff.Neg(diff)
	}
	diff.Mul(diff, big.NewRat(10_000, 1))
	diff.Quo(diff, left)
	return bigutil.CeilRat(diff).Uint64()
}

// PriceInputs are the source data used to construct WorkerTypes.PriceSnapshot.
type PriceInputs struct {
	SrcNativeUSD         *big.Rat
	DstNativeUSD         *big.Rat
	DstGasPriceWei       *big.Int
	DstDataFeePerByteWei *big.Int
	UpdatedAtUnix        uint64
	StaleAfterSeconds    uint64
}

// PriceSnapshot mirrors WorkerTypes.PriceSnapshot for ABI encoding.
type PriceSnapshot struct {
	DstGasPriceInSrcToken       *big.Int `abi:"dstGasPriceInSrcToken"`
	DstDataFeePerByteInSrcToken *big.Int `abi:"dstDataFeePerByteInSrcToken"`
	UpdatedAt                   uint64   `abi:"updatedAt"`
	StaleAfter                  uint64   `abi:"staleAfter"`
}

// PriceSnapshotUpdate mirrors WorkerTypes.PriceSnapshotUpdate for ABI encoding.
type PriceSnapshotUpdate struct {
	DstEid   uint32        `abi:"dstEid"`
	Snapshot PriceSnapshot `abi:"snapshot"`
}

// BuildPriceSnapshot converts destination gas cost into source native-token units.
func BuildPriceSnapshot(inputs PriceInputs) (PriceSnapshot, error) {
	if inputs.SrcNativeUSD == nil || inputs.SrcNativeUSD.Sign() <= 0 {
		return PriceSnapshot{}, errors.New("source native USD price must be positive")
	}
	if inputs.DstNativeUSD == nil || inputs.DstNativeUSD.Sign() <= 0 {
		return PriceSnapshot{}, errors.New("destination native USD price must be positive")
	}
	if inputs.DstGasPriceWei == nil || inputs.DstGasPriceWei.Sign() <= 0 {
		return PriceSnapshot{}, errors.New("destination gas price must be positive")
	}
	if inputs.DstDataFeePerByteWei == nil || inputs.DstDataFeePerByteWei.Sign() < 0 {
		return PriceSnapshot{}, errors.New("destination data fee per byte must be non-negative")
	}
	if inputs.UpdatedAtUnix == 0 {
		return PriceSnapshot{}, errors.New("updated_at is required")
	}
	if inputs.StaleAfterSeconds == 0 {
		return PriceSnapshot{}, errors.New("stale_after is required")
	}
	dstGasPriceInSrcToken := new(big.Rat).SetInt(inputs.DstGasPriceWei)
	dstGasPriceInSrcToken.Mul(dstGasPriceInSrcToken, inputs.DstNativeUSD)
	dstGasPriceInSrcToken.Quo(dstGasPriceInSrcToken, inputs.SrcNativeUSD)
	dstDataFeePerByteInSrcToken := new(big.Rat).SetInt(inputs.DstDataFeePerByteWei)
	dstDataFeePerByteInSrcToken.Mul(dstDataFeePerByteInSrcToken, inputs.DstNativeUSD)
	dstDataFeePerByteInSrcToken.Quo(dstDataFeePerByteInSrcToken, inputs.SrcNativeUSD)
	return PriceSnapshot{
		DstGasPriceInSrcToken:       bigutil.CeilRat(dstGasPriceInSrcToken),
		DstDataFeePerByteInSrcToken: bigutil.CeilRat(dstDataFeePerByteInSrcToken),
		UpdatedAt:                   inputs.UpdatedAtUnix,
		StaleAfter:                  inputs.StaleAfterSeconds,
	}, nil
}

// BuildSetPriceSnapshotCalldata ABI-encodes OpenPriceFeed setPriceSnapshot.
func BuildSetPriceSnapshotCalldata(updates []PriceSnapshotUpdate) ([]byte, error) {
	if len(updates) == 0 {
		return nil, errors.New("price snapshot updates are required")
	}
	for _, update := range updates {
		if update.DstEid == 0 {
			return nil, errors.New("destination eid is required")
		}
		if err := update.Snapshot.Validate(); err != nil {
			return nil, err
		}
	}
	return priceSnapshotABI.Pack("setPriceSnapshot", updates)
}

// BuildSetPriceSnapshotTx creates an outbox transaction for a shared price feed update.
func BuildSetPriceSnapshotTx(chainEID uint32, priceFeed common.Address, signerID string, updates []PriceSnapshotUpdate) (db.TxRequest, error) {
	if chainEID == 0 {
		return db.TxRequest{}, errors.New("chain eid is required")
	}
	if priceFeed == (common.Address{}) {
		return db.TxRequest{}, errors.New("price feed address is required")
	}
	if signerID == "" {
		return db.TxRequest{}, errors.New("signer id is required")
	}
	calldata, err := BuildSetPriceSnapshotCalldata(updates)
	if err != nil {
		return db.TxRequest{}, err
	}
	return db.TxRequest{
		ChainEID: chainEID,
		Purpose:  TxPurposeSetPriceSnapshot,
		To:       priceFeed,
		Calldata: calldata,
		Value:    new(big.Int),
		SignerID: signerID,
	}, nil
}

// Validate checks the on-chain price snapshot invariants the worker can know before sending.
func (s PriceSnapshot) Validate() error {
	if s.DstGasPriceInSrcToken == nil || s.DstGasPriceInSrcToken.Sign() <= 0 {
		return errors.New("price snapshot destination gas price must be positive")
	}
	if s.DstDataFeePerByteInSrcToken == nil || s.DstDataFeePerByteInSrcToken.Sign() < 0 {
		return errors.New("price snapshot destination data fee per byte must be non-negative")
	}
	if s.UpdatedAt == 0 {
		return errors.New("price snapshot updated_at is required")
	}
	if s.StaleAfter == 0 {
		return errors.New("price snapshot stale_after is required")
	}
	return nil
}
