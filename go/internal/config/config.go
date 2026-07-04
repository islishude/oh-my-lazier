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

	"gopkg.in/yaml.v3"
)

// ChainFamily identifies the chain runtime family for a configured LayerZero endpoint.
type ChainFamily string

const (
	// ChainFamilyEVM selects the phase-1 EVM runtime.
	ChainFamilyEVM ChainFamily = "evm"
)

// DVNMode selects whether the DVN verifier only reports or also submits verification transactions.
type DVNMode string

const (
	// DVNModeShadow verifies and reports what the DVN would submit without sending transactions.
	DVNModeShadow DVNMode = "shadow"
	// DVNModeActive verifies and enqueues active DVN verification transactions.
	DVNModeActive DVNMode = "active"
)

// Config is the startup configuration for the single-process worker.
type Config struct {
	// DatabaseURL is the Postgres connection string; Load may override it with DATABASE_URL.
	DatabaseURL string `yaml:"database_url"`
	// Metrics controls the local HTTP listener used for worker metrics and health probes.
	Metrics MetricsConfig `yaml:"metrics"`
	// Services controls which durable worker loops this process starts; omitted roles default to enabled.
	Services ServicesConfig `yaml:"services"`
	// Pricing controls the optional price bot that enqueues worker price-config updates.
	Pricing PricingConfig `yaml:"pricing"`
	// Signers lists local signing backends referenced by pricing and chain transaction roles.
	Signers []SignerConfig `yaml:"signers"`
	// Chains defines LayerZero endpoint chains, RPC access, and local tx submission policy.
	Chains []ChainConfig `yaml:"chains"`
	// Pathways defines source-to-destination OApp routes, workers, limits, and worker fee models.
	Pathways []PathwayConfig `yaml:"pathways"`
}

// MetricsConfig controls the worker HTTP metrics and health endpoint.
type MetricsConfig struct {
	// ListenAddress is the metrics HTTP bind address; it defaults to :9090 when omitted.
	ListenAddress string `yaml:"listen_address"`
}

// ServicesConfig selects which durable worker roles this process runs.
type ServicesConfig struct {
	// Executor enables the executor loop and its local transaction requirements.
	Executor ServiceToggleConfig `yaml:"executor"`
	// DVN enables the DVN verification loop; active submission still depends on pathway mode.
	DVN ServiceToggleConfig `yaml:"dvn"`
}

// ServiceToggleConfig controls one optional worker role.
type ServiceToggleConfig struct {
	// Enabled is tri-state so omitted YAML can use the caller-supplied default.
	Enabled *bool `yaml:"enabled"`
}

// EnabledOrDefault returns the configured service state or the supplied default.
func (s ServiceToggleConfig) EnabledOrDefault(defaultEnabled bool) bool {
	if s.Enabled == nil {
		return defaultEnabled
	}
	return *s.Enabled
}

// ExecutorEnabled reports whether this process should run executor workflows.
func (c Config) ExecutorEnabled() bool {
	return c.Services.Executor.EnabledOrDefault(true)
}

// DVNEnabled reports whether this process should run DVN workflows.
func (c Config) DVNEnabled() bool {
	return c.Services.DVN.EnabledOrDefault(true)
}

// ExecutorTxRoleConfig controls executor transaction submission on one chain.
type ExecutorTxRoleConfig struct {
	// Signer references a configured signer used for executor destination transactions on this chain.
	Signer EVMAddress `yaml:"signer"`
	// MaxFeePerGasWei caps txmgr send-time gas pricing for executor transactions.
	MaxFeePerGasWei string `yaml:"max_fee_per_gas_wei"`
	// MaxPriorityFeePerGasWei caps dynamic-fee priority tips; legacy transactions may leave it empty.
	MaxPriorityFeePerGasWei string `yaml:"max_priority_fee_per_gas_wei"`
}

