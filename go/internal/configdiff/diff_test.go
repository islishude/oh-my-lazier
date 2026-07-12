package configdiff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestDiffUsesSemanticKeysForLists(t *testing.T) {
	before := validConfig()
	after := validConfig()
	after.Services.Executor.Enabled = new(false)
	after.Pathways[1].SourceWorkers.OpenExecutor = config.MustEVMAddress("0x7777777777777777777777777777777777777777")
	after.Pathways[0].MaxMessageSize = 20000
	after.Pricing = validPricingConfig()
	after.Pricing.Chains[1].Uniswap.Fee = 3000

	changes := Diff(before, after)

	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}
	want := []string{
		"services",
		"pricing.chains[40449]",
		"pathways[40161:40449:0x7777777777777777777777777777777777777777:0x8888888888888888888888888888888888888888]",
		"pathways[40449:40161:0x8888888888888888888888888888888888888888:0x7777777777777777777777777777777777777777]",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestRenderTextReportsNoConfigChanges(t *testing.T) {
	output := RenderText(nil)
	if output != "no config changes\n" {
		t.Fatalf("RenderText(nil) = %q", output)
	}
}

func TestRenderTextIncludesChangedPath(t *testing.T) {
	before := validConfig()
	after := validConfig()
	after.Pathways[0].DVN.Mode = config.DVNModeActive

	output := RenderText(Diff(before, after))

	if !strings.Contains(output, "pathways[40161:40449:0x7777777777777777777777777777777777777777:0x8888888888888888888888888888888888888888]\n") {
		t.Fatalf("output missing pathway path:\n%s", output)
	}
	if !strings.Contains(output, `"Mode":"shadow"`) || !strings.Contains(output, `"Mode":"active"`) {
		t.Fatalf("output missing before/after values:\n%s", output)
	}
}

func TestDiffRedactsDatabaseAndRPCURLCredentials(t *testing.T) {
	before := validConfig()
	after := validConfig()
	before.DatabaseURL = "postgres://worker:before-password@db.internal:5432/worker?sslmode=disable&password=before-query-password&api_key=before-db-key"
	after.DatabaseURL = "postgres://worker:after-password@db.internal:5432/worker?sslmode=require&password=after-query-password&api_key=after-db-key"
	before.Chains[0].RPCURLs = []string{"https://before-user:before-password@before-secret.example/v2/before-path-key?api_key=before-query-key"}
	after.Chains[0].RPCURLs = []string{"https://after-user:after-password@after-secret.example/v2/after-path-key?api_key=after-query-key"}

	changes := Diff(before, after)
	encoded, err := json.Marshal(changes)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	output := string(encoded) + RenderText(changes)
	for _, secret := range []string{
		"before-password",
		"after-password",
		"before-user",
		"after-user",
		"before-secret",
		"after-secret",
		"before-path-key",
		"after-path-key",
		"before-query-key",
		"after-query-key",
		"before-query-password",
		"after-query-password",
		"before-db-key",
		"after-db-key",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("config diff leaked %q:\n%s", secret, output)
		}
	}
	if !strings.Contains(output, `"path":"database_url"`) || !strings.Contains(output, `"path":"chains[40161]"`) {
		t.Fatalf("config diff omitted redacted changes:\n%s", output)
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("config diff missing redaction marker:\n%s", output)
	}
	if !strings.Contains(output, "sslmode=disable") || !strings.Contains(output, "sslmode=require") {
		t.Fatalf("config diff omitted security-relevant database parameters:\n%s", output)
	}
}

