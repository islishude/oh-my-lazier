package config

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

const (
	// TxTypeDynamicFee selects EIP-1559 dynamic-fee transaction signing.
	TxTypeDynamicFee = "dynamic_fee"
	// TxTypeLegacy selects legacy gas-price transaction signing.
	TxTypeLegacy = "legacy"
)

// Config is the startup configuration for the single-process worker.
type Config struct {
	DatabaseURL string          `yaml:"database_url"`
	Metrics     MetricsConfig   `yaml:"metrics"`
	Executor    ExecutorConfig  `yaml:"executor"`
	DVN         DVNConfig       `yaml:"dvn"`
	Pricing     PricingConfig   `yaml:"pricing"`
	Signers     []SignerConfig  `yaml:"signers"`
	Chains      []ChainConfig   `yaml:"chains"`
	Pathways    []PathwayConfig `yaml:"pathways"`
}

// MetricsConfig controls the worker HTTP metrics and health endpoint.
type MetricsConfig struct {
	ListenAddress string `yaml:"listen_address"`
}

// ExecutorConfig controls executor transaction submission.
type ExecutorConfig struct {
	Signer string `yaml:"signer"`
}

// DVNConfig controls whether the DVN workflow runs in shadow or active mode.
type DVNConfig struct {
	Mode                    string `yaml:"mode"`
	Signer                  string `yaml:"signer"`
	TxGasLimit              uint64 `yaml:"tx_gas_limit"`
	MaxFeePerGasWei         string `yaml:"max_fee_per_gas_wei"`
	MaxPriorityFeePerGasWei string `yaml:"max_priority_fee_per_gas_wei"`
}

// PricingConfig controls optional price update generation.
type PricingConfig struct {
	Enabled                 bool                 `yaml:"enabled"`
	Signer                  string               `yaml:"signer"`
	IntervalSeconds         uint64               `yaml:"interval_seconds"`
	BaseFeeWei              string               `yaml:"base_fee_wei"`
	BufferBps               uint16               `yaml:"buffer_bps"`
	StaleAfterSeconds       uint64               `yaml:"stale_after_seconds"`
	MaxDeviationBps         uint64               `yaml:"max_deviation_bps"`
	GasSpikeBps             uint64               `yaml:"gas_spike_bps"`
	AllowUniswapFallback    bool                 `yaml:"allow_uniswap_fallback"`
	TxGasLimit              uint64               `yaml:"tx_gas_limit"`
	MaxFeePerGasWei         string               `yaml:"max_fee_per_gas_wei"`
	MaxPriorityFeePerGasWei string               `yaml:"max_priority_fee_per_gas_wei"`
	PrimarySource           string               `yaml:"primary_source"`
	BinanceBaseURL          string               `yaml:"binance_base_url"`
	CoinMarketCapBaseURL    string               `yaml:"coinmarketcap_base_url"`
	CoinMarketCapAPIKeyEnv  string               `yaml:"coinmarketcap_api_key_env"`
	CoinGeckoBaseURL        string               `yaml:"coingecko_base_url"`
	Chains                  []PricingChainConfig `yaml:"chains"`
}

// PricingChainConfig configures price sources for one chain's native asset.
type PricingChainConfig struct {
	EID                 uint32               `yaml:"eid"`
	BinanceSymbol       string               `yaml:"binance_symbol"`
	CoinMarketCapSymbol string               `yaml:"coinmarketcap_symbol"`
	CoinGeckoID         string               `yaml:"coingecko_id"`
	Uniswap             UniswapPricingConfig `yaml:"uniswap"`
}

// UniswapPricingConfig configures one V3 quoter sanity route.
type UniswapPricingConfig struct {
	QuoterAddress    string `yaml:"quoter_address"`
	TokenIn          string `yaml:"token_in"`
	TokenOut         string `yaml:"token_out"`
	Fee              uint32 `yaml:"fee"`
	AmountInWei      string `yaml:"amount_in_wei"`
	TokenOutDecimals uint8  `yaml:"token_out_decimals"`
}

// SignerConfig configures one local signing backend without embedding raw secret material.
type SignerConfig struct {
	ID       string               `yaml:"id"`
	Type     string               `yaml:"type"`
	Keystore KeystoreSignerConfig `yaml:"keystore"`
	KMS      KMSSignerConfig      `yaml:"kms"`
}

