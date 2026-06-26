package rpcquorum

import (
	"math/big"
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
