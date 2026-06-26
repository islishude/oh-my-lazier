package configdiff

import (
	"strings"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestDiffUsesSemanticKeysForLists(t *testing.T) {
	before := validConfig()
	after := validConfig()
	after.Chains[1].Workers.OpenExecutor = "0x7777777777777777777777777777777777777777"
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
		"chains[40245]",
		"pathways[40161:40245:0x7777777777777777777777777777777777777777:0x8888888888888888888888888888888888888888]",
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
	after.DVN.Mode = "active"

	output := RenderText(Diff(before, after))

	if !strings.Contains(output, "dvn\n") {
		t.Fatalf("output missing dvn path:\n%s", output)
	}
	if !strings.Contains(output, `"Mode":"shadow"`) || !strings.Contains(output, `"Mode":"active"`) {
		t.Fatalf("output missing before/after values:\n%s", output)
	}
}

func validConfig() config.Config {
	return config.Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Metrics:     config.MetricsConfig{ListenAddress: ":9090"},
		Executor:    config.ExecutorConfig{Signer: "0x9999999999999999999999999999999999999999"},
		DVN:         config.DVNConfig{Mode: "shadow"},
		Pricing: config.PricingConfig{
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
			Chains: []config.PricingChainConfig{
				{
					EID:           40161,
					BinanceSymbol: "ETHUSDT",
					Uniswap: config.UniswapPricingConfig{
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
					Uniswap: config.UniswapPricingConfig{
						QuoterAddress:    "0x4444444444444444444444444444444444444444",
						TokenIn:          "0x5555555555555555555555555555555555555555",
						TokenOut:         "0x6666666666666666666666666666666666666666",
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
				ChainID:         11155111,
				EndpointAddress: "0x1111111111111111111111111111111111111111",
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8545"},
				Workers: config.WorkerContractsConfig{
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
				Workers: config.WorkerContractsConfig{
					OpenExecutor: "0x5555555555555555555555555555555555555555",
					OpenDVN:      "0x6666666666666666666666666666666666666666",
				},
			},
		},
		Pathways: []config.PathwayConfig{
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

func validPricingConfig() config.PricingConfig {
	cfg := validConfig()
	return cfg.Pricing
}