// KeystoreSignerConfig points at an encrypted geth keystore and its password source.
type KeystoreSignerConfig struct {
	Path         string `yaml:"path"`
	PasswordEnv  string `yaml:"password_env"`
	PasswordFile string `yaml:"password_file"`
}

// KMSSignerConfig points at an AWS-compatible KMS signing key.
type KMSSignerConfig struct {
	KeyID    string `yaml:"key_id"`
	Region   string `yaml:"region"`
	Address  string `yaml:"address"`
	Endpoint string `yaml:"endpoint"`
}

// ChainConfig defines one LayerZero endpoint chain watched by the worker.
type ChainConfig struct {
	EID                    uint32                `yaml:"eid"`
	Name                   string                `yaml:"name"`
	ChainID                int64                 `yaml:"chain_id"`
	TxType                 string                `yaml:"tx_type"`
	EndpointAddress        string                `yaml:"endpoint_address"`
	Confirmations          uint64                `yaml:"confirmations"`
	StartBlockNumber       uint64                `yaml:"start_block_number"`
	IndexerQueryBlockRange uint64                `yaml:"indexer_query_block_range"`
	RPCURLs                []string              `yaml:"rpc_urls"`
	Workers                WorkerContractsConfig `yaml:"workers"`
}

// WorkerContractsConfig identifies the self-hosted worker contracts deployed on one chain.
type WorkerContractsConfig struct {
	OpenExecutor string `yaml:"open_executor"`
	OpenDVN      string `yaml:"open_dvn"`
}

// PathwayConfig defines an allowed source-to-destination message pathway.
type PathwayConfig struct {
	SrcEID          uint32 `yaml:"src_eid"`
	DstEID          uint32 `yaml:"dst_eid"`
	SrcOApp         string `yaml:"src_oapp"`
	DstOApp         string `yaml:"dst_oapp"`
	SendLib         string `yaml:"send_lib"`
	ReceiveLib      string `yaml:"receive_lib"`
	Enabled         bool   `yaml:"enabled"`
	MaxMessageSize  uint64 `yaml:"max_message_size"`
	MinLzReceiveGas uint64 `yaml:"min_lz_receive_gas"`
	MaxLzReceiveGas uint64 `yaml:"max_lz_receive_gas"`
}

// Load reads a YAML config file, applies environment overrides, and validates the result.
func Load(path string) (Config, error) {
	return load(path, true)
}

// LoadStatic reads a YAML config file, applies defaults, and validates it without environment overrides.
func LoadStatic(path string) (Config, error) {
	return load(path, false)
}

func load(path string, applyEnv bool) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if env := os.Getenv("DATABASE_URL"); applyEnv && env != "" {
		// Compose and managed deployments inject credentials through the environment.
		cfg.DatabaseURL = env
	}
	if cfg.Metrics.ListenAddress == "" {
		cfg.Metrics.ListenAddress = ":9090"
	}
	if cfg.DVN.Mode == "" {
		cfg.DVN.Mode = "shadow"
	}
	if cfg.Pricing.Enabled {
		if cfg.Pricing.IntervalSeconds == 0 {
			cfg.Pricing.IntervalSeconds = 300
		}
		if cfg.Pricing.StaleAfterSeconds == 0 {
			cfg.Pricing.StaleAfterSeconds = 1800
		}
		if cfg.Pricing.MaxDeviationBps == 0 {
			cfg.Pricing.MaxDeviationBps = 500
		}
		if cfg.Pricing.GasSpikeBps == 0 {
			cfg.Pricing.GasSpikeBps = 1_000
		}
	}
	for idx := range cfg.Chains {
		if cfg.Chains[idx].IndexerQueryBlockRange == 0 {
			cfg.Chains[idx].IndexerQueryBlockRange = 500
		}
		cfg.Chains[idx].TxType = NormalizeTxType(cfg.Chains[idx].TxType)
	}
	return cfg, cfg.Validate()
}

// NormalizeTxType applies the worker's default transaction type.
func NormalizeTxType(txType string) string {
	if txType == "" {
		return TxTypeDynamicFee
	}
	return txType
}

