package rpcquorum

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

func TestReceiptFingerprintIncludesLogEvidence(t *testing.T) {
	receipt := testReceipt()
	mutated := testReceipt()
	mutated.Logs[0].Data = []byte{0x01, 0x02}

	if receiptFingerprint(receipt) == receiptFingerprint(mutated) {
		t.Fatal("receipt fingerprint ignored log data")
	}
}

func TestReceiptFingerprintMatchesEquivalentReceipts(t *testing.T) {
	left := testReceipt()
	right := testReceipt()

	if receiptFingerprint(left) != receiptFingerprint(right) {
		t.Fatalf("equivalent receipt fingerprints differ:\nleft:  %s\nright: %s", receiptFingerprint(left), receiptFingerprint(right))
	}
}

func TestIsReceiptConflict(t *testing.T) {
	err := &ReceiptConflictError{TxHash: common.HexToHash("0x1234")}
	if !IsReceiptConflict(err) {
		t.Fatal("IsReceiptConflict() = false, want true")
	}
}

func TestValidateProviderChainIDsAcceptsAllExpected(t *testing.T) {
	err := validateProviderChainIDs("testnet", big.NewInt(11155111), []providerChainID{
		{ProviderID: "provider-a", ChainID: big.NewInt(11155111)},
		{ProviderID: "provider-b", ChainID: big.NewInt(11155111)},
	})
	if err != nil {
		t.Fatalf("validateProviderChainIDs() error = %v", err)
	}
}

func TestValidateProviderChainIDsRejectsUnexpectedProviderChainID(t *testing.T) {
	err := validateProviderChainIDs("testnet", big.NewInt(11155111), []providerChainID{
		{ProviderID: "provider-a", ChainID: big.NewInt(11155111)},
		{ProviderID: "provider-b", ChainID: big.NewInt(560048)},
	})
	if err == nil {
		t.Fatal("validateProviderChainIDs() error = nil, want mismatch")
	}
	if !IsChainIDMismatch(err) {
		t.Fatalf("IsChainIDMismatch() = false for %T", err)
	}
	if !strings.Contains(err.Error(), "provider-b returned 560048") {
		t.Fatalf("error = %q, want provider detail", err)
	}
}

func TestValidateChainIDRedactsProviderURLOnRequestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	const user = "rpc-user"
	const password = "rpc-password"
	const apiKey = "rpc-api-key"
	rawURL := strings.Replace(server.URL, "http://", "http://"+user+":"+password+"@", 1) + "?api_key=" + apiKey
	client := New("testnet", []string{rawURL})
	defer client.Close()

	err := client.ValidateChainID(context.Background(), big.NewInt(1))
	if err == nil {
		t.Fatal("ValidateChainID() error = nil, want request failure")
	}
	if !strings.Contains(err.Error(), "provider[0] eth_chainId failed") {
		t.Fatalf("ValidateChainID() error = %q, want redacted provider failure", err)
	}
	for _, secret := range []string{user, password, apiKey, rawURL} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("ValidateChainID() leaked %q in error %q", secret, err)
		}
	}
}

