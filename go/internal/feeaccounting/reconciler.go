package feeaccounting

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"math/big"
	"time"

	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
)

const (
	defaultInterval = time.Minute
	defaultLimit    = 100
)

// Store persists source-token receipt gas-cost reconciliation results.
type Store interface {
	ListUnpricedWorkerReceiptCosts(ctx context.Context, limit int) ([]db.UnpricedWorkerReceiptCost, error)
	MarkTxReceiptCostPriced(ctx context.Context, id int64, gasCostSrcWei *big.Int) error
}

// Settings controls fee-accounting reconciliation behavior.
type Settings struct {
	Interval       time.Duration
	Limit          int
	PriceSelection pricing.PriceSelectionPolicy
}

// Reconciler prices mined worker receipt gas costs into source-chain native wei.
type Reconciler struct {
	store    Store
	sources  map[uint32]pricing.ChainSources
	settings Settings
	logger   *slog.Logger
}

// New creates a fee-accounting reconciler.
func New(store Store, sources map[uint32]pricing.ChainSources, settings Settings, logger *slog.Logger) (*Reconciler, error) {
	if store == nil {
		return nil, errors.New("fee accounting store is required")
	}
	normalized := normalizeSettings(settings)
	copiedSources := make(map[uint32]pricing.ChainSources, len(sources))
	maps.Copy(copiedSources, sources)
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{store: store, sources: copiedSources, settings: normalized, logger: logger}, nil
}

func normalizeSettings(settings Settings) Settings {
	if settings.Interval <= 0 {
		settings.Interval = defaultInterval
	}
	if settings.Limit <= 0 {
		settings.Limit = defaultLimit
	}
	return settings
}

// Run starts the fee-accounting reconciliation loop until the context is canceled.
func (r *Reconciler) Run(ctx context.Context) error {
	if _, err := r.ProcessOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(r.settings.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := r.ProcessOnce(ctx); err != nil {
				return err
			}
		}
	}
}

// ProcessOnce prices one batch of mined worker receipt gas costs.
func (r *Reconciler) ProcessOnce(ctx context.Context) (int, error) {
	costs, err := r.store.ListUnpricedWorkerReceiptCosts(ctx, r.settings.Limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, cost := range costs {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		gasCostSrcWei, err := r.priceReceiptCost(ctx, cost)
		if err != nil {
			r.logger.Warn("deferred fee reconciliation", "tx_outbox_id", cost.ID, "role", cost.Role, "src_eid", cost.SrcEID, "dst_eid", cost.DstEID, "purpose", cost.Purpose, "guid", cost.GUID, "error", err.Error())
			continue
		}
		if err := r.store.MarkTxReceiptCostPriced(ctx, cost.ID, gasCostSrcWei); err != nil {
			return processed, err
		}
		r.logger.Info("priced worker receipt gas cost", "tx_outbox_id", cost.ID, "role", cost.Role, "src_eid", cost.SrcEID, "dst_eid", cost.DstEID, "purpose", cost.Purpose, "guid", cost.GUID, "gas_cost_dst_wei", cost.GasCostDstWei, "gas_cost_src_wei", gasCostSrcWei)
		processed++
	}
	return processed, nil
}

func (r *Reconciler) priceReceiptCost(ctx context.Context, cost db.UnpricedWorkerReceiptCost) (*big.Int, error) {
	if cost.GasCostDstWei == nil || cost.GasCostDstWei.Sign() < 0 {
		return nil, errors.New("receipt destination gas cost must be non-negative")
	}
	if cost.ChainEID != cost.DstEID {
		return nil, errors.New("receipt chain eid does not match packet destination eid")
	}
	srcPrice, dstPrice, err := pricing.PathwayNativePrices(ctx, r.sources, cost.SrcEID, cost.DstEID, r.settings.PriceSelection)
	if err != nil {
		return nil, err
	}
	return pricing.ConvertDstWeiToSrcWei(cost.GasCostDstWei, srcPrice, dstPrice)
}
