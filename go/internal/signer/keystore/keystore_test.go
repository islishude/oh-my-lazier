package keystore

import (
	"errors"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestResolvePasswordSources(t *testing.T) {
	t.Setenv("KEYSTORE_PASSWORD", "from-env")
	dir := t.TempDir()
	passwordFile := filepath.Join(dir, "password.txt")
	if err := os.WriteFile(passwordFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name   string
		source PasswordSource
		want   string
	}{
		{name: "value", source: PasswordSource{Value: "direct"}, want: "direct"},
		{name: "env", source: PasswordSource{Env: "KEYSTORE_PASSWORD"}, want: "from-env"},
		{name: "file", source: PasswordSource{File: passwordFile}, want: "from-file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePassword(tt.source)
			if err != nil {
				t.Fatalf("ResolvePassword() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolvePassword() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePasswordRejectsMissingSource(t *testing.T) {
	if _, err := ResolvePassword(PasswordSource{}); err == nil {
		t.Fatal("ResolvePassword() error = nil, want missing source error")
	}
}

func TestResolvePasswordFileReadErrorDoesNotEchoPath(t *testing.T) {
	const secret = "actual-keystore-password=abc123"
	passwordFile := filepath.Join(t.TempDir(), secret)
	_, err := ResolvePassword(PasswordSource{File: passwordFile})
	if err == nil {
		t.Fatal("ResolvePassword() error = nil, want missing password file error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("errors.Is(%v, fs.ErrNotExist) = false", err)
	}
	if strings.Contains(err.Error(), passwordFile) || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "abc123") {
		t.Fatalf("ResolvePassword() error leaked password file path: %q", err)
	}
	if err.Error() != "read keystore password file failed" {
		t.Fatalf("ResolvePassword() error = %q, want redacted file read error", err)
	}
}

func TestSignerSignsEIP1559Transaction(t *testing.T) {
	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}

	signer, err := LoadWithPasswordSource(account.URL.Path, PasswordSource{Value: password})
	if err != nil {
		t.Fatalf("LoadWithPasswordSource() error = %v", err)
	}

	chainID := big.NewInt(11155111)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     7,
		GasTipCap: big.NewInt(1_000_000_000),
		GasFeeCap: big.NewInt(2_000_000_000),
		Gas:       100_000,
		To:        new(common.HexToAddress("0x1111111111111111111111111111111111111111")),
		Value:     big.NewInt(123),
		Data:      []byte{0x01, 0x02, 0x03},
	})

	signed, err := signer.SignTx(t.Context(), tx, chainID)
	if err != nil {
		t.Fatalf("SignTx() error = %v", err)
	}
	if signed.Type() != types.DynamicFeeTxType {
		t.Fatalf("signed tx type = %d, want %d", signed.Type(), types.DynamicFeeTxType)
	}
	from, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatalf("Sender() error = %v", err)
	}
	if from != signer.Address() {
		t.Fatalf("sender = %s, want %s", from, signer.Address())
	}
}

func TestSignerSignsLegacyTransaction(t *testing.T) {
	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}

	signer, err := LoadWithPasswordSource(account.URL.Path, PasswordSource{Value: password})
	if err != nil {
		t.Fatalf("LoadWithPasswordSource() error = %v", err)
	}

	chainID := big.NewInt(11155111)
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    7,
		GasPrice: big.NewInt(2_000_000_000),
		Gas:      100_000,
		To:       &to,
		Value:    big.NewInt(123),
		Data:     []byte{0x01, 0x02, 0x03},
	})

	signed, err := signer.SignTx(t.Context(), tx, chainID)
	if err != nil {
		t.Fatalf("SignTx() error = %v", err)
	}
	if signed.Type() != types.LegacyTxType {
		t.Fatalf("signed tx type = %d, want %d", signed.Type(), types.LegacyTxType)
	}
	from, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatalf("Sender() error = %v", err)
	}
	if from != signer.Address() {
		t.Fatalf("sender = %s, want %s", from, signer.Address())
	}
}
