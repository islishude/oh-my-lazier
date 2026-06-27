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
	cfg.Chains[0].Workers.OpenDVN = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing worker contract error")
	}
}

func TestValidateRejectsMissingExecutorSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Executor.Signer = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing executor signer error")
	}
}

func TestValidateRejectsUnknownExecutorSigner(t *testing.T) {
	cfg := validConfig()
	cfg.Executor.Signer = "0x1111111111111111111111111111111111111111"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown executor signer error")
	}
}

func TestValidateAcceptsActiveDVNConfig(t *testing.T) {
	cfg := validConfig()
	cfg.DVN = validActiveDVNConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsActiveDVNWithoutSigner(t *testing.T) {
	cfg := validConfig()
	cfg.DVN = validActiveDVNConfig()
	cfg.DVN.Signer = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing active dvn signer error")
	}
}

func TestValidateRejectsActiveDVNWithoutFees(t *testing.T) {
	cfg := validConfig()
	cfg.DVN = validActiveDVNConfig()
	cfg.DVN.MaxFeePerGasWei = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing active dvn fee error")
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
			KeyID:              "test-key",
			Region:             "us-east-1",
			Address:            "0x1111111111111111111111111111111111111111",
			AccessKeyIDEnv:     "AWS_ACCESS_KEY_ID",
			SecretAccessKeyEnv: "AWS_SECRET_ACCESS_KEY",
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
executor:
  signer: "0x9999999999999999999999999999999999999999"
dvn:
  mode: shadow
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
    workers:
      open_executor: "0x2222222222222222222222222222222222222222"
      open_dvn: "0x3333333333333333333333333333333333333333"
  - eid: 40245
    name: base-sepolia
    chain_id: 84532
    endpoint_address: "0x4444444444444444444444444444444444444444"
    confirmations: 12
    rpc_urls:
      - http://localhost:8546
    workers:
      open_executor: "0x5555555555555555555555555555555555555555"
      open_dvn: "0x6666666666666666666666666666666666666666"
pathways:
  - src_eid: 40161
    dst_eid: 40245
    src_oapp: "0x7777777777777777777777777777777777777777"
    dst_oapp: "0x8888888888888888888888888888888888888888"
    send_lib: "0x9999999999999999999999999999999999999999"
    receive_lib: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    enabled: true
    max_message_size: 10000
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
		Executor:    ExecutorConfig{Signer: "0x9999999999999999999999999999999999999999"},
		DVN:         DVNConfig{Mode: "shadow"},
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
				EID:             40161,
				Name:            "ethereum-sepolia",
				ChainID:         11155111,
				EndpointAddress: "0x1111111111111111111111111111111111111111",
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8545"},
				Workers: WorkerContractsConfig{
					OpenExecutor: "0x2222222222222222222222222222222222222222",
					OpenDVN:      "0x3333333333333333333333333333333333333333",
				},
			},
			{
				EID:             40245,
				Name:            "base-sepolia",
				ChainID:         84532,
				EndpointAddress: "0x4444444444444444444444444444444444444444",
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8546"},
				Workers: WorkerContractsConfig{
					OpenExecutor: "0x5555555555555555555555555555555555555555",
					OpenDVN:      "0x6666666666666666666666666666666666666666",
				},
			},
		},
		Pathways: []PathwayConfig{
			{
				SrcEID:         40161,
				DstEID:         40245,
				SrcOApp:        "0x7777777777777777777777777777777777777777",
				DstOApp:        "0x8888888888888888888888888888888888888888",
				SendLib:        "0x9999999999999999999999999999999999999999",
				ReceiveLib:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Enabled:        true,
				MaxMessageSize: 10000,
			},
			{
				SrcEID:         40245,
				DstEID:         40161,
				SrcOApp:        "0x8888888888888888888888888888888888888888",
				DstOApp:        "0x7777777777777777777777777777777777777777",
				SendLib:        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				ReceiveLib:     "0xcccccccccccccccccccccccccccccccccccccccc",
				Enabled:        true,
				MaxMessageSize: 10000,
			},
		},
	}
}

func validActiveDVNConfig() DVNConfig {
	return DVNConfig{
		Mode:                    "active",
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
