package txmgr

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/signer/keystore"
)

func TestProcessNextSignsAndBroadcastsDynamicFeeTx(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 10}
	manager := New(store, nil)

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID:             40161,
		Purpose:              "commit-verification",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x01, 0x02, 0x03},
		Value:                big.NewInt(123),
		GasLimit:             big.NewInt(100_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), 40161, big.NewInt(11155111), signer, client)
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(client.sent))
	}
	sent := client.sent[0]
	if sent.Type() != types.DynamicFeeTxType {
		t.Fatalf("sent tx type = %d, want dynamic fee", sent.Type())
	}
	if sent.Nonce() != 10 {
		t.Fatalf("sent nonce = %d, want 10", sent.Nonce())
	}
	if sent.GasFeeCap().Cmp(big.NewInt(2_000_000_000)) != 0 {
		t.Fatalf("sent gas fee cap = %s", sent.GasFeeCap())
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(11155111)), sent)
	if err != nil {
		t.Fatalf("Sender() error = %v", err)
	}
	if from != signer.Address() {
		t.Fatalf("sender = %s, want %s", from, signer.Address())
	}

	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Status != db.TxStatusBroadcast {
		t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusBroadcast)
	}
}

func TestPrepareReplacementTxPreservesNonceAndBumpsFees(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 21}
	manager := New(store, nil)

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID:             40161,
		Purpose:              "lz-receive",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x04, 0x05},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), 40161, big.NewInt(11155111), signer, client)
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if err := store.PrepareReplacementTx(t.Context(), id, big.NewInt(3_000_000_000), big.NewInt(1_500_000_000)); err != nil {
		t.Fatalf("PrepareReplacementTx() error = %v", err)
	}
	replacement, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if replacement.Nonce != 21 {
		t.Fatalf("replacement nonce = %d, want 21", replacement.Nonce)
	}
	if replacement.Status != db.TxStatusNonceAssigned {
		t.Fatalf("replacement status = %q, want %q", replacement.Status, db.TxStatusNonceAssigned)
	}
	if replacement.MaxFeePerGas.Cmp(big.NewInt(3_000_000_000)) != 0 {
		t.Fatalf("replacement max fee = %s", replacement.MaxFeePerGas)
	}
	if replacement.MaxPriorityFeePerGas.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("replacement priority fee = %s", replacement.MaxPriorityFeePerGas)
	}
	if replacement.Attempts != 1 {
		t.Fatalf("replacement attempts = %d, want 1", replacement.Attempts)
	}
}

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	store, err := db.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	registry, err := chain.NewRegistry(testChains(), testPathways())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := store.SyncConfig(ctx, registry); err != nil {
		t.Fatalf("SyncConfig() error = %v", err)
	}
	return store
}

func newTestKeystoreSigner(t *testing.T) *keystore.Signer {
	t.Helper()
	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}
	signer, err := keystore.LoadWithPasswordSource(filepath.Clean(account.URL.Path), keystore.PasswordSource{Value: password})
	if err != nil {
		t.Fatalf("LoadWithPasswordSource() error = %v", err)
	}
	return signer
}

type fakeChainClient struct {
	pendingNonce uint64
	sent         []*types.Transaction
}

func (f *fakeChainClient) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return f.pendingNonce, nil
}

func (f *fakeChainClient) SendTransaction(_ context.Context, tx *types.Transaction) error {
	f.sent = append(f.sent, tx)
	return nil
}

func testChains() []config.ChainConfig {
	return []config.ChainConfig{
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
	}
}

func testPathways() []config.PathwayConfig {
	return []config.PathwayConfig{
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
	}
}
