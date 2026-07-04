package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateAcceptsSepoliaPathways(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMissingWorkerContractAddress(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"source": func(cfg *Config) {
			cfg.Pathways[0].SourceWorkers.OpenDVN = EVMAddress{}
		},
		"destination": func(cfg *Config) {
			cfg.Pathways[0].DestinationWorkers.OpenDVN = EVMAddress{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want missing worker contract error")
			}
		})
	}
}

func TestValidateRejectsUnsupportedChainFamilies(t *testing.T) {
	for name, family := range map[string]ChainFamily{
		"missing": "",
		"solana":  "solana",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Chains[0].Family = family
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want unsupported chain family error")
			}
		})
	}
}

func TestEVMAddressUnmarshalRejectsInvalidValues(t *testing.T) {
	for name, body := range map[string]string{
		"empty":    `address: ""`,
		"invalid":  `address: not-an-address`,
		"sequence": `address: []`,
	} {
		t.Run(name, func(t *testing.T) {
			var target struct {
				Address EVMAddress `yaml:"address"`
			}
			if err := yaml.Unmarshal([]byte(body), &target); err == nil {
				t.Fatal("Unmarshal() error = nil, want invalid evm address error")
			}
		})
	}
}

func TestValidateRejectsInvalidPathwayGasBounds(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"missingMax": func(cfg *Config) {
			cfg.Pathways[0].MaxLzReceiveGas = 0
		},
		"minExceedsMax": func(cfg *Config) {
			cfg.Pathways[0].MinLzReceiveGas = 300000
			cfg.Pathways[0].MaxLzReceiveGas = 200000
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want invalid pathway gas bounds error")
			}
		})
	}
}

func TestValidateAcceptsConfiguredConfirmations(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].Confirmations = 6
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMissingConfirmations(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].Confirmations = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing confirmations error")
	}
}

func TestValidateAcceptsSupportedRPCURLFormats(t *testing.T) {
	for _, rpcURL := range []string{
		"http://localhost:8545",
		"https://rpc.example.com",
		"ws://localhost:8546",
		"wss://rpc.example.com/ws",
		"/var/run/geth.ipc",
	} {
		t.Run(rpcURL, func(t *testing.T) {
			cfg := validConfig()
			cfg.Chains[0].RPCURLs = []string{rpcURL}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidRPCURLFormats(t *testing.T) {
	for name, rpcURL := range map[string]string{
		"empty":       "",
		"whitespace":  " http://localhost:8545",
		"missingHost": "https:///rpc",
		"unknown":     "ftp://localhost/rpc",
		"relativeIPC": "geth.ipc",
		"ipcScheme":   "ipc:///var/run/geth.ipc",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Chains[0].RPCURLs = []string{rpcURL}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want invalid rpc url error")
			}
		})
	}
}

func TestValidateRejectsMissingExecutorSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].TxRoles.Executor.Signer = EVMAddress{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing executor signer error")
	}
}

func TestValidateRejectsUnknownExecutorSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].TxRoles.Executor.Signer = MustEVMAddress("0x1111111111111111111111111111111111111111")
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown executor signer error")
	}
}

func TestValidateRejectsIncompleteExecutorFeePolicy(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].TxRoles.Executor.MaxFeePerGasWei = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing executor max fee error")
	}
}

