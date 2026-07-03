package rpcquorum

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
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
		{URL: "provider-a", ChainID: big.NewInt(11155111)},
		{URL: "provider-b", ChainID: big.NewInt(11155111)},
	})
	if err != nil {
		t.Fatalf("validateProviderChainIDs() error = %v", err)
	}
}

func TestValidateProviderChainIDsRejectsUnexpectedProviderChainID(t *testing.T) {
	err := validateProviderChainIDs("testnet", big.NewInt(11155111), []providerChainID{
		{URL: "provider-a", ChainID: big.NewInt(11155111)},
		{URL: "provider-b", ChainID: big.NewInt(560048)},
	})
	if err == nil {
		t.Fatal("validateProviderChainIDs() error = nil, want mismatch")
	}
	if !IsChainIDMismatch(err) {
		t.Fatalf("IsChainIDMismatch() = false for %T", err)
	}
	if !strings.Contains(err.Error(), "provider provider-b returned 560048") {
		t.Fatalf("error = %q, want provider detail", err)
	}
}

func TestSelectCanonicalHeadRejectsSameHeightHashConflict(t *testing.T) {
	_, err := selectCanonicalHead("testnet", []providerHead{
		{URL: "provider-a", Number: big.NewInt(42), Hash: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{URL: "provider-b", Number: big.NewInt(42), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	})
	if err == nil {
		t.Fatal("selectCanonicalHead() error = nil, want conflict")
	}
	if !IsHeadConflict(err) {
		t.Fatalf("IsHeadConflict() = false for %T", err)
	}
}

func TestSelectCanonicalHeadIgnoresLaggingProvider(t *testing.T) {
	expectedHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	head, err := selectCanonicalHead("testnet", []providerHead{
		{URL: "provider-a", Number: big.NewInt(42), Hash: expectedHash},
		{URL: "provider-b", Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
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
		{URL: "provider-a", Number: big.NewInt(42), Hash: expectedHash},
		{URL: "provider-b", Number: big.NewInt(42), Hash: expectedHash},
		{URL: "provider-c", Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
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
		{URL: "provider-a", Number: big.NewInt(42), Hash: canonicalHash},
		{URL: "provider-b", Number: big.NewInt(41), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	}, HeadResult{Number: big.NewInt(42), Hash: canonicalHash.Hex()})

	if statuses["provider-a"] != ProviderHealthy {
		t.Fatalf("provider-a status = %q, want %q", statuses["provider-a"], ProviderHealthy)
	}
	if statuses["provider-b"] != ProviderLagging {
		t.Fatalf("provider-b status = %q, want %q", statuses["provider-b"], ProviderLagging)
	}
}

func TestClassifyHeadProviderStatusesMarksSameHeightConflict(t *testing.T) {
	statuses := classifyHeadProviderStatuses([]providerHead{
		{URL: "provider-a", Number: big.NewInt(42), Hash: common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")},
		{URL: "provider-b", Number: big.NewInt(42), Hash: common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
	}, HeadResult{})

	if statuses["provider-a"] != ProviderConflict {
		t.Fatalf("provider-a status = %q, want %q", statuses["provider-a"], ProviderConflict)
	}
	if statuses["provider-b"] != ProviderConflict {
		t.Fatalf("provider-b status = %q, want %q", statuses["provider-b"], ProviderConflict)
	}
}

func TestUpdateHeadProviderStatusesAllowsLaggingProviderRecovery(t *testing.T) {
	canonicalHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	client := &Client{providers: []Provider{
		{URL: "provider-a", Status: ProviderHealthy},
		{URL: "provider-b", Status: ProviderLagging},
	}}

	client.updateHeadProviderStatuses([]providerHead{
		{URL: "provider-a", Number: big.NewInt(43), Hash: canonicalHash},
		{URL: "provider-b", Number: big.NewInt(43), Hash: canonicalHash},
	}, HeadResult{Number: big.NewInt(43), Hash: canonicalHash.Hex()})

	providers := client.Providers()
	if providers[1].Status != ProviderHealthy {
		t.Fatalf("provider-b status = %q, want %q", providers[1].Status, ProviderHealthy)
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
