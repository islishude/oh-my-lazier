package main

import (
	"math/big"
	"testing"

	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestChainTxTypeDefaultsAndResolvesConfiguredType(t *testing.T) {
	cfg := config.Config{
		Chains: []config.ChainConfig{
			{EID: 40161},
			{EID: 40245, TxType: config.TxTypeLegacy},
		},
	}

	txType, err := chainTxType(cfg, 40161)
	if err != nil {
		t.Fatalf("chainTxType(default) error = %v", err)
	}
	if txType != config.TxTypeDynamicFee {
		t.Fatalf("chainTxType(default) = %q, want %q", txType, config.TxTypeDynamicFee)
	}
	txType, err = chainTxType(cfg, 40245)
	if err != nil {
		t.Fatalf("chainTxType(legacy) error = %v", err)
	}
	if txType != config.TxTypeLegacy {
		t.Fatalf("chainTxType(legacy) = %q, want %q", txType, config.TxTypeLegacy)
	}
	if _, err := chainTxType(cfg, 1); err == nil {
		t.Fatal("chainTxType(unknown) error = nil, want error")
	}
}

func TestRetryInputsDynamicFeeActions(t *testing.T) {
	maxFee, priorityFee, legacyReplacement, err := retryInputs("retry-failed", config.TxTypeDynamicFee, "3000000000", "")
	if err != nil {
		t.Fatalf("retryInputs(retry-failed) error = %v", err)
	}
	if legacyReplacement {
		t.Fatal("legacyReplacement = true, want false")
	}
	if maxFee.Cmp(big.NewInt(3_000_000_000)) != 0 {
		t.Fatalf("max fee = %s, want 3000000000", maxFee)
	}
	if priorityFee != nil {
		t.Fatalf("priority fee = %s, want nil", priorityFee)
	}

	maxFee, priorityFee, legacyReplacement, err = retryInputs("replace", config.TxTypeDynamicFee, "3000000000", "1500000000")
	if err != nil {
		t.Fatalf("retryInputs(replace) error = %v", err)
	}
	if legacyReplacement {
		t.Fatal("legacyReplacement = true, want false")
	}
	if maxFee.Cmp(big.NewInt(3_000_000_000)) != 0 {
		t.Fatalf("max fee = %s, want 3000000000", maxFee)
	}
	if priorityFee.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("priority fee = %s, want 1500000000", priorityFee)
	}
}

func TestRetryInputsLegacyActions(t *testing.T) {
	maxFee, priorityFee, legacyReplacement, err := retryInputs("replace", config.TxTypeLegacy, "", "")
	if err != nil {
		t.Fatalf("retryInputs(legacy replace) error = %v", err)
	}
	if !legacyReplacement {
		t.Fatal("legacyReplacement = false, want true")
	}
	if maxFee != nil || priorityFee != nil {
		t.Fatalf("fees = %v/%v, want nil", maxFee, priorityFee)
	}

	_, _, legacyReplacement, err = retryInputs("retry-failed", config.TxTypeLegacy, "", "")
	if err != nil {
		t.Fatalf("retryInputs(legacy retry-failed) error = %v", err)
	}
	if legacyReplacement {
		t.Fatal("legacy retry-failed replacement = true, want false")
	}

	if _, _, _, err := retryInputs("replace", config.TxTypeLegacy, "3000000000", "1500000000"); err == nil {
		t.Fatal("retryInputs(legacy fees) error = nil, want unsupported fee flag error")
	}
}