func TestValidateRoleAwareSignerRequirements(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{
			name: "executor disabled allows missing executor signer and fees",
			mutate: func(cfg *Config) {
				cfg.Services.Executor.Enabled = new(false)
				for i := range cfg.Chains {
					cfg.Chains[i].TxRoles.Executor = ExecutorTxRoleConfig{}
				}
			},
		},
		{
			name: "dvn disabled allows active pathway without dvn signer and fees",
			mutate: func(cfg *Config) {
				cfg.Services.DVN.Enabled = new(false)
				cfg.Pathways[0].DVN.Mode = DVNModeActive
				for i := range cfg.Chains {
					cfg.Chains[i].TxRoles.DVN = DVNTxRoleConfig{}
				}
			},
		},
		{
			name: "empty signers allowed when no enabled service needs signing",
			mutate: func(cfg *Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Services.DVN.Enabled = new(false)
				cfg.Signers = nil
				for i := range cfg.Chains {
					cfg.Chains[i].TxRoles.Executor = ExecutorTxRoleConfig{}
					cfg.Chains[i].TxRoles.DVN = DVNTxRoleConfig{}
				}
			},
		},
		{
			name: "empty signers rejected for executor",
			mutate: func(cfg *Config) {
				cfg.Services.DVN.Enabled = new(false)
				cfg.Signers = nil
			},
			wantErr: true,
		},
		{
			name: "empty signers rejected for active dvn",
			mutate: func(cfg *Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Pathways[0].DVN.Mode = DVNModeActive
				cfg.Signers = nil
			},
			wantErr: true,
		},
		{
			name: "empty signers rejected for pricing",
			mutate: func(cfg *Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Services.DVN.Enabled = new(false)
				cfg.Pricing = validPricingConfig()
				cfg.Signers = nil
			},
			wantErr: true,
		},
		{
			name: "pricing only allows missing worker tx roles with pricing signer",
			mutate: func(cfg *Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Services.DVN.Enabled = new(false)
				cfg.Pricing = validPricingConfig()
				for i := range cfg.Chains {
					cfg.Chains[i].TxRoles.Executor = ExecutorTxRoleConfig{}
					cfg.Chains[i].TxRoles.DVN = DVNTxRoleConfig{}
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			err := cfg.Validate()
			if test.wantErr && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateAcceptsActiveDVNConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsShadowDVNWithoutSigner(t *testing.T) {
	cfg := validConfig()
	for i := range cfg.Chains {
		cfg.Chains[i].TxRoles.DVN.Signer = EVMAddress{}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsActiveDVNWithoutSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	cfg.Chains[1].TxRoles.DVN.Signer = EVMAddress{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing active dvn signer error")
	}
}

func TestValidateRejectsActiveDVNWithoutFees(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	cfg.Chains[1].TxRoles.DVN.MaxFeePerGasWei = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing active dvn fee error")
	}
}

func TestValidateAcceptsActiveDVNWithoutPriorityFeeCap(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	cfg.Chains[1].TxRoles.DVN.MaxPriorityFeePerGasWei = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsKeystoreSignerWithoutPasswordSource(t *testing.T) {
	cfg := validConfig()
	cfg.Signers[0].Keystore.PasswordEnv = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing keystore password source error")
	}
}

func TestValidateRejectsMismatchedKMSSignerAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Signers[0] = SignerConfig{
		ID:   MustEVMAddress("0x9999999999999999999999999999999999999999"),
		Type: "kms",
		KMS: KMSSignerConfig{
			KeyID:   "test-key",
			Region:  "us-east-1",
			Address: MustEVMAddress("0x1111111111111111111111111111111111111111"),
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want mismatched kms address error")
	}
}

func TestValidateRejectsDuplicatePathway(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways = append(cfg.Pathways, cfg.Pathways[0])
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate pathway error")
	}
}

func TestValidateAcceptsEnabledPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsEnabledPricingWithoutPathwayFees(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pathways[0].Pricing = PathwayPricingConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing pathway pricing error")
	}
}

func TestValidateRejectsInvalidPathwayPricingFeeModel(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PathwayPricingConfig)
	}{
		{
			name: "negative fixed fee",
			mutate: func(pricing *PathwayPricingConfig) {
				pricing.ExecutorFee.FixedFeeWei = "-1"
			},
		},
		{
			name: "invalid fixed fee",
			mutate: func(pricing *PathwayPricingConfig) {
				pricing.ExecutorFee.FixedFeeWei = "abc"
			},
		},
		{
			name: "margin too high",
			mutate: func(pricing *PathwayPricingConfig) {
				pricing.DVNFee.MarginBps = 10_001
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Pricing = validPricingConfig()
			test.mutate(&cfg.Pathways[0].Pricing)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want invalid pathway pricing error")
			}
		})
	}
}