func TestProviderOperationErrorRedactsCauseAndPreservesIdentity(t *testing.T) {
	cause := testRPCError{message: "upstream included rpc-secret-token", code: 3}
	err := wrapProviderOperationError(2, "eth_getLogs", cause)
	if err.Error() != "provider[2] eth_getLogs failed" {
		t.Fatalf("error = %q, want redacted provider operation", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is() = false, want wrapped cause identity")
	}
	var rpcErr rpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.ErrorCode() != 3 {
		t.Fatalf("errors.As() did not preserve rpc error: %v", err)
	}
	if strings.Contains(err.Error(), "rpc-secret-token") {
		t.Fatalf("error leaked cause: %q", err)
	}
	canceled := wrapProviderOperationError(2, "eth_getLogs", context.Canceled)
	if !errors.Is(canceled, context.Canceled) {
		t.Fatalf("errors.Is(context.Canceled) = false for %v", canceled)
	}
}

func TestProvidersReturnRedactedIdentities(t *testing.T) {
	const secretURL = "https://rpc-user:rpc-password@rpc-secret.example/v2/rpc-api-key"
	client := New("testnet", []string{secretURL})
	providers := client.Providers()
	if len(providers) != 1 {
		t.Fatalf("Providers() length = %d, want 1", len(providers))
	}
	if providers[0].ID != "provider[0]" || providers[0].Status != ProviderHealthy {
		t.Fatalf("Providers()[0] = %+v, want redacted healthy provider", providers[0])
	}
	if strings.Contains(providers[0].ID, "rpc-secret") {
		t.Fatalf("provider id leaked configured URL: %q", providers[0].ID)
	}
}

func TestCheckHeadConflictRedactsProviderURLs(t *testing.T) {
	const firstURL = "https://first-user:first-password@first-secret.example/v2/first-api-key"
	const secondURL = "https://second-user:second-password@second-secret.example/v2/second-api-key"
	client := &Client{chainName: "testnet", providers: []configuredProvider{
		{url: firstURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{header: testHeader(0x01)})},
		{url: secondURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{header: testHeader(0x02)})},
	}}

	_, err := client.CheckHead(context.Background())
	if err == nil || !IsHeadConflict(err) {
		t.Fatalf("CheckHead() error = %v, want head conflict", err)
	}
	assertRedactedProviderError(t, err, []string{firstURL, secondURL, "first-password", "second-api-key"}, "provider[0]", "provider[1]")
}

func TestTransactionReceiptConflictRedactsProviderURLs(t *testing.T) {
	const firstURL = "https://first-user:first-password@first-secret.example/v2/first-api-key"
	const secondURL = "https://second-user:second-password@second-secret.example/v2/second-api-key"
	firstReceipt := testReceipt()
	secondReceipt := testReceipt()
	secondReceipt.Status = gethtypes.ReceiptStatusFailed
	client := &Client{chainName: "testnet", providers: []configuredProvider{
		{url: firstURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{receipt: firstReceipt})},
		{url: secondURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{receipt: secondReceipt})},
	}}

	_, err := client.TransactionReceipt(context.Background(), firstReceipt.TxHash)
	if err == nil || !IsReceiptConflict(err) {
		t.Fatalf("TransactionReceipt() error = %v, want receipt conflict", err)
	}
	assertRedactedProviderError(t, err, []string{firstURL, secondURL, "first-password", "second-api-key"}, "provider[0]", "provider[1]")
}

func TestTransactionReceiptPartialNotFoundRedactsProviderURLs(t *testing.T) {
	const firstURL = "https://first-user:first-password@first-secret.example/v2/first-api-key"
	const secondURL = "https://second-user:second-password@second-secret.example/v2/second-api-key"
	receipt := testReceipt()
	client := &Client{chainName: "testnet", providers: []configuredProvider{
		{url: firstURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{receipt: receipt})},
		{url: secondURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{})},
	}}

	_, err := client.TransactionReceipt(context.Background(), receipt.TxHash)
	if err == nil || !IsReceiptConflict(err) {
		t.Fatalf("TransactionReceipt() error = %v, want partial not-found conflict", err)
	}
	assertRedactedProviderError(t, err, []string{firstURL, secondURL, "first-password", "second-api-key"}, "provider[1]")
}