// DVNTxRoleConfig controls active DVN verification transaction submission on one chain.
type DVNTxRoleConfig struct {
	// Signer references a configured signer used only when a pathway requires active DVN submission.
	Signer EVMAddress `yaml:"signer"`
	// MaxFeePerGasWei caps txmgr send-time gas pricing for DVN verification transactions.
	MaxFeePerGasWei string `yaml:"max_fee_per_gas_wei"`
	// MaxPriorityFeePerGasWei caps dynamic-fee priority tips; shadow-only DVN configs may omit all role fields.
	MaxPriorityFeePerGasWei string `yaml:"max_priority_fee_per_gas_wei"`
}

// ChainTxRolesConfig configures local transaction roles for one chain.
type ChainTxRolesConfig struct {
	// Executor is the local transaction policy for executor deliveries on this destination chain.
	Executor ExecutorTxRoleConfig `yaml:"executor"`
	// DVN is the local transaction policy for active DVN verification on this destination chain.
	DVN DVNTxRoleConfig `yaml:"dvn"`
}

// PricingConfig controls optional price update generation.
type PricingConfig struct {
	// Enabled turns on price-bot startup validation and setPriceConfig transaction generation.
	Enabled bool `yaml:"enabled"`
	// Signer references the local signer used for price-config update transactions.
	Signer EVMAddress `yaml:"signer"`
	// IntervalSeconds is the scheduled full refresh interval; it defaults to 300 when pricing is enabled.
	IntervalSeconds uint64 `yaml:"interval_seconds"`
	// StaleAfterSeconds is written into worker PriceConfig and defaults to 1800 when pricing is enabled.
	StaleAfterSeconds uint64 `yaml:"stale_after_seconds"`
	// MaxDeviationBps is the allowed primary-vs-sanity feed deviation; it defaults to 500.
	MaxDeviationBps uint64 `yaml:"max_deviation_bps"`
	// GasSpikeBps triggers early updates when destination gas rises past the previous quoted price.
	GasSpikeBps uint64 `yaml:"gas_spike_bps"`
	// AllowSanityFallback lets the bot use a healthy sanity source only when the primary source is unhealthy.
	AllowSanityFallback bool `yaml:"allow_sanity_fallback"`
	// MaxFeePerGasWei caps txmgr send-time gas pricing for price-config update transactions.
	MaxFeePerGasWei string `yaml:"max_fee_per_gas_wei"`
	// MaxPriorityFeePerGasWei caps dynamic-fee priority tips for price-config update transactions.
	MaxPriorityFeePerGasWei string `yaml:"max_priority_fee_per_gas_wei"`
	// BinanceBaseURL optionally overrides the Binance HTTP API endpoint.
	BinanceBaseURL string `yaml:"binance_base_url"`
	// CoinMarketCapBaseURL optionally overrides the CoinMarketCap HTTP API endpoint.
	CoinMarketCapBaseURL string `yaml:"coinmarketcap_base_url"`
	// CoinMarketCapAPIKeyEnv names the environment variable containing the CoinMarketCap API key.
	CoinMarketCapAPIKeyEnv string `yaml:"coinmarketcap_api_key_env"`
	// CoinGeckoBaseURL optionally overrides the CoinGecko HTTP API endpoint.
	CoinGeckoBaseURL string `yaml:"coingecko_base_url"`
	// Chains configures native-asset price feeds for every chain when pricing is enabled.
	Chains []PricingChainConfig `yaml:"chains"`
}

// WorkerFeeModelConfig controls one worker role's source-chain service fee model.
type WorkerFeeModelConfig struct {
	// FixedFeeWei is the fixed source-chain native-token fee component for this worker role.
	FixedFeeWei string `yaml:"fixed_fee_wei"`
	// DstGasOverhead is the fixed destination gas unit component added before price conversion.
	DstGasOverhead uint64 `yaml:"dst_gas_overhead"`
	// MarginBps is applied after fixed fee plus destination gas cost; it must not exceed 10000.
	MarginBps uint16 `yaml:"margin_bps"`
}