func TestValidateAllowsOmittedPathwayPricingWhenPricingDisabled(t *testing.T) {
	cfg := validConfig()
	for i := range cfg.Pathways {
		cfg.Pathways[i].Pricing = PathwayPricingConfig{}
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsIncompletePricingChains(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.Chains = cfg.Pricing.Chains[:1]
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want incomplete pricing chains error")
	}
}

func TestValidateAcceptsPricingWithoutPriorityFeeCap(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.MaxPriorityFeePerGasWei = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsCoinMarketCapPrimaryPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.CoinMarketCapAPIKeyEnv = "COINMARKETCAP_API_KEY"
	for i := range cfg.Pricing.Chains {
		cfg.Pricing.Chains[i].PrimarySource = "coinmarketcap"
		cfg.Pricing.Chains[i].SanitySources = []string{"uniswap", "binance"}
		cfg.Pricing.Chains[i].CoinMarketCapSymbol = "ETH"
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsCoinMarketCapPrimaryWithoutAPIKeyEnv(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	for i := range cfg.Pricing.Chains {
		cfg.Pricing.Chains[i].PrimarySource = "coinmarketcap"
		cfg.Pricing.Chains[i].SanitySources = []string{"uniswap", "binance"}
		cfg.Pricing.Chains[i].CoinMarketCapSymbol = "ETH"
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing coinmarketcap api key env error")
	}
}

func TestValidateRejectsCoinMarketCapSanityWithoutAPIKeyEnv(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.Chains[0].CoinMarketCapSymbol = "ETH"
	cfg.Pricing.Chains[0].SanitySources = []string{"uniswap", "coinmarketcap"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing coinmarketcap api key env error")
	}
}

func TestValidateAcceptsCoinGeckoPrimaryPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	for i := range cfg.Pricing.Chains {
		cfg.Pricing.Chains[i].PrimarySource = "coingecko"
		cfg.Pricing.Chains[i].SanitySources = []string{"uniswap", "binance"}
		cfg.Pricing.Chains[i].CoinGeckoID = "ethereum"
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUniswapPrimaryPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.Chains[0].PrimarySource = "uniswap"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported uniswap primary error")
	}
}

func TestLoadStaticIgnoresDatabaseURLEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := []byte(`database_url: postgres://file:file@localhost:5432/file?sslmode=disable
signers:
  - id: "0x9999999999999999999999999999999999999999"
    type: keystore
    keystore:
      path: /run/secrets/executor-keystore.json
      password_env: KEYSTORE_PASSWORD
chains:
  - eid: 40161
    name: ethereum-sepolia
    family: evm
    chain_id: 11155111
    endpoint_address: "0x1111111111111111111111111111111111111111"
    confirmations: 12
    rpc_urls:
      - http://localhost:8545
    tx_roles:
      executor:
        signer: "0x9999999999999999999999999999999999999999"
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
      dvn:
        signer: "0x9999999999999999999999999999999999999999"
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
  - eid: 40449
    name: hoodi
    family: evm
    chain_id: 560048
    endpoint_address: "0x4444444444444444444444444444444444444444"
    confirmations: 12
    rpc_urls:
      - http://localhost:8546
    tx_roles:
      executor:
        signer: "0x9999999999999999999999999999999999999999"
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
      dvn:
        signer: "0x9999999999999999999999999999999999999999"
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
pathways:
  - src_eid: 40161
    dst_eid: 40449
    src_oapp: "0x7777777777777777777777777777777777777777"
    dst_oapp: "0x8888888888888888888888888888888888888888"
    send_lib: "0x9999999999999999999999999999999999999999"
    receive_lib: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    source_workers:
      open_executor: "0x2222222222222222222222222222222222222222"
      open_dvn: "0x3333333333333333333333333333333333333333"
    destination_workers:
      open_dvn: "0x6666666666666666666666666666666666666666"
    enabled: true
    max_message_size: 10000
    min_lz_receive_gas: 100000
    max_lz_receive_gas: 300000
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://env:env@localhost:5432/env?sslmode=disable")

	staticConfig, err := LoadStatic(path)
	if err != nil {
		t.Fatalf("LoadStatic() error = %v", err)
	}
	if staticConfig.DatabaseURL != "postgres://file:file@localhost:5432/file?sslmode=disable" {
		t.Fatalf("LoadStatic() database_url = %q, want file value", staticConfig.DatabaseURL)
	}
	if staticConfig.Chains[0].StartBlockNumber != 0 {
		t.Fatalf("LoadStatic() start_block_number = %d, want default 0", staticConfig.Chains[0].StartBlockNumber)
	}
	if staticConfig.Chains[0].IndexerQueryBlockRange != 500 {
		t.Fatalf("LoadStatic() indexer_query_block_range = %d, want default 500", staticConfig.Chains[0].IndexerQueryBlockRange)
	}
	if staticConfig.Pathways[0].DVN.Mode != DVNModeShadow {
		t.Fatalf("LoadStatic() pathway dvn mode = %q, want %q", staticConfig.Pathways[0].DVN.Mode, DVNModeShadow)
	}
	if !staticConfig.ExecutorEnabled() || !staticConfig.DVNEnabled() {
		t.Fatal("LoadStatic() omitted services should default executor and dvn to enabled")
	}
	workerConfig, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if workerConfig.DatabaseURL != "postgres://env:env@localhost:5432/env?sslmode=disable" {
		t.Fatalf("Load() database_url = %q, want env override", workerConfig.DatabaseURL)
	}
}

func validConfig() Config {
	return Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Signers: []SignerConfig{
			{
				ID:   MustEVMAddress("0x9999999999999999999999999999999999999999"),
				Type: "keystore",
				Keystore: KeystoreSignerConfig{
					Path:        "/run/secrets/executor-keystore.json",
					PasswordEnv: "KEYSTORE_PASSWORD",
				},
			},
		},
		Chains: []ChainConfig{
			{
				EID:                    40161,
				Name:                   "ethereum-sepolia",
				Family:                 ChainFamilyEVM,
				ChainID:                11155111,
				EndpointAddress:        MustEVMAddress("0x1111111111111111111111111111111111111111"),
				Confirmations:          12,
				IndexerQueryBlockRange: 500,
				RPCURLs:                []string{"http://localhost:8545"},
				TxRoles: ChainTxRolesConfig{
					Executor: validExecutorTxRoleConfig(),
					DVN:      validDVNTxRoleConfig(),
				},
			},
			{
				EID:                    40449,
				Name:                   "hoodi",
				Family:                 ChainFamilyEVM,
				ChainID:                560048,
				EndpointAddress:        MustEVMAddress("0x4444444444444444444444444444444444444444"),
				Confirmations:          12,
				IndexerQueryBlockRange: 500,
				RPCURLs:                []string{"http://localhost:8546"},
				TxRoles: ChainTxRolesConfig{
					Executor: validExecutorTxRoleConfig(),
					DVN:      validDVNTxRoleConfig(),
				},
			},
		},
		Pathways: []PathwayConfig{
			{
				SrcEID:     40161,
				DstEID:     40449,
				SrcOApp:    MustEVMAddress("0x7777777777777777777777777777777777777777"),
				DstOApp:    MustEVMAddress("0x8888888888888888888888888888888888888888"),
				SendLib:    MustEVMAddress("0x9999999999999999999999999999999999999999"),
				ReceiveLib: MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				SourceWorkers: WorkerContractsConfig{
					OpenExecutor: MustEVMAddress("0x2222222222222222222222222222222222222222"),
					OpenDVN:      MustEVMAddress("0x3333333333333333333333333333333333333333"),
				},
				DestinationWorkers: DestinationWorkerContractsConfig{
					OpenDVN: MustEVMAddress("0x6666666666666666666666666666666666666666"),
				},
				DVN:             PathwayDVNConfig{Mode: DVNModeShadow},
				Pricing:         validPathwayPricingConfig(),
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
			{
				SrcEID:     40449,
				DstEID:     40161,
				SrcOApp:    MustEVMAddress("0x8888888888888888888888888888888888888888"),
				DstOApp:    MustEVMAddress("0x7777777777777777777777777777777777777777"),
				SendLib:    MustEVMAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				ReceiveLib: MustEVMAddress("0xcccccccccccccccccccccccccccccccccccccccc"),
				SourceWorkers: WorkerContractsConfig{
					OpenExecutor: MustEVMAddress("0x5555555555555555555555555555555555555555"),
					OpenDVN:      MustEVMAddress("0x6666666666666666666666666666666666666666"),
				},
				DestinationWorkers: DestinationWorkerContractsConfig{
					OpenDVN: MustEVMAddress("0x3333333333333333333333333333333333333333"),
				},
				DVN:             PathwayDVNConfig{Mode: DVNModeShadow},
				Pricing:         validPathwayPricingConfig(),
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
		},
	}
}

func validExecutorTxRoleConfig() ExecutorTxRoleConfig {
	return ExecutorTxRoleConfig{
		Signer:                  MustEVMAddress("0x9999999999999999999999999999999999999999"),
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

func validDVNTxRoleConfig() DVNTxRoleConfig {
	return DVNTxRoleConfig{
		Signer:                  MustEVMAddress("0x9999999999999999999999999999999999999999"),
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

func validPricingConfig() PricingConfig {
	return PricingConfig{
		Enabled:                 true,
		Signer:                  MustEVMAddress("0x9999999999999999999999999999999999999999"),
		IntervalSeconds:         300,
		StaleAfterSeconds:       1800,
		MaxDeviationBps:         500,
		GasSpikeBps:             1000,
		AllowSanityFallback:     true,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
		Chains: []PricingChainConfig{
			{
				EID:           40161,
				PrimarySource: "binance",
				SanitySources: []string{"uniswap"},
				BinanceSymbol: "ETHUSDT",
				Uniswap: UniswapPricingConfig{
					QuoterAddress:    MustEVMAddress("0x1111111111111111111111111111111111111111"),
					TokenIn:          MustEVMAddress("0x2222222222222222222222222222222222222222"),
					TokenOut:         MustEVMAddress("0x3333333333333333333333333333333333333333"),
					Fee:              500,
					AmountInWei:      "1000000000000000000",
					TokenOutDecimals: 6,
				},
			},
			{
				EID:           40449,
				PrimarySource: "binance",
				SanitySources: []string{"uniswap"},
				BinanceSymbol: "ETHUSDT",
				Uniswap: UniswapPricingConfig{
					QuoterAddress:    MustEVMAddress("0x4444444444444444444444444444444444444444"),
					TokenIn:          MustEVMAddress("0x5555555555555555555555555555555555555555"),
					TokenOut:         MustEVMAddress("0x6666666666666666666666666666666666666666"),
					Fee:              500,
					AmountInWei:      "1000000000000000000",
					TokenOutDecimals: 6,
				},
			},
		},
	}
}

func validPathwayPricingConfig() PathwayPricingConfig {
	return PathwayPricingConfig{
		ExecutorFee: WorkerFeeModelConfig{FixedFeeWei: "1000", DstGasOverhead: 50000, MarginBps: 100},
		DVNFee:      WorkerFeeModelConfig{FixedFeeWei: "2000", DstGasOverhead: 150000, MarginBps: 200},
	}
}