func validConfig() config.Config {
	return config.Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Metrics:     config.MetricsConfig{ListenAddress: ":9090"},
		Pricing: config.PricingConfig{
			Enabled:                 true,
			Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
			IntervalSeconds:         300,
			StaleAfterSeconds:       1800,
			MaxDeviationBps:         500,
			GasSpikeBps:             1000,
			AllowSanityFallback:     true,
			MaxFeePerGasWei:         "2000000000",
			MaxPriorityFeePerGasWei: "1000000000",
			MinNativeBalanceWei:     "100000000000000000",
			Chains: []config.PricingChainConfig{
				{
					EID:               40161,
					NativeAssetID:     "eth",
					DataFeePerByteWei: "0",
					PrimarySource:     "binance",
					SanitySources:     []string{"uniswap"},
					BinanceSymbol:     "ETHUSDT",
					Uniswap: config.UniswapPricingConfig{
						QuoterAddress:    config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
						TokenIn:          config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
						TokenOut:         config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
						Fee:              500,
						AmountInWei:      "1000000000000000000",
						TokenOutDecimals: 6,
					},
				},
				{
					EID:               40449,
					NativeAssetID:     "hoodi-eth",
					DataFeePerByteWei: "0",
					PrimarySource:     "binance",
					SanitySources:     []string{"uniswap"},
					BinanceSymbol:     "ETHUSDT",
					Uniswap: config.UniswapPricingConfig{
						QuoterAddress:    config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
						TokenIn:          config.MustEVMAddress("0x5555555555555555555555555555555555555555"),
						TokenOut:         config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
						Fee:              500,
						AmountInWei:      "1000000000000000000",
						TokenOutDecimals: 6,
					},
				},
			},
		},
		Chains: []config.ChainConfig{
			{
				EID:             40161,
				Name:            "ethereum-sepolia",
				Family:          config.ChainFamilyEVM,
				ChainID:         11155111,
				EndpointAddress: config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8545"},
				TxRoles: config.ChainTxRolesConfig{
					Executor: testExecutorRole(),
				},
			},
			{
				EID:             40449,
				Name:            "hoodi",
				Family:          config.ChainFamilyEVM,
				ChainID:         560048,
				EndpointAddress: config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8546"},
				TxRoles: config.ChainTxRolesConfig{
					Executor: testExecutorRole(),
				},
			},
		},
		Pathways: []config.PathwayConfig{
			{
				SrcEID:     40161,
				DstEID:     40449,
				SrcOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
				DstOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
				SendLib:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
				ReceiveLib: config.MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				SourceWorkers: config.WorkerContractsConfig{
					OpenExecutor: config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
					OpenDVN:      config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
					PriceFeed:    config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
				},
				DestinationWorkers: config.DestinationWorkerContractsConfig{
					OpenDVN: config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
				},
				DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
				Pricing:        testPathwayPricingConfig(),
				Enabled:        true,
				MaxMessageSize: 10000,
			},
			{
				SrcEID:     40449,
				DstEID:     40161,
				SrcOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
				DstOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
				SendLib:    config.MustEVMAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				ReceiveLib: config.MustEVMAddress("0xcccccccccccccccccccccccccccccccccccccccc"),
				SourceWorkers: config.WorkerContractsConfig{
					OpenExecutor: config.MustEVMAddress("0x5555555555555555555555555555555555555555"),
					OpenDVN:      config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
					PriceFeed:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
				},
				DestinationWorkers: config.DestinationWorkerContractsConfig{
					OpenDVN: config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
				},
				DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
				Pricing:        testPathwayPricingConfig(),
				Enabled:        true,
				MaxMessageSize: 10000,
			},
		},
	}
}

func testExecutorRole() config.ExecutorTxRoleConfig {
	return config.ExecutorTxRoleConfig{
		Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
		MinNativeBalanceWei:     "100000000000000000",
	}
}

func validPricingConfig() config.PricingConfig {
	cfg := validConfig()
	return cfg.Pricing
}

func testPathwayPricingConfig() config.PathwayPricingConfig {
	return config.PathwayPricingConfig{
		ExecutorFee: config.WorkerFeeModelConfig{FixedFeeWei: "1000", DstGasOverhead: 50000, DataSizeOverheadBytes: new(uint64(0)), MarginBps: 100},
		DVNFee:      config.WorkerFeeModelConfig{FixedFeeWei: "2000", DstGasOverhead: 150000, DataSizeOverheadBytes: new(uint64(0)), MarginBps: 200},
	}
}