// PricingChainConfig configures price sources for one chain's native asset.
type PricingChainConfig struct {
	// EID links this feed config to one configured ChainConfig endpoint ID.
	EID uint32 `yaml:"eid"`
	// PrimarySource is the price source the bot quotes from; supported values exclude uniswap.
	PrimarySource string `yaml:"primary_source"`
	// SanitySources cross-check the primary source and must include uniswap without duplicating the primary.
	SanitySources []string `yaml:"sanity_sources"`
	// BinanceSymbol is required when binance is selected as primary or sanity source.
	BinanceSymbol string `yaml:"binance_symbol"`
	// CoinMarketCapSymbol is required when coinmarketcap is selected as primary or sanity source.
	CoinMarketCapSymbol string `yaml:"coinmarketcap_symbol"`
	// CoinGeckoID is required when coingecko is selected as primary or sanity source.
	CoinGeckoID string `yaml:"coingecko_id"`
	// Uniswap configures the on-chain V3 sanity route for this chain's native asset.
	Uniswap UniswapPricingConfig `yaml:"uniswap"`
}

// UniswapPricingConfig configures one V3 quoter sanity route.
type UniswapPricingConfig struct {
	// QuoterAddress is the Uniswap V3 quoter contract used for sanity pricing.
	QuoterAddress EVMAddress `yaml:"quoter_address"`
	// TokenIn is the chain-native or wrapped-native token being priced.
	TokenIn EVMAddress `yaml:"token_in"`
	// TokenOut is the reference token returned by the quoter, usually a stablecoin.
	TokenOut EVMAddress `yaml:"token_out"`
	// Fee is the Uniswap V3 pool fee tier encoded as a uint24-compatible value.
	Fee uint32 `yaml:"fee"`
	// AmountInWei is the positive token-in amount used for the sanity quote.
	AmountInWei string `yaml:"amount_in_wei"`
	// TokenOutDecimals converts the quoted token-out amount into USD/native units.
	TokenOutDecimals uint8 `yaml:"token_out_decimals"`
}

// SignerConfig configures one local signing backend without embedding raw secret material.
type SignerConfig struct {
	// ID is the address other config sections reference as a local signer.
	ID EVMAddress `yaml:"id"`
	// Type selects the signer backend; supported values are keystore and kms.
	Type string `yaml:"type"`
	// Keystore configures a local geth keystore backend when Type is keystore.
	Keystore KeystoreSignerConfig `yaml:"keystore"`
	// KMS configures an AWS-compatible KMS backend when Type is kms.
	KMS KMSSignerConfig `yaml:"kms"`
}

// KeystoreSignerConfig points at an encrypted geth keystore and its password source.
type KeystoreSignerConfig struct {
	// Path is the encrypted geth keystore JSON path available to the worker process.
	Path string `yaml:"path"`
	// PasswordEnv names the environment variable containing the keystore password.
	PasswordEnv string `yaml:"password_env"`
	// PasswordFile points at a file containing the keystore password; use exactly one password source.
	PasswordFile string `yaml:"password_file"`
}

// KMSSignerConfig points at an AWS-compatible KMS signing key.
type KMSSignerConfig struct {
	// KeyID identifies the KMS key used for secp256k1 signing.
	KeyID string `yaml:"key_id"`
	// Region selects the AWS region for the KMS client.
	Region string `yaml:"region"`
	// Address is the EVM account controlled by the KMS key and must match the signer ID.
	Address EVMAddress `yaml:"address"`
	// Endpoint optionally targets a compatible local or managed KMS endpoint.
	Endpoint string `yaml:"endpoint"`
}

// ChainConfig defines one LayerZero endpoint chain watched by the worker.
type ChainConfig struct {
	// EID is the LayerZero endpoint ID and is the stable key used by pathways and pricing.
	EID uint32 `yaml:"eid"`
	// Name is a human-readable chain label used in logs and validation errors.
	Name string `yaml:"name"`
	// Family must be evm in phase 1; non-EVM chain families are intentionally unsupported.
	Family ChainFamily `yaml:"family"`
	// ChainID is the EVM chain ID every configured RPC endpoint must report.
	ChainID uint64 `yaml:"chain_id"`
	// EndpointAddress is the LayerZero EndpointV2 contract address on this chain.
	EndpointAddress EVMAddress `yaml:"endpoint_address"`
	// Confirmations is the minimum confirmation depth before indexed source events are trusted.
	Confirmations uint64 `yaml:"confirmations"`
	// StartBlockNumber seeds the first indexer backfill when no durable cursor exists; omitted means 0.
	StartBlockNumber uint64 `yaml:"start_block_number"`
	// IndexerQueryBlockRange bounds each FilterLogs window and defaults to 500 when omitted.
	IndexerQueryBlockRange uint64 `yaml:"indexer_query_block_range"`
	// RPCURLs lists every RPC endpoint in the quorum; http(s), ws(s), and absolute IPC paths are supported.
	RPCURLs []string `yaml:"rpc_urls"`
	// TxRoles defines local send-time tx policies for worker submissions on this chain.
	TxRoles ChainTxRolesConfig `yaml:"tx_roles"`
}

