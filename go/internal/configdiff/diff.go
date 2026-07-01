package configdiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

// Change describes one semantic configuration difference.
type Change struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

// Diff returns a deterministic semantic diff between two validated worker configs.
func Diff(before, after config.Config) []Change {
	var changes []Change
	compare(&changes, "database_url", before.DatabaseURL, after.DatabaseURL)
	compare(&changes, "metrics", before.Metrics, after.Metrics)
	compare(&changes, "pricing", pricingGlobal(before.Pricing), pricingGlobal(after.Pricing))
	diffMaps(&changes, "pricing.chains", pricingChains(before.Pricing.Chains), pricingChains(after.Pricing.Chains))
	diffMaps(&changes, "chains", chains(before.Chains), chains(after.Chains))
	diffMaps(&changes, "pathways", pathways(before.Pathways), pathways(after.Pathways))
	return changes
}

// RenderText renders a human-readable diff for review logs.
func RenderText(changes []Change) string {
	if len(changes) == 0 {
		return "no config changes\n"
	}
	var output strings.Builder
	for _, change := range changes {
		fmt.Fprintf(&output, "%s\n", change.Path)
		fmt.Fprintf(&output, "  before: %s\n", jsonValue(change.Before))
		fmt.Fprintf(&output, "  after:  %s\n", jsonValue(change.After))
	}
	return output.String()
}

func diffMaps[T any](changes *[]Change, prefix string, before, after map[string]T) {
	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	for key := range after {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		beforeValue, beforeOK := before[key]
		afterValue, afterOK := after[key]
		path := fmt.Sprintf("%s[%s]", prefix, key)
		switch {
		case !beforeOK:
			*changes = append(*changes, Change{Path: path, After: afterValue})
		case !afterOK:
			*changes = append(*changes, Change{Path: path, Before: beforeValue})
		default:
			compare(changes, path, beforeValue, afterValue)
		}
	}
}

func compare(changes *[]Change, path string, before, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	*changes = append(*changes, Change{Path: path, Before: before, After: after})
}

func chains(items []config.ChainConfig) map[string]config.ChainConfig {
	result := make(map[string]config.ChainConfig, len(items))
	for _, item := range items {
		result[fmt.Sprintf("%d", item.EID)] = item
	}
	return result
}

func pathways(items []config.PathwayConfig) map[string]config.PathwayConfig {
	result := make(map[string]config.PathwayConfig, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%d:%d:%s:%s", item.SrcEID, item.DstEID, item.SrcOApp, item.DstOApp)
		result[key] = item
	}
	return result
}

func pricingChains(items []config.PricingChainConfig) map[string]config.PricingChainConfig {
	result := make(map[string]config.PricingChainConfig, len(items))
	for _, item := range items {
		result[fmt.Sprintf("%d", item.EID)] = item
	}
	return result
}

type pricingConfigGlobal struct {
	Enabled                 bool   `json:"enabled"`
	Signer                  string `json:"signer"`
	IntervalSeconds         uint64 `json:"interval_seconds"`
	BaseFeeWei              string `json:"base_fee_wei"`
	BufferBps               uint16 `json:"buffer_bps"`
	StaleAfterSeconds       uint64 `json:"stale_after_seconds"`
	MaxDeviationBps         uint64 `json:"max_deviation_bps"`
	GasSpikeBps             uint64 `json:"gas_spike_bps"`
	AllowUniswapFallback    bool   `json:"allow_uniswap_fallback"`
	TxGasLimit              uint64 `json:"tx_gas_limit"`
	MaxFeePerGasWei         string `json:"max_fee_per_gas_wei"`
	MaxPriorityFeePerGasWei string `json:"max_priority_fee_per_gas_wei"`
	PrimarySource           string `json:"primary_source"`
	BinanceBaseURL          string `json:"binance_base_url"`
	CoinMarketCapBaseURL    string `json:"coinmarketcap_base_url"`
	CoinMarketCapAPIKeyEnv  string `json:"coinmarketcap_api_key_env"`
	CoinGeckoBaseURL        string `json:"coingecko_base_url"`
}

func pricingGlobal(pricing config.PricingConfig) pricingConfigGlobal {
	return pricingConfigGlobal{
		Enabled:                 pricing.Enabled,
		Signer:                  pricing.Signer,
		IntervalSeconds:         pricing.IntervalSeconds,
		BaseFeeWei:              pricing.BaseFeeWei,
		BufferBps:               pricing.BufferBps,
		StaleAfterSeconds:       pricing.StaleAfterSeconds,
		MaxDeviationBps:         pricing.MaxDeviationBps,
		GasSpikeBps:             pricing.GasSpikeBps,
		AllowUniswapFallback:    pricing.AllowUniswapFallback,
		TxGasLimit:              pricing.TxGasLimit,
		MaxFeePerGasWei:         pricing.MaxFeePerGasWei,
		MaxPriorityFeePerGasWei: pricing.MaxPriorityFeePerGasWei,
		PrimarySource:           pricing.PrimarySource,
		BinanceBaseURL:          pricing.BinanceBaseURL,
		CoinMarketCapBaseURL:    pricing.CoinMarketCapBaseURL,
		CoinMarketCapAPIKeyEnv:  pricing.CoinMarketCapAPIKeyEnv,
		CoinGeckoBaseURL:        pricing.CoinGeckoBaseURL,
	}
}

func jsonValue(value any) string {
	if value == nil {
		return "null"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}
