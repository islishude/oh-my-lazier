package config

import "testing"

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

func TestValidateRejectsDuplicatePathway(t *testing.T) {
	cfg := validConfig()
	cfg.Pathways = append(cfg.Pathways, cfg.Pathways[0])
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate pathway error")
	}
}

func validConfig() Config {
	return Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Executor:    ExecutorConfig{Signer: "0x9999999999999999999999999999999999999999"},
		DVN:         DVNConfig{Mode: "shadow"},
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
