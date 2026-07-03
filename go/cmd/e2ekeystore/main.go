package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
)

func main() {
	var out string
	var privateKeyEnv string
	var passwordEnv string
	flag.StringVar(&out, "out", "", "output geth keystore path")
	flag.StringVar(&privateKeyEnv, "private-key-env", "E2E_WORKER_PRIVATE_KEY", "environment variable containing the local test private key")
	flag.StringVar(&passwordEnv, "password-env", "E2E_KEYSTORE_PASSWORD", "environment variable containing the keystore password")
	flag.Parse()

	if out == "" {
		fatal("out is required")
	}
	privateKeyRaw := os.Getenv(privateKeyEnv)
	if privateKeyRaw == "" {
		fatal("%s is required", privateKeyEnv)
	}
	password := os.Getenv(passwordEnv)
	if password == "" {
		fatal("%s is required", passwordEnv)
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyRaw, "0x"))
	if err != nil {
		fatal("parse %s: %v", privateKeyEnv, err)
	}
	key := &gethkeystore.Key{
		Id:         uuid.New(),
		Address:    crypto.PubkeyToAddress(privateKey.PublicKey),
		PrivateKey: privateKey,
	}
	encoded, err := gethkeystore.EncryptKey(key, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		fatal("encrypt keystore: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fatal("create output directory: %v", err)
	}
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		fatal("write keystore: %v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