// Validate checks that required chains, pathways, and mode settings are internally consistent.
func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("database_url is required")
	}
	if len(c.Chains) == 0 {
		return errors.New("at least one chain is required")
	}
	if !common.IsHexAddress(c.Executor.Signer) {
		return errors.New("executor signer must be a hex address")
	}
	signers, err := c.validateSigners()
	if err != nil {
		return err
	}
	if _, ok := signers[common.HexToAddress(c.Executor.Signer).Hex()]; !ok {
		return errors.New("executor signer must reference a configured signer")
	}
	seen := make(map[uint32]struct{}, len(c.Chains))
	dynamicFeeCapsRequired := false
	for _, chain := range c.Chains {
		if chain.EID == 0 {
			return errors.New("chain eid is required")
		}
		if chain.Name == "" {
			return fmt.Errorf("chain %d name is required", chain.EID)
		}
		if chain.ChainID <= 0 {
			return fmt.Errorf("chain %s chain_id is required", chain.Name)
		}
		switch NormalizeTxType(chain.TxType) {
		case TxTypeDynamicFee, TxTypeLegacy:
		default:
			return fmt.Errorf("chain %s tx_type must be %q or %q", chain.Name, TxTypeDynamicFee, TxTypeLegacy)
		}
		if NormalizeTxType(chain.TxType) == TxTypeDynamicFee {
			dynamicFeeCapsRequired = true
		}
		for label, value := range map[string]string{
			"endpoint_address":      chain.EndpointAddress,
			"workers.open_executor": chain.Workers.OpenExecutor,
			"workers.open_dvn":      chain.Workers.OpenDVN,
		} {
			if !common.IsHexAddress(value) {
				return fmt.Errorf("chain %s %s must be a hex address", chain.Name, label)
			}
		}
		if chain.Confirmations != 12 {
			return fmt.Errorf("chain %s confirmations must be 12 in phase 1", chain.Name)
		}
		if chain.IndexerQueryBlockRange == 0 {
			return fmt.Errorf("chain %s indexer_query_block_range is required", chain.Name)
		}
		if len(chain.RPCURLs) == 0 {
			return fmt.Errorf("chain %s must configure at least one rpc url", chain.Name)
		}
		for i, rpcURL := range chain.RPCURLs {
			if err := validateRPCURL(rpcURL); err != nil {
				return fmt.Errorf("chain %s rpc_urls[%d] is invalid: %w", chain.Name, i, err)
			}
		}
		if _, ok := seen[chain.EID]; ok {
			return fmt.Errorf("duplicate chain eid %d", chain.EID)
		}
		seen[chain.EID] = struct{}{}
	}
	switch c.DVN.Mode {
	case "shadow", "active":
	default:
		return fmt.Errorf("unsupported dvn mode %q", c.DVN.Mode)
	}
	if c.DVN.Mode == "active" {
		if !common.IsHexAddress(c.DVN.Signer) {
			return errors.New("dvn signer must be a hex address in active mode")
		}
		if _, ok := signers[common.HexToAddress(c.DVN.Signer).Hex()]; !ok {
			return errors.New("dvn signer must reference a configured signer")
		}
		if c.DVN.TxGasLimit == 0 {
			return errors.New("dvn tx_gas_limit is required in active mode")
		}
		for label, value := range map[string]string{
			"max_fee_per_gas_wei":          c.DVN.MaxFeePerGasWei,
			"max_priority_fee_per_gas_wei": c.DVN.MaxPriorityFeePerGasWei,
		} {
			if value == "" {
				if dynamicFeeCapsRequired {
					return fmt.Errorf("dvn %s is required in active mode when a dynamic_fee chain is configured", label)
				}
				continue
			}
			parsed, ok := new(big.Int).SetString(value, 10)
			if !ok || parsed.Sign() <= 0 {
				return fmt.Errorf("dvn %s must be a positive integer", label)
			}
		}
	}
	if err := c.validatePricing(seen, signers, dynamicFeeCapsRequired); err != nil {
		return err
	}
	pathways := make(map[string]struct{}, len(c.Pathways))
	for _, pathway := range c.Pathways {
		if _, ok := seen[pathway.SrcEID]; !ok {
			return fmt.Errorf("pathway source eid %d is not configured", pathway.SrcEID)
		}
		if _, ok := seen[pathway.DstEID]; !ok {
			return fmt.Errorf("pathway destination eid %d is not configured", pathway.DstEID)
		}
		if pathway.SrcEID == pathway.DstEID {
			return fmt.Errorf("pathway %d -> %d must cross chains", pathway.SrcEID, pathway.DstEID)
		}
		for label, value := range map[string]string{
			"src_oapp":    pathway.SrcOApp,
			"dst_oapp":    pathway.DstOApp,
			"send_lib":    pathway.SendLib,
			"receive_lib": pathway.ReceiveLib,
		} {
			if !common.IsHexAddress(value) {
				return fmt.Errorf("pathway %d -> %d %s must be a hex address", pathway.SrcEID, pathway.DstEID, label)
			}
		}
		if pathway.MaxMessageSize == 0 {
			return fmt.Errorf("pathway %d -> %d max_message_size is required", pathway.SrcEID, pathway.DstEID)
		}
		if pathway.MaxMessageSize > math.MaxInt32 {
			return fmt.Errorf("pathway %d -> %d max_message_size exceeds database integer limit", pathway.SrcEID, pathway.DstEID)
		}
		if pathway.MaxLzReceiveGas == 0 {
			return fmt.Errorf("pathway %d -> %d max_lz_receive_gas is required", pathway.SrcEID, pathway.DstEID)
		}
		if pathway.MinLzReceiveGas > pathway.MaxLzReceiveGas {
			return fmt.Errorf("pathway %d -> %d min_lz_receive_gas exceeds max_lz_receive_gas", pathway.SrcEID, pathway.DstEID)
		}
		key := fmt.Sprintf("%d:%d:%s:%s", pathway.SrcEID, pathway.DstEID, common.HexToAddress(pathway.SrcOApp), common.HexToAddress(pathway.DstOApp))
		if _, ok := pathways[key]; ok {
			return fmt.Errorf("duplicate pathway %s", key)
		}
		pathways[key] = struct{}{}
	}
	return nil
}

