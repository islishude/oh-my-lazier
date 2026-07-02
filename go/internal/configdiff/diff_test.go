package configdiff

import (
	"strings"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestDiffUsesSemanticKeysForLists(t *testing.T) {
	before := validConfig()
	after := validConfig()
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
		"pricing.chains[40245]",
		"pathways[40161:40245:0x7777777777777777777777777777777777777777:0x8888888888888888888888888888888888888888]",
		"pathways[40245:40161:0x8888888888888888888888888888888888888888:0x7777777777777777777777777777777777777777]",
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

	if !strings.Contains(output, "pathways[40161:40245:0x7777777777777777777777777777777777777777:0x8888888888888888888888888888888888888888]\n") {
		t.Fatalf("output missing pathway path:\n%s", output)
	}
	if !strings.Contains(output, `"Mode":"shadow"`) || !strings.Contains(output, `"Mode":"active"`) {
		t.Fatalf("output missing before/after values:\n%s", output)
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
			BaseFeeWei:              "1000",
			BufferBps:               100,
			StaleAfterSeconds:       1800,
			MaxDeviationBps:         500,
			GasSpikeBps:             1000,
			AllowUniswapFallback:    true,
			TxGasLimit:              100000,
			MaxFeePerGasWei:         "2000000000",
			MaxPriorityFeePerGasWei: "1000000000",
			Chains: []config.PricingChainConfig{
				{
					EID:           40161,
					BinanceSymbol: "ETHUSDT",
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
					EID:           40245,
					BinanceSymbol: "ETHUSDT",
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
					Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
				},
			},
			{
				EID:             40245,
				Name:            "base-sepolia",
				Family:          config.ChainFamilyEVM,
				ChainID:         84532,
				EndpointAddress: config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8546"},
				TxRoles: config.ChainTxRolesConfig{
					Executor: config.ExecutorTxRoleConfig{Signer: config.MustEVMAddress("0x9999999999999999999999999999999999999999")},
				},
			},
		},
		Pathways: []config.PathwayConfig{
			{
				SrcEID:     40161,
				DstEID:     40245,
				SrcOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
				DstOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
				SendLib:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
				ReceiveLib: config.MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				SourceWorkers: config.WorkerContractsConfig{
					OpenExecutor: config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
					OpenDVN:      config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
				},
				DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
				Enabled:        true,
				MaxMessageSize: 10000,
			},
			{
				SrcEID:     40245,
				DstEID:     40161,
				SrcOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
				DstOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
				SendLib:    config.MustEVMAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				ReceiveLib: config.MustEVMAddress("0xcccccccccccccccccccccccccccccccccccccccc"),
				SourceWorkers: config.WorkerContractsConfig{
					OpenExecutor: config.MustEVMAddress("0x5555555555555555555555555555555555555555"),
					OpenDVN:      config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
				},
				DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
				Enabled:        true,
				MaxMessageSize: 10000,
			},
		},
	}
}

func validPricingConfig() config.PricingConfig {
	cfg := validConfig()
	return cfg.Pricing
}
