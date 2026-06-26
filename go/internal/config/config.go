package config

import (
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"
)

// Config is the startup configuration for the single-process worker.
type Config struct {
	DatabaseURL string          `yaml:"database_url"`
	Metrics     MetricsConfig   `yaml:"metrics"`
	Executor    ExecutorConfig  `yaml:"executor"`
	DVN         DVNConfig       `yaml:"dvn"`
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
	Mode string `yaml:"mode"`
}

// ChainConfig defines one LayerZero endpoint chain watched by the worker.
type ChainConfig struct {
	EID             uint32                `yaml:"eid"`
	Name            string                `yaml:"name"`
	ChainID         int64                 `yaml:"chain_id"`
	EndpointAddress string                `yaml:"endpoint_address"`
	Confirmations   uint64                `yaml:"confirmations"`
	RPCURLs         []string              `yaml:"rpc_urls"`
	Workers         WorkerContractsConfig `yaml:"workers"`
}

// WorkerContractsConfig identifies the self-hosted worker contracts deployed on one chain.
type WorkerContractsConfig struct {
	OpenExecutor string `yaml:"open_executor"`
	OpenDVN      string `yaml:"open_dvn"`
}

// PathwayConfig defines an allowed source-to-destination message pathway.
type PathwayConfig struct {
	SrcEID         uint32 `yaml:"src_eid"`
	DstEID         uint32 `yaml:"dst_eid"`
	SrcOApp        string `yaml:"src_oapp"`
	DstOApp        string `yaml:"dst_oapp"`
	SendLib        string `yaml:"send_lib"`
	ReceiveLib     string `yaml:"receive_lib"`
	Enabled        bool   `yaml:"enabled"`
	MaxMessageSize uint64 `yaml:"max_message_size"`
}

// Load reads a YAML config file, applies environment overrides, and validates the result.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if env := os.Getenv("DATABASE_URL"); env != "" {
		// Compose and managed deployments inject credentials through the environment.
		cfg.DatabaseURL = env
	}
	if cfg.Metrics.ListenAddress == "" {
		cfg.Metrics.ListenAddress = ":9090"
	}
	if cfg.DVN.Mode == "" {
		cfg.DVN.Mode = "shadow"
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
	if !common.IsHexAddress(c.Executor.Signer) {
		return errors.New("executor signer must be a hex address")
	}
	seen := make(map[uint32]struct{}, len(c.Chains))
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
		for label, value := range map[string]string{
			"endpoint_address":      chain.EndpointAddress,
			"workers.open_executor": chain.Workers.OpenExecutor,
			"workers.open_dvn":      chain.Workers.OpenDVN,
		} {
			if !common.IsHexAddress(value) {
				return fmt.Errorf("chain %s %s must be a hex address", chain.Name, label)
			}
		}
		if chain.Confirmations == 0 {
			return fmt.Errorf("chain %s confirmations is required", chain.Name)
		}
		if len(chain.RPCURLs) == 0 {
			return fmt.Errorf("chain %s must configure at least one rpc url", chain.Name)
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
		key := fmt.Sprintf("%d:%d:%s:%s", pathway.SrcEID, pathway.DstEID, common.HexToAddress(pathway.SrcOApp), common.HexToAddress(pathway.DstOApp))
		if _, ok := pathways[key]; ok {
			return fmt.Errorf("duplicate pathway %s", key)
		}
		pathways[key] = struct{}{}
	}
	return nil
}