func validateRPCURL(raw string) error {
	if raw == "" {
		return errors.New("value is required")
	}
	if strings.TrimSpace(raw) != raw {
		return errors.New("value must not contain leading or trailing whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "http", "https", "ws", "wss":
		if parsed.Host == "" {
			return fmt.Errorf("%s url must include a host", parsed.Scheme)
		}
		return nil
	case "":
		if !filepath.IsAbs(raw) {
			return errors.New("ipc path must be absolute")
		}
		return nil
	default:
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
}

func (c Config) validateSigners() (map[string]struct{}, error) {
	if len(c.Signers) == 0 {
		return nil, errors.New("at least one signer is required")
	}
	seen := make(map[string]struct{}, len(c.Signers))
	for _, signer := range c.Signers {
		if !common.IsHexAddress(signer.ID) {
			return nil, errors.New("signer id must be a hex address")
		}
		id := common.HexToAddress(signer.ID).Hex()
		if _, ok := seen[id]; ok {
			return nil, fmt.Errorf("duplicate signer id %s", id)
		}
		switch signer.Type {
		case "keystore":
			if signer.Keystore.Path == "" {
				return nil, fmt.Errorf("signer %s keystore.path is required", id)
			}
			sources := 0
			for _, value := range []string{signer.Keystore.PasswordEnv, signer.Keystore.PasswordFile} {
				if value != "" {
					sources++
				}
			}
			if sources != 1 {
				return nil, fmt.Errorf("signer %s must configure exactly one keystore password source", id)
			}
		case "kms":
			if !common.IsHexAddress(signer.KMS.Address) {
				return nil, fmt.Errorf("signer %s kms.address must be a hex address", id)
			}
			if common.HexToAddress(signer.KMS.Address).Hex() != id {
				return nil, fmt.Errorf("signer %s kms.address must match id", id)
			}
			if signer.KMS.KeyID == "" {
				return nil, fmt.Errorf("signer %s kms.key_id is required", id)
			}
			if signer.KMS.Region == "" {
				return nil, fmt.Errorf("signer %s kms.region is required", id)
			}
		default:
			return nil, fmt.Errorf("unsupported signer type %q", signer.Type)
		}
		seen[id] = struct{}{}
	}
	return seen, nil
}

func (c Config) validatePricing(chains map[uint32]struct{}, signers map[string]struct{}, dynamicFeeCapsRequired bool) error {
	if !c.Pricing.Enabled {
		return nil
	}
	if !common.IsHexAddress(c.Pricing.Signer) {
		return errors.New("pricing signer must be a hex address")
	}
	if _, ok := signers[common.HexToAddress(c.Pricing.Signer).Hex()]; !ok {
		return errors.New("pricing signer must reference a configured signer")
	}
	if c.Pricing.IntervalSeconds == 0 {
		return errors.New("pricing interval_seconds is required")
	}
	if c.Pricing.StaleAfterSeconds == 0 {
		return errors.New("pricing stale_after_seconds is required")
	}
	if c.Pricing.MaxDeviationBps == 0 {
		return errors.New("pricing max_deviation_bps is required")
	}
	if c.Pricing.GasSpikeBps == 0 {
		return errors.New("pricing gas_spike_bps is required")
	}
	if c.Pricing.BufferBps > 10_000 {
		return errors.New("pricing buffer_bps exceeds 10000")
	}
	if c.Pricing.BaseFeeWei == "" {
		return errors.New("pricing base_fee_wei is required")
	}
	baseFee, ok := new(big.Int).SetString(c.Pricing.BaseFeeWei, 10)
	if !ok || baseFee.Sign() < 0 {
		return errors.New("pricing base_fee_wei must be a non-negative integer")
	}
	for label, value := range map[string]string{
		"max_fee_per_gas_wei":          c.Pricing.MaxFeePerGasWei,
		"max_priority_fee_per_gas_wei": c.Pricing.MaxPriorityFeePerGasWei,
	} {
		if value == "" {
			if dynamicFeeCapsRequired {
				return fmt.Errorf("pricing %s is required when a dynamic_fee chain is configured", label)
			}
			continue
		}
		parsed, ok := new(big.Int).SetString(value, 10)
		if !ok || parsed.Sign() < 0 {
			return fmt.Errorf("pricing %s must be a non-negative integer", label)
		}
	}
	if c.Pricing.TxGasLimit == 0 {
		return errors.New("pricing tx_gas_limit is required")
	}
	primarySource := c.Pricing.PrimarySource
	if primarySource == "" {
		primarySource = "binance"
	}
	switch primarySource {
	case "binance", "coinmarketcap", "coingecko":
	default:
		return fmt.Errorf("unsupported pricing primary_source %q", c.Pricing.PrimarySource)
	}
	if primarySource == "coinmarketcap" && c.Pricing.CoinMarketCapAPIKeyEnv == "" {
		return errors.New("pricing coinmarketcap_api_key_env is required when coinmarketcap is primary")
	}
	seen := make(map[uint32]struct{}, len(c.Pricing.Chains))
	for _, chain := range c.Pricing.Chains {
		if _, ok := chains[chain.EID]; !ok {
			return fmt.Errorf("pricing chain eid %d is not configured", chain.EID)
		}
		if _, ok := seen[chain.EID]; ok {
			return fmt.Errorf("duplicate pricing chain eid %d", chain.EID)
		}
		seen[chain.EID] = struct{}{}
		if chain.CoinMarketCapSymbol != "" && c.Pricing.CoinMarketCapAPIKeyEnv == "" {
			return fmt.Errorf("pricing chain %d coinmarketcap_api_key_env is required when coinmarketcap_symbol is configured", chain.EID)
		}
		switch primarySource {
		case "binance":
			if chain.BinanceSymbol == "" {
				return fmt.Errorf("pricing chain %d binance_symbol is required", chain.EID)
			}
		case "coinmarketcap":
			if chain.CoinMarketCapSymbol == "" {
				return fmt.Errorf("pricing chain %d coinmarketcap_symbol is required", chain.EID)
			}
		case "coingecko":
			if chain.CoinGeckoID == "" {
				return fmt.Errorf("pricing chain %d coingecko_id is required", chain.EID)
			}
		}
		for label, value := range map[string]string{
			"uniswap.quoter_address": chain.Uniswap.QuoterAddress,
			"uniswap.token_in":       chain.Uniswap.TokenIn,
			"uniswap.token_out":      chain.Uniswap.TokenOut,
		} {
			if !common.IsHexAddress(value) {
				return fmt.Errorf("pricing chain %d %s must be a hex address", chain.EID, label)
			}
		}
		if chain.Uniswap.Fee > (1<<24)-1 {
			return fmt.Errorf("pricing chain %d uniswap fee exceeds uint24", chain.EID)
		}
		if chain.Uniswap.AmountInWei == "" {
			return fmt.Errorf("pricing chain %d uniswap amount_in_wei is required", chain.EID)
		}
		amountIn, ok := new(big.Int).SetString(chain.Uniswap.AmountInWei, 10)
		if !ok || amountIn.Sign() <= 0 {
			return fmt.Errorf("pricing chain %d uniswap amount_in_wei must be positive", chain.EID)
		}
		if chain.Uniswap.TokenOutDecimals == 0 {
			return fmt.Errorf("pricing chain %d uniswap token_out_decimals is required", chain.EID)
		}
	}
	if len(seen) != len(chains) {
		return errors.New("pricing must configure every chain when enabled")
	}
	return nil
}
