package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAcceptsSepoliaPathways(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMissingWorkerContractAddress(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].SourceWorkers.OpenDVN = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing worker contract error")
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

func TestValidateRejectsNonPhaseOneConfirmations(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].Confirmations = 6
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want phase-1 confirmations error")
	}
}

func TestValidateAcceptsSupportedChainTxTypes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		txType string
	}{
		{name: "default empty", txType: ""},
		{name: "dynamic fee", txType: TxTypeDynamicFee},
		{name: "legacy", txType: TxTypeLegacy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Chains[0].TxType = tt.txType
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidChainTxType(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].TxType = "blob"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid tx_type error")
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
	cfg.Chains[0].TxRoles.Executor.Signer = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing executor signer error")
	}
}

func TestValidateRejectsUnknownExecutorSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Chains[0].TxRoles.Executor.Signer = "0x1111111111111111111111111111111111111111"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown executor signer error")
	}
}

func TestValidateAcceptsActiveDVNConfig(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsActiveDVNWithoutSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	cfg.Chains[1].TxRoles.DVN.Signer = ""
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

func TestValidateAcceptsLegacyActiveDVNWithoutDynamicFeeCaps(t *testing.T) {
	cfg := validConfig()
	for i := range cfg.Chains {
		cfg.Chains[i].TxType = TxTypeLegacy
	}
	cfg.Pathways[0].DVN.Mode = DVNModeActive
	cfg.Chains[1].TxRoles.DVN.MaxFeePerGasWei = ""
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
		ID:   "0x9999999999999999999999999999999999999999",
		Type: "kms",
		KMS: KMSSignerConfig{
			KeyID:   "test-key",
			Region:  "us-east-1",
			Address: "0x1111111111111111111111111111111111111111",
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

func TestValidateRejectsIncompletePricingChains(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.Chains = cfg.Pricing.Chains[:1]
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want incomplete pricing chains error")
	}
}

func TestValidateAcceptsLegacyPricingWithoutDynamicFeeCaps(t *testing.T) {
	cfg := validConfig()
	for i := range cfg.Chains {
		cfg.Chains[i].TxType = TxTypeLegacy
	}
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.MaxFeePerGasWei = ""
	cfg.Pricing.MaxPriorityFeePerGasWei = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateAcceptsCoinMarketCapPrimaryPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.PrimarySource = "coinmarketcap"
	cfg.Pricing.CoinMarketCapAPIKeyEnv = "COINMARKETCAP_API_KEY"
	for i := range cfg.Pricing.Chains {
		cfg.Pricing.Chains[i].CoinMarketCapSymbol = "ETH"
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsCoinMarketCapPrimaryWithoutAPIKeyEnv(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.PrimarySource = "coinmarketcap"
	for i := range cfg.Pricing.Chains {
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
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing coinmarketcap api key env error")
	}
}

func TestValidateAcceptsCoinGeckoPrimaryPricing(t *testing.T) {
	cfg := validConfig()
	cfg.Pricing = validPricingConfig()
	cfg.Pricing.PrimarySource = "coingecko"
	for i := range cfg.Pricing.Chains {
		cfg.Pricing.Chains[i].CoinGeckoID = "ethereum"
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
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
    chain_id: 11155111
    endpoint_address: "0x1111111111111111111111111111111111111111"
    confirmations: 12
    rpc_urls:
      - http://localhost:8545
    tx_roles:
      executor:
        signer: "0x9999999999999999999999999999999999999999"
      dvn:
        signer: "0x9999999999999999999999999999999999999999"
        tx_gas_limit: 120000
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
  - eid: 40245
    name: base-sepolia
    chain_id: 84532
    endpoint_address: "0x4444444444444444444444444444444444444444"
    confirmations: 12
    rpc_urls:
      - http://localhost:8546
    tx_roles:
      executor:
        signer: "0x9999999999999999999999999999999999999999"
      dvn:
        signer: "0x9999999999999999999999999999999999999999"
        tx_gas_limit: 120000
        max_fee_per_gas_wei: "2000000000"
        max_priority_fee_per_gas_wei: "1000000000"
pathways:
  - src_eid: 40161
    dst_eid: 40245
    src_oapp: "0x7777777777777777777777777777777777777777"
    dst_oapp: "0x8888888888888888888888888888888888888888"
    send_lib: "0x9999999999999999999999999999999999999999"
    receive_lib: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    source_workers:
      open_executor: "0x2222222222222222222222222222222222222222"
      open_dvn: "0x3333333333333333333333333333333333333333"
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
	if staticConfig.Chains[0].TxType != TxTypeDynamicFee {
		t.Fatalf("LoadStatic() tx_type = %q, want %q", staticConfig.Chains[0].TxType, TxTypeDynamicFee)
	}
	if staticConfig.Pathways[0].DVN.Mode != DVNModeShadow {
		t.Fatalf("LoadStatic() pathway dvn mode = %q, want %q", staticConfig.Pathways[0].DVN.Mode, DVNModeShadow)
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
				ID:   "0x9999999999999999999999999999999999999999",
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
				ChainID:                11155111,
				EndpointAddress:        "0x1111111111111111111111111111111111111111",
				Confirmations:          12,
				IndexerQueryBlockRange: 500,
				RPCURLs:                []string{"http://localhost:8545"},
				TxRoles: ChainTxRolesConfig{
					Executor: ExecutorTxRoleConfig{Signer: "0x9999999999999999999999999999999999999999"},
					DVN:      validDVNTxRoleConfig(),
				},
			},
			{
				EID:                    40245,
				Name:                   "base-sepolia",
				ChainID:                84532,
				EndpointAddress:        "0x4444444444444444444444444444444444444444",
				Confirmations:          12,
				IndexerQueryBlockRange: 500,
				RPCURLs:                []string{"http://localhost:8546"},
				TxRoles: ChainTxRolesConfig{
					Executor: ExecutorTxRoleConfig{Signer: "0x9999999999999999999999999999999999999999"},
					DVN:      validDVNTxRoleConfig(),
				},
			},
		},
		Pathways: []PathwayConfig{
			{
				SrcEID:     40161,
				DstEID:     40245,
				SrcOApp:    "0x7777777777777777777777777777777777777777",
				DstOApp:    "0x8888888888888888888888888888888888888888",
				SendLib:    "0x9999999999999999999999999999999999999999",
				ReceiveLib: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SourceWorkers: WorkerContractsConfig{
					OpenExecutor: "0x2222222222222222222222222222222222222222",
					OpenDVN:      "0x3333333333333333333333333333333333333333",
				},
				DVN:             PathwayDVNConfig{Mode: DVNModeShadow},
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
			{
				SrcEID:     40245,
				DstEID:     40161,
				SrcOApp:    "0x8888888888888888888888888888888888888888",
				DstOApp:    "0x7777777777777777777777777777777777777777",
				SendLib:    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				ReceiveLib: "0xcccccccccccccccccccccccccccccccccccccccc",
				SourceWorkers: WorkerContractsConfig{
					OpenExecutor: "0x5555555555555555555555555555555555555555",
					OpenDVN:      "0x6666666666666666666666666666666666666666",
				},
				DVN:             PathwayDVNConfig{Mode: DVNModeShadow},
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
		},
	}
}

func validDVNTxRoleConfig() DVNTxRoleConfig {
	return DVNTxRoleConfig{
		Signer:                  "0x9999999999999999999999999999999999999999",
		TxGasLimit:              120000,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

func validPricingConfig() PricingConfig {
	return PricingConfig{
		Enabled:                 true,
		Signer:                  "0x9999999999999999999999999999999999999999",
		IntervalSeconds:         300,
		BaseFeeWei:              "1000",
		BufferBps:               100,
		StaleAfterSeconds:       1800,
		MaxDeviationBps:         500,
		GasSpikeBps:             1000,
		AllowUniswapFallback:    true,
		TxGasLimit:              100000,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
		Chains: []PricingChainConfig{
			{
				EID:           40161,
				BinanceSymbol: "ETHUSDT",
				Uniswap: UniswapPricingConfig{
					QuoterAddress:    "0x1111111111111111111111111111111111111111",
					TokenIn:          "0x2222222222222222222222222222222222222222",
					TokenOut:         "0x3333333333333333333333333333333333333333",
					Fee:              500,
					AmountInWei:      "1000000000000000000",
					TokenOutDecimals: 6,
				},
			},
			{
				EID:           40245,
				BinanceSymbol: "ETHUSDT",
				Uniswap: UniswapPricingConfig{
					QuoterAddress:    "0x4444444444444444444444444444444444444444",
					TokenIn:          "0x5555555555555555555555555555555555555555",
					TokenOut:         "0x6666666666666666666666666666666666666666",
					Fee:              500,
					AmountInWei:      "1000000000000000000",
					TokenOutDecimals: 6,
				},
			},
		},
	}
}
