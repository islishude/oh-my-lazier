package app

import (
	"path/filepath"
	"strings"
	"testing"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestTxTargetsLoadsKeystoreSignerForEveryChain(t *testing.T) {
	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}
	t.Setenv("KEYSTORE_PASSWORD", password)

	cfg := testConfig(account.Address.Hex(), filepath.Clean(account.URL.Path))
	registry, err := chain.NewRegistry(cfg.Chains, cfg.Pathways)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	worker, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	targets, err := worker.txTargets(t.Context(), registry)
	if err != nil {
		t.Fatalf("txTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want one per chain", len(targets))
	}
	for _, target := range targets {
		if target.Signer.Address() != account.Address {
			t.Fatalf("target signer = %s, want %s", target.Signer.Address(), account.Address)
		}
		if target.ChainID == nil || target.ChainID.Sign() <= 0 {
			t.Fatalf("target chain id = %v, want positive", target.ChainID)
		}
		if target.Client == nil {
			t.Fatal("target client is nil")
		}
	}
}

func TestRunPriceOnceRejectsDisabledPricing(t *testing.T) {
	worker, err := New(testConfig("0x9999999999999999999999999999999999999999", "/unused/keystore.json"), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = worker.RunPriceOnce(t.Context())
	if err == nil {
		t.Fatal("RunPriceOnce() error = nil, want disabled pricing error")
	}
	if !strings.Contains(err.Error(), "pricing is disabled") {
		t.Fatalf("RunPriceOnce() error = %v, want disabled pricing error", err)
	}
}

func testConfig(signerID, keystorePath string) config.Config {
	return config.Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Executor:    config.ExecutorConfig{Signer: signerID},
		DVN:         config.DVNConfig{Mode: "shadow"},
		Signers: []config.SignerConfig{
			{
				ID:   signerID,
				Type: "keystore",
				Keystore: config.KeystoreSignerConfig{
					Path:        keystorePath,
					PasswordEnv: "KEYSTORE_PASSWORD",
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
		},
	}
}