// WorkerContractsConfig identifies the self-hosted worker contracts selected for one source pathway.
type WorkerContractsConfig struct {
	// OpenExecutor is the source-chain executor configured for this pathway.
	OpenExecutor EVMAddress `yaml:"open_executor"`
	// OpenDVN is the source-chain DVN configured in the source SendUln required DVNs.
	OpenDVN EVMAddress `yaml:"open_dvn"`
}

// DestinationWorkerContractsConfig identifies target-chain worker contracts selected for a pathway.
type DestinationWorkerContractsConfig struct {
	// OpenDVN is the destination-chain OpenDVN whose verifier authorization is checked for active DVN flow.
	OpenDVN EVMAddress `yaml:"open_dvn"`
}

// PathwayDVNConfig controls DVN behavior for one source-to-destination pathway.
type PathwayDVNConfig struct {
	// Mode defaults to shadow; active mode enqueues destination-chain DVN verification transactions.
	Mode DVNMode `yaml:"mode"`
}

// PathwayPricingConfig controls price updates for one source-to-destination pathway.
type PathwayPricingConfig struct {
	// ExecutorFee is the source-chain worker quote model for the pathway's OpenExecutor.
	ExecutorFee WorkerFeeModelConfig `yaml:"executor_fee"`
	// DVNFee is the source-chain worker quote model for the pathway's OpenDVN.
	DVNFee WorkerFeeModelConfig `yaml:"dvn_fee"`
}

// PathwayConfig defines an allowed source-to-destination message pathway.
type PathwayConfig struct {
	// SrcEID is the source LayerZero endpoint ID and part of the pathway identity.
	SrcEID uint32 `yaml:"src_eid"`
	// DstEID is the destination LayerZero endpoint ID and part of the pathway identity.
	DstEID uint32 `yaml:"dst_eid"`
	// SrcOApp is the source-chain OApp address and part of the pathway identity.
	SrcOApp EVMAddress `yaml:"src_oapp"`
	// DstOApp is the destination-chain OApp peer address and part of the pathway identity.
	DstOApp EVMAddress `yaml:"dst_oapp"`
	// SendLib is the source-chain LayerZero send library expected for this pathway.
	SendLib EVMAddress `yaml:"send_lib"`
	// ReceiveLib is the destination-chain LayerZero receive library expected for this pathway.
	ReceiveLib EVMAddress `yaml:"receive_lib"`
	// SourceWorkers selects the source-chain OpenExecutor and OpenDVN contracts for this route.
	SourceWorkers WorkerContractsConfig `yaml:"source_workers"`
	// DestinationWorkers selects destination-side worker contracts used for verification checks.
	DestinationWorkers DestinationWorkerContractsConfig `yaml:"destination_workers"`
	// DVN controls whether the local DVN stays in shadow mode or actively submits verification.
	DVN PathwayDVNConfig `yaml:"dvn"`
	// Pricing holds pathway-scoped worker quote models; it is required only when pricing is enabled.
	Pricing PathwayPricingConfig `yaml:"pricing"`
	// Enabled is the expected on-chain worker pathway enablement, not the process service toggle.
	Enabled bool `yaml:"enabled"`
	// MaxMessageSize is the maximum source message payload size accepted by the workers.
	MaxMessageSize uint64 `yaml:"max_message_size"`
	// MinLzReceiveGas is the minimum executor lzReceive gas accepted for this pathway.
	MinLzReceiveGas uint64 `yaml:"min_lz_receive_gas"`
	// MaxLzReceiveGas is the maximum executor lzReceive gas accepted for this pathway.
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
	}
	for idx := range cfg.Pathways {
		if cfg.Pathways[idx].DVN.Mode == "" {
			cfg.Pathways[idx].DVN.Mode = DVNModeShadow
		}
	}
	return cfg, cfg.Validate()
}

