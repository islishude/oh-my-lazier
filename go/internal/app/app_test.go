package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/configcheck"
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
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
	worker, err := New(cfg, discardLogger())
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
	wantTxTypes := map[uint32]string{40161: config.TxTypeDynamicFee, 40245: config.TxTypeLegacy}
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
		if target.TxType != wantTxTypes[target.ChainEID] {
			t.Fatalf("target tx type for chain %d = %q, want %q", target.ChainEID, target.TxType, wantTxTypes[target.ChainEID])
		}
	}
}

func TestNewRejectsNilLogger(t *testing.T) {
	_, err := New(testConfig("0x9999999999999999999999999999999999999999", "/unused/keystore.json"), nil)
	if err == nil {
		t.Fatal("New() error = nil, want logger error")
	}
	if !strings.Contains(err.Error(), "logger is required") {
		t.Fatalf("New() error = %v, want logger error", err)
	}
}

func TestRunPriceOnceRejectsDisabledPricing(t *testing.T) {
	worker, err := New(testConfig("0x9999999999999999999999999999999999999999", "/unused/keystore.json"), discardLogger())
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

func TestRunPriceOnceChecksOnChainConfigBeforeDatabaseSync(t *testing.T) {
	originalCheck := checkOnChainConfig
	defer func() { checkOnChainConfig = originalCheck }()
	checkOnChainConfig = func(_ context.Context, _ *chain.Registry) (configcheck.Report, error) {
		return configcheck.Report{
			Issues: []configcheck.Issue{{Path: "chains[40161].chain_id", Message: "wrong"}},
		}, nil
	}

	cfg := testConfig("0x9999999999999999999999999999999999999999", "/unused/keystore.json")
	cfg.DatabaseURL = "postgres://invalid:invalid@127.0.0.1:1/db?sslmode=disable"
	cfg.Pricing = testPricingConfig()
	worker, err := New(cfg, discardLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = worker.RunPriceOnce(t.Context())
	if err == nil {
		t.Fatal("RunPriceOnce() error = nil, want on-chain config error")
	}
	if !strings.Contains(err.Error(), "on-chain config check failed") {
		t.Fatalf("RunPriceOnce() error = %v, want on-chain config error", err)
	}
}

func TestLoadKMSAWSConfigUsesDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_SESSION_TOKEN", "test-session-token")

	awsConfig, err := loadKMSAWSConfig(t.Context(), config.KMSSignerConfig{
		Region: "us-west-2",
	})
	if err != nil {
		t.Fatalf("loadKMSAWSConfig() error = %v", err)
	}
	if awsConfig.Region != "us-west-2" {
		t.Fatalf("Region = %q, want us-west-2", awsConfig.Region)
	}
	credentials, err := awsConfig.Credentials.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("Credentials.Retrieve() error = %v", err)
	}
	if credentials.AccessKeyID != "test-access-key" {
		t.Fatal("AccessKeyID did not match default credential chain env value")
	}
	if credentials.SecretAccessKey != "test-secret-key" {
		t.Fatal("SecretAccessKey did not match default credential chain env value")
	}
	if credentials.SessionToken != "test-session-token" {
		t.Fatal("SessionToken did not match default credential chain env value")
	}
}

func TestSuperviseLoopRestartsReturnedErrorsUntilContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan int, 2)
	done := make(chan struct{})
	errCh := make(chan error, 1)
	retries := &recordingLoopRetries{}
	go func() {
		defer close(done)
		attempts := 0
		errCh <- superviseLoop(ctx, "test", 0, discardLogger(), retries, func(context.Context) error {
			attempts++
			calls <- attempts
			if attempts == 2 {
				cancel()
				return context.Canceled
			}
			return errors.New("temporary loop error")
		})
	}()

	for want := 1; want <= 2; want++ {
		select {
		case got := <-calls:
			if got != want {
				t.Fatalf("attempt = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("attempt %d did not run", want)
		}
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("superviseLoop did not stop after context cancellation")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("superviseLoop() error = %v, want nil", err)
	}
	if len(retries.names) != 1 || retries.names[0] != "test" {
		t.Fatalf("retry metrics = %#v, want one test retry", retries.names)
	}
}

func TestSuperviseLoopReturnsFatalErrorWithoutRetry(t *testing.T) {
	wantErr := errors.New("bad loop configuration")
	retries := &recordingLoopRetries{}
	calls := 0

	err := superviseLoop(context.Background(), "test", 0, discardLogger(), retries, func(context.Context) error {
		calls++
		return workerloop.Fatal(wantErr)
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("superviseLoop() error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if len(retries.names) != 0 {
		t.Fatalf("retry metrics = %#v, want none", retries.names)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recordingLoopRetries struct {
	names []string
}

func (r *recordingLoopRetries) RecordLoopRetry(name string) {
	r.names = append(r.names, name)
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
				TxType:          config.TxTypeLegacy,
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
				SrcEID:          40161,
				DstEID:          40245,
				SrcOApp:         "0x7777777777777777777777777777777777777777",
				DstOApp:         "0x8888888888888888888888888888888888888888",
				SendLib:         "0x9999999999999999999999999999999999999999",
				ReceiveLib:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
		},
	}
}

func testPricingConfig() config.PricingConfig {
	return config.PricingConfig{
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
		Chains: []config.PricingChainConfig{
			{EID: 40161, BinanceSymbol: "ETHUSDT"},
			{EID: 40245, BinanceSymbol: "ETHUSDT"},
		},
	}
}