func TestTransactionReceiptTransientErrorRedactsProviderURL(t *testing.T) {
	const secretURL = "https://rpc-user:rpc-password@rpc-secret.example/v2/rpc-api-key"
	cause := errors.New("upstream echoed rpc-api-key")
	client := &Client{chainName: "testnet", providers: []configuredProvider{
		{url: secretURL, status: ProviderHealthy, client: newTestEthClient(t, testEthService{err: cause})},
	}}

	_, err := client.TransactionReceipt(context.Background(), common.HexToHash("0x1234"))
	if err == nil {
		t.Fatal("TransactionReceipt() error = nil, want transient failure")
	}
	assertRedactedProviderError(t, err, []string{secretURL, "rpc-password", "rpc-api-key"}, "provider[0]", "eth_getTransactionReceipt")
}

func TestSelectCanonicalHeadRejectsSameHeightHashConflict(t *testing.T) {
	_, err := selectCanonicalHead("testnet", []providerHead{
		{Index: 0, Number: big.NewInt(42), Hash: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{Index: 1, Number: big.NewInt(42), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	})
	if err == nil {
		t.Fatal("selectCanonicalHead() error = nil, want conflict")
	}
	if !IsHeadConflict(err) {
		t.Fatalf("IsHeadConflict() = false for %T", err)
	}
	if !strings.Contains(err.Error(), "provider[0]") || !strings.Contains(err.Error(), "provider[1]") {
		t.Fatalf("error = %q, want redacted provider identities", err)
	}
}

func TestSelectCanonicalHeadIgnoresLaggingProvider(t *testing.T) {
	expectedHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	head, err := selectCanonicalHead("testnet", []providerHead{
		{Index: 0, Number: big.NewInt(42), Hash: expectedHash},
		{Index: 1, Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	})
	if err != nil {
		t.Fatalf("selectCanonicalHead() error = %v", err)
	}
	if head.Number.Uint64() != 42 {
		t.Fatalf("head number = %s, want 42", head.Number)
	}
	if head.Hash != expectedHash.Hex() {
		t.Fatalf("head hash = %s, want %s", head.Hash, expectedHash.Hex())
	}
}

func TestSelectCanonicalHeadAcceptsTwoOfThreeAgreement(t *testing.T) {
	expectedHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	head, err := selectCanonicalHead("testnet", []providerHead{
		{Index: 0, Number: big.NewInt(42), Hash: expectedHash},
		{Index: 1, Number: big.NewInt(42), Hash: expectedHash},
		{Index: 2, Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	})
	if err != nil {
		t.Fatalf("selectCanonicalHead() error = %v", err)
	}
	if head.Number.Uint64() != 42 {
		t.Fatalf("head number = %s, want 42", head.Number)
	}
	if head.Hash != expectedHash.Hex() {
		t.Fatalf("head hash = %s, want %s", head.Hash, expectedHash.Hex())
	}
}

func TestClassifyHeadProviderStatusesDegradesLaggingProvider(t *testing.T) {
	canonicalHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	statuses := classifyHeadProviderStatuses([]providerHead{
		{Index: 0, Number: big.NewInt(42), Hash: canonicalHash},
		{Index: 1, Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	}, HeadResult{Number: big.NewInt(42), Hash: canonicalHash.Hex()})

	if statuses[0] != ProviderHealthy {
		t.Fatalf("provider[0] status = %q, want %q", statuses[0], ProviderHealthy)
	}
	if statuses[1] != ProviderLagging {
		t.Fatalf("provider[1] status = %q, want %q", statuses[1], ProviderLagging)
	}
}

func TestClassifyHeadProviderStatusesMarksSameHeightConflict(t *testing.T) {
	statuses := classifyHeadProviderStatuses([]providerHead{
		{Index: 0, Number: big.NewInt(42), Hash: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{Index: 1, Number: big.NewInt(42), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	}, HeadResult{})

	if statuses[0] != ProviderConflict {
		t.Fatalf("provider[0] status = %q, want %q", statuses[0], ProviderConflict)
	}
	if statuses[1] != ProviderConflict {
		t.Fatalf("provider[1] status = %q, want %q", statuses[1], ProviderConflict)
	}
}

func TestUpdateHeadProviderStatusesAllowsLaggingProviderRecovery(t *testing.T) {
	canonicalHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	client := &Client{providers: []configuredProvider{
		{url: "provider-a", status: ProviderHealthy},
		{url: "provider-b", status: ProviderLagging},
	}}

	client.updateHeadProviderStatuses([]providerHead{
		{Index: 0, Number: big.NewInt(43), Hash: canonicalHash},
		{Index: 1, Number: big.NewInt(43), Hash: canonicalHash},
	}, HeadResult{Number: big.NewInt(43), Hash: canonicalHash.Hex()})

	providers := client.Providers()
	if providers[1].ID != "provider[1]" {
		t.Fatalf("provider id = %q, want provider[1]", providers[1].ID)
	}
	if providers[1].Status != ProviderHealthy {
		t.Fatalf("provider[1] status = %q, want %q", providers[1].Status, ProviderHealthy)
	}
}

func TestUpdateHeadProviderStatusesAllowsConflictProviderRecovery(t *testing.T) {
	canonicalHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	client := &Client{providers: []configuredProvider{
		{url: "provider-a", status: ProviderHealthy},
		{url: "provider-b", status: ProviderConflict},
	}}

	client.updateHeadProviderStatuses([]providerHead{
		{Index: 0, Number: big.NewInt(43), Hash: canonicalHash},
		{Index: 1, Number: big.NewInt(43), Hash: canonicalHash},
	}, HeadResult{Number: big.NewInt(43), Hash: canonicalHash.Hex()})

	providers := client.Providers()
	if providers[1].Status != ProviderHealthy {
		t.Fatalf("provider[1] status = %q, want %q", providers[1].Status, ProviderHealthy)
	}
}

type testEthService struct {
	header  *gethtypes.Header
	receipt *gethtypes.Receipt
	err     error
}

type testRPCError struct {
	message string
	code    int
}

func (e testRPCError) Error() string {
	return e.message
}

func (e testRPCError) ErrorCode() int {
	return e.code
}

func (s testEthService) GetBlockByNumber(context.Context, rpc.BlockNumber, bool) (*gethtypes.Header, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.header, nil
}

func (s testEthService) GetTransactionReceipt(context.Context, common.Hash) (any, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.receipt == nil {
		return nil, nil
	}
	return s.receipt, nil
}

func newTestEthClient(t *testing.T, service testEthService) *ethclient.Client {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("eth", service); err != nil {
		t.Fatalf("RegisterName() error = %v", err)
	}
	rpcClient := rpc.DialInProc(server)
	client := ethclient.NewClient(rpcClient)
	t.Cleanup(func() {
		client.Close()
		server.Stop()
	})
	return client
}

func testHeader(extra byte) *gethtypes.Header {
	return &gethtypes.Header{
		ParentHash:  common.HexToHash("0x1111"),
		UncleHash:   gethtypes.EmptyUncleHash,
		Coinbase:    common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Root:        common.HexToHash("0x3333"),
		TxHash:      gethtypes.EmptyTxsHash,
		ReceiptHash: gethtypes.EmptyReceiptsHash,
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(42),
		GasLimit:    30_000_000,
		Time:        1_700_000_000,
		Extra:       []byte{extra},
		BaseFee:     big.NewInt(1_000_000_000),
	}
}

func assertRedactedProviderError(t *testing.T, err error, secrets []string, expected ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %q", secret, err)
		}
	}
	for _, value := range expected {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("error = %q, want %q", err, value)
		}
	}
}

func testReceipt() *gethtypes.Receipt {
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	return &gethtypes.Receipt{
		TxHash:      txHash,
		Status:      gethtypes.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(99),
		BlockHash:   common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Logs: []*gethtypes.Log{{
			Address:     common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Topics:      []common.Hash{common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")},
			Data:        []byte{0x01},
			TxHash:      txHash,
			BlockNumber: 99,
			BlockHash:   common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			Index:       7,
		}},
	}
}
