package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/configcheck"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
)

func TestTxTargetsLoadsKeystoreSignerForEveryChain(t *testing.T) {
	stubReadTxHeader(t, &gethtypes.Header{})

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
		if len(target.FeePolicies) != 2 {
			t.Fatalf("target fee policies for chain %d = %d, want 2", target.ChainEID, len(target.FeePolicies))
		}
	}
}

func TestTxTargetsRejectsDynamicFeeChainWithoutPriorityCap(t *testing.T) {
	stubReadTxHeader(t, &gethtypes.Header{BaseFee: bigOne()})

	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}
	t.Setenv("KEYSTORE_PASSWORD", password)

	cfg := testConfig(account.Address.Hex(), filepath.Clean(account.URL.Path))
	cfg.Chains[0].TxRoles.Executor.MaxPriorityFeePerGasWei = ""
	registry, err := chain.NewRegistry(cfg.Chains, cfg.Pathways)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	worker, err := New(cfg, discardLogger())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = worker.txTargets(t.Context(), registry)
	if err == nil {
		t.Fatal("txTargets() error = nil, want dynamic priority cap error")
	}
	if !strings.Contains(err.Error(), "max_priority_fee_per_gas_wei is required") {
		t.Fatalf("txTargets() error = %v, want priority cap error", err)
	}
}

func TestTxTargetsSelectsTargetsForEnabledRoles(t *testing.T) {
	stubReadTxHeader(t, &gethtypes.Header{})

	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}
	t.Setenv("KEYSTORE_PASSWORD", password)

	tests := []struct {
		name         string
		mutate       func(*config.Config)
		wantPurposes map[uint32][]string
	}{
		{
			name: "executor only by default",
			wantPurposes: map[uint32][]string{
				40161: {"executor_commit_verification", "executor_lz_receive"},
				40449: {"executor_commit_verification", "executor_lz_receive"},
			},
		},
		{
			name: "dvn only active",
			mutate: func(cfg *config.Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Pathways[0].DVN.Mode = config.DVNModeActive
				cfg.Chains[1].TxRoles.DVN = testDVNRole(config.MustEVMAddress(account.Address.Hex()))
			},
			wantPurposes: map[uint32][]string{
				40449: {"dvn_verify"},
			},
		},
		{
			name: "pricing only",
			mutate: func(cfg *config.Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Services.DVN.Enabled = new(false)
				cfg.Pricing = testPricingConfig()
				cfg.Pricing.Signer = config.MustEVMAddress(account.Address.Hex())
			},
			wantPurposes: map[uint32][]string{
				40161: {"pricing_set_dvn_price_config", "pricing_set_executor_price_config"},
				40449: {"pricing_set_dvn_price_config", "pricing_set_executor_price_config"},
			},
		},
		{
			name: "no tx targets",
			mutate: func(cfg *config.Config) {
				cfg.Services.Executor.Enabled = new(false)
				cfg.Services.DVN.Enabled = new(false)
				cfg.Signers = nil
			},
			wantPurposes: map[uint32][]string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := testConfig(account.Address.Hex(), filepath.Clean(account.URL.Path))
			if test.mutate != nil {
				test.mutate(&cfg)
			}
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
			got := purposesByChain(targets)
			if !equalPurposeSets(got, test.wantPurposes) {
				t.Fatalf("purposes = %#v, want %#v", got, test.wantPurposes)
			}
		})
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
	signerAddress := config.MustEVMAddress(signerID)
	return config.Config{
		DatabaseURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Signers: []config.SignerConfig{
			{
				ID:   signerAddress,
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
				Family:          config.ChainFamilyEVM,
				ChainID:         11155111,
				EndpointAddress: config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
				Confirmations:   12,
				RPCURLs:         []string{"http://localhost:8545"},
				TxRoles: config.ChainTxRolesConfig{
					Executor: testExecutorRole(signerAddress),
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
					Executor: testExecutorRole(signerAddress),
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
				},
				DestinationWorkers: config.DestinationWorkerContractsConfig{
					OpenDVN: config.MustEVMAddress("0x6666666666666666666666666666666666666666"),
				},
				DVN:             config.PathwayDVNConfig{Mode: config.DVNModeShadow},
				Enabled:         true,
				MaxMessageSize:  10000,
				MinLzReceiveGas: 100000,
				MaxLzReceiveGas: 300000,
			},
		},
	}
}

func stubReadTxHeader(t *testing.T, header *gethtypes.Header) {
	t.Helper()
	original := readTxHeader
	t.Cleanup(func() { readTxHeader = original })
	readTxHeader = func(context.Context, txmgr.ChainClient) (*gethtypes.Header, error) {
		return header, nil
	}
}

func bigOne() *big.Int {
	return big.NewInt(1)
}

func testExecutorRole(signer config.EVMAddress) config.ExecutorTxRoleConfig {
	return config.ExecutorTxRoleConfig{
		Signer:                  signer,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

func testDVNRole(signer config.EVMAddress) config.DVNTxRoleConfig {
	return config.DVNTxRoleConfig{
		Signer:                  signer,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

func testPricingConfig() config.PricingConfig {
	return config.PricingConfig{
		Enabled:                 true,
		Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
		IntervalSeconds:         300,
		BaseFeeWei:              "1000",
		BufferBps:               100,
		StaleAfterSeconds:       1800,
		MaxDeviationBps:         500,
		GasSpikeBps:             1000,
		AllowUniswapFallback:    true,
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
		Chains: []config.PricingChainConfig{
			{EID: 40161, BinanceSymbol: "ETHUSDT"},
			{EID: 40449, BinanceSymbol: "ETHUSDT"},
		},
	}
}

func purposesByChain(targets []txmgr.Target) map[uint32][]string {
	out := make(map[uint32][]string, len(targets))
	for _, target := range targets {
		for purpose := range target.FeePolicies {
			out[target.ChainEID] = append(out[target.ChainEID], purpose)
		}
	}
	for eid := range out {
		sortStrings(out[eid])
	}
	return out
}

func equalPurposeSets(a, b map[uint32][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for eid, aPurposes := range a {
		bPurposes, ok := b[eid]
		if !ok {
			return false
		}
		sortStrings(bPurposes)
		if len(aPurposes) != len(bPurposes) {
			return false
		}
		for idx := range aPurposes {
			if aPurposes[idx] != bPurposes[idx] {
				return false
			}
		}
	}
	return true
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