// Validate checks that required chains, pathways, and mode settings are internally consistent.
func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("database_url is required")
	}
	if len(c.Chains) == 0 {
		return errors.New("at least one chain is required")
	}
	signers, err := c.validateSigners()
	if err != nil {
		return err
	}
	seen := make(map[uint32]struct{}, len(c.Chains))
	chains := make(map[uint32]ChainConfig, len(c.Chains))
	for _, chain := range c.Chains {
		if chain.EID == 0 {
			return errors.New("chain eid is required")
		}
		if chain.Name == "" {
			return fmt.Errorf("chain %d name is required", chain.EID)
		}
		switch chain.Family {
		case ChainFamilyEVM:
		case "":
			return fmt.Errorf("chain %s family is required", chain.Name)
		default:
			return fmt.Errorf("chain %s family must be %q in phase 1", chain.Name, ChainFamilyEVM)
		}
		if chain.ChainID <= 0 {
			return fmt.Errorf("chain %s chain_id is required", chain.Name)
		}
		if chain.EndpointAddress.IsZero() {
			return fmt.Errorf("chain %s endpoint_address is required", chain.Name)
		}
		if c.ExecutorEnabled() {
			if err := validateExecutorTxRole(chain.Name, chain.TxRoles.Executor, signers); err != nil {
				return err
			}
		}
		if c.DVNEnabled() {
			if err := validateOptionalDVNTxRole(chain.Name, chain.TxRoles.DVN); err != nil {
				return err
			}
		}
		if chain.Confirmations == 0 {
			return fmt.Errorf("chain %s confirmations is required", chain.Name)
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
		chains[chain.EID] = chain
	}
	if err := c.validatePricing(seen, signers); err != nil {
		return err
	}
	pathways := make(map[string]struct{}, len(c.Pathways))
	activeDVNDestinations := make(map[uint32]struct{})
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
		for label, value := range map[string]EVMAddress{
			"src_oapp":                     pathway.SrcOApp,
			"dst_oapp":                     pathway.DstOApp,
			"send_lib":                     pathway.SendLib,
			"receive_lib":                  pathway.ReceiveLib,
			"source_workers.open_executor": pathway.SourceWorkers.OpenExecutor,
			"source_workers.open_dvn":      pathway.SourceWorkers.OpenDVN,
			"destination_workers.open_dvn": pathway.DestinationWorkers.OpenDVN,
		} {
			if value.IsZero() {
				return fmt.Errorf("pathway %d -> %d %s is required", pathway.SrcEID, pathway.DstEID, label)
			}
		}
		switch pathway.DVN.Mode {
		case DVNModeShadow:
		case DVNModeActive:
			activeDVNDestinations[pathway.DstEID] = struct{}{}
		default:
			return fmt.Errorf("pathway %d -> %d unsupported dvn mode %q", pathway.SrcEID, pathway.DstEID, pathway.DVN.Mode)
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
		if c.Pricing.Enabled {
			if err := validatePathwayPricing(pathway); err != nil {
				return err
			}
		}
		key := fmt.Sprintf("%d:%d:%s:%s", pathway.SrcEID, pathway.DstEID, pathway.SrcOApp, pathway.DstOApp)
		if _, ok := pathways[key]; ok {
			return fmt.Errorf("duplicate pathway %s", key)
		}
		pathways[key] = struct{}{}
	}
	if c.DVNEnabled() {
		for eid := range activeDVNDestinations {
			chain := chains[eid]
			if err := validateRequiredDVNTxRole(chain.Name, chain.TxRoles.DVN, signers); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExecutorTxRole(chainName string, role ExecutorTxRoleConfig, signers map[string]struct{}) error {
	if role.Signer.IsZero() {
		return fmt.Errorf("chain %s tx_roles.executor.signer is required", chainName)
	}
	if _, ok := signers[role.Signer.Hex()]; !ok {
		return fmt.Errorf("chain %s tx_roles.executor.signer must reference a configured signer", chainName)
	}
	return validateTxFeePolicy(fmt.Sprintf("chain %s tx_roles.executor", chainName), role.MaxFeePerGasWei, role.MaxPriorityFeePerGasWei)
}

func validateOptionalDVNTxRole(chainName string, role DVNTxRoleConfig) error {
	return validateOptionalTxFeePolicy(fmt.Sprintf("chain %s tx_roles.dvn", chainName), role.MaxFeePerGasWei, role.MaxPriorityFeePerGasWei)
}

func validateRequiredDVNTxRole(chainName string, role DVNTxRoleConfig, signers map[string]struct{}) error {
	if role.Signer.IsZero() {
		return fmt.Errorf("chain %s tx_roles.dvn.signer is required for active dvn pathways", chainName)
	}
	if _, ok := signers[role.Signer.Hex()]; !ok {
		return fmt.Errorf("chain %s tx_roles.dvn.signer must reference a configured signer", chainName)
	}
	return validateTxFeePolicy(fmt.Sprintf("chain %s tx_roles.dvn", chainName), role.MaxFeePerGasWei, role.MaxPriorityFeePerGasWei)
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
	seen := make(map[string]struct{}, len(c.Signers))
	for _, signer := range c.Signers {
		if signer.ID.IsZero() {
			return nil, errors.New("signer id is required")
		}
		id := signer.ID.Hex()
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
			if signer.KMS.Address.IsZero() {
				return nil, fmt.Errorf("signer %s kms.address is required", id)
			}
			if signer.KMS.Address.Hex() != id {
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

func (c Config) validatePricing(chains map[uint32]struct{}, signers map[string]struct{}) error {
	if !c.Pricing.Enabled {
		return nil
	}
	if c.Pricing.Signer.IsZero() {
		return errors.New("pricing signer is required")
	}
	if _, ok := signers[c.Pricing.Signer.Hex()]; !ok {
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
	if err := validateTxFeePolicy("pricing", c.Pricing.MaxFeePerGasWei, c.Pricing.MaxPriorityFeePerGasWei); err != nil {
		return err
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
		if err := validatePricingChainSources(chain, c.Pricing.CoinMarketCapAPIKeyEnv); err != nil {
			return err
		}
	}
	if len(seen) != len(chains) {
		return errors.New("pricing must configure every chain when enabled")
	}
	return nil
}

func validatePathwayPricing(pathway PathwayConfig) error {
	prefix := fmt.Sprintf("pathway %d -> %d pricing", pathway.SrcEID, pathway.DstEID)
	if err := validateWorkerFeeModel(prefix+".executor_fee", pathway.Pricing.ExecutorFee); err != nil {
		return err
	}
	if err := validateWorkerFeeModel(prefix+".dvn_fee", pathway.Pricing.DVNFee); err != nil {
		return err
	}
	return nil
}

func validateWorkerFeeModel(prefix string, model WorkerFeeModelConfig) error {
	if model.FixedFeeWei == "" {
		return fmt.Errorf("%s.fixed_fee_wei is required", prefix)
	}
	fixedFee, ok := new(big.Int).SetString(model.FixedFeeWei, 10)
	if !ok || fixedFee.Sign() < 0 {
		return fmt.Errorf("%s.fixed_fee_wei must be a non-negative integer", prefix)
	}
	if model.MarginBps > 10_000 {
		return fmt.Errorf("%s.margin_bps exceeds 10000", prefix)
	}
	return nil
}

func validatePricingChainSources(chain PricingChainConfig, coinMarketCapAPIKeyEnv string) error {
	if err := validatePrimaryPricingSourceName(chain.EID, chain.PrimarySource); err != nil {
		return err
	}
	if len(chain.SanitySources) == 0 {
		return fmt.Errorf("pricing chain %d sanity_sources is required", chain.EID)
	}
	seen := make(map[string]struct{}, len(chain.SanitySources)+1)
	seen[chain.PrimarySource] = struct{}{}
	hasUniswap := false
	for idx, source := range chain.SanitySources {
		if err := validateSanityPricingSourceName(chain.EID, fmt.Sprintf("sanity_sources[%d]", idx), source); err != nil {
			return err
		}
		if _, ok := seen[source]; ok {
			return fmt.Errorf("pricing chain %d source %q is configured more than once", chain.EID, source)
		}
		seen[source] = struct{}{}
		if source == "uniswap" {
			hasUniswap = true
		}
	}
	if !hasUniswap {
		return fmt.Errorf("pricing chain %d sanity_sources must include uniswap", chain.EID)
	}
	for source := range seen {
		if err := validateConfiguredPricingSource(chain, source, coinMarketCapAPIKeyEnv); err != nil {
			return err
		}
	}
	return nil
}

func validatePrimaryPricingSourceName(eid uint32, source string) error {
	switch source {
	case "binance", "coinmarketcap", "coingecko":
		return nil
	case "":
		return fmt.Errorf("pricing chain %d primary_source is required", eid)
	default:
		return fmt.Errorf("pricing chain %d primary_source has unsupported source %q", eid, source)
	}
}

func validateSanityPricingSourceName(eid uint32, field, source string) error {
	switch source {
	case "binance", "coinmarketcap", "coingecko", "uniswap":
		return nil
	case "":
		return fmt.Errorf("pricing chain %d %s is required", eid, field)
	default:
		return fmt.Errorf("pricing chain %d %s has unsupported source %q", eid, field, source)
	}
}

func validateConfiguredPricingSource(chain PricingChainConfig, source, coinMarketCapAPIKeyEnv string) error {
	switch source {
	case "binance":
		if chain.BinanceSymbol == "" {
			return fmt.Errorf("pricing chain %d binance_symbol is required", chain.EID)
		}
	case "coinmarketcap":
		if coinMarketCapAPIKeyEnv == "" {
			return fmt.Errorf("pricing chain %d coinmarketcap_api_key_env is required when coinmarketcap is used", chain.EID)
		}
		if chain.CoinMarketCapSymbol == "" {
			return fmt.Errorf("pricing chain %d coinmarketcap_symbol is required", chain.EID)
		}
	case "coingecko":
		if chain.CoinGeckoID == "" {
			return fmt.Errorf("pricing chain %d coingecko_id is required", chain.EID)
		}
	case "uniswap":
		if err := validateUniswapPricingSource(chain); err != nil {
			return err
		}
	}
	return nil
}

func validateUniswapPricingSource(chain PricingChainConfig) error {
	for label, value := range map[string]EVMAddress{
		"uniswap.quoter_address": chain.Uniswap.QuoterAddress,
		"uniswap.token_in":       chain.Uniswap.TokenIn,
		"uniswap.token_out":      chain.Uniswap.TokenOut,
	} {
		if value.IsZero() {
			return fmt.Errorf("pricing chain %d %s is required", chain.EID, label)
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
	return nil
}

func validateTxFeePolicy(prefix, maxFeePerGasWei, maxPriorityFeePerGasWei string) error {
	if maxFeePerGasWei == "" {
		return fmt.Errorf("%s.max_fee_per_gas_wei is required", prefix)
	}
	maxFee, err := parsePositiveInteger(fmt.Sprintf("%s.max_fee_per_gas_wei", prefix), maxFeePerGasWei)
	if err != nil {
		return err
	}
	if maxPriorityFeePerGasWei == "" {
		return nil
	}
	priorityFee, err := parsePositiveInteger(fmt.Sprintf("%s.max_priority_fee_per_gas_wei", prefix), maxPriorityFeePerGasWei)
	if err != nil {
		return err
	}
	if priorityFee.Cmp(maxFee) > 0 {
		return fmt.Errorf("%s.max_priority_fee_per_gas_wei must not exceed max_fee_per_gas_wei", prefix)
	}
	return nil
}

func validateOptionalTxFeePolicy(prefix, maxFeePerGasWei, maxPriorityFeePerGasWei string) error {
	if maxFeePerGasWei == "" && maxPriorityFeePerGasWei == "" {
		return nil
	}
	return validateTxFeePolicy(prefix, maxFeePerGasWei, maxPriorityFeePerGasWei)
}

func parsePositiveInteger(field, value string) (*big.Int, error) {
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok || parsed.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", field)
	}
	return parsed, nil
}
