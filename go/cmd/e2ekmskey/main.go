package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type kmsKeyFile struct {
	KeyID   string `json:"keyId"`
	Region  string `json:"region"`
	Address string `json:"address"`
}

func main() {
	var out string
	var endpoint string
	var region string
	flag.StringVar(&out, "out", "", "output KMS metadata JSON path")
	flag.StringVar(&endpoint, "endpoint", envOrDefault("E2E_KMS_HOST_ENDPOINT", "http://127.0.0.1:4566"), "Rustack KMS endpoint")
	flag.StringVar(&region, "region", envOrDefault("E2E_KMS_REGION", envOrDefault("AWS_REGION", "us-east-1")), "KMS region")
	flag.Parse()

	if out == "" {
		fatal("out is required")
	}
	if endpoint == "" {
		fatal("endpoint is required")
	}
	if region == "" {
		fatal("region is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		fatal("load AWS config: %v", err)
	}
	client := awskms.NewFromConfig(cfg, func(o *awskms.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	key, err := createKMSKey(ctx, client)
	if err != nil {
		fatal("create KMS key: %v", err)
	}
	rawPublicKey, err := publicKey(ctx, client, key)
	if err != nil {
		fatal("get KMS public key: %v", err)
	}
	parsedPublicKey, err := parseKMSPublicKey(rawPublicKey)
	if err != nil {
		fatal("parse KMS public key: %v", err)
	}
	payload := kmsKeyFile{
		KeyID:   key,
		Region:  region,
		Address: crypto.PubkeyToAddress(*parsedPublicKey).Hex(),
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fatal("encode metadata: %v", err)
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fatal("create output directory: %v", err)
	}
	if err := os.WriteFile(out, encoded, 0o644); err != nil {
		fatal("write metadata: %v", err)
	}
}

func createKMSKey(ctx context.Context, client *awskms.Client) (string, error) {
	var lastErr error
	for {
		out, err := client.CreateKey(ctx, &awskms.CreateKeyInput{
			KeySpec:  kmstypes.KeySpecEccSecgP256k1,
			KeyUsage: kmstypes.KeyUsageTypeSignVerify,
		})
		if err == nil {
			if out.KeyMetadata == nil || out.KeyMetadata.KeyId == nil || *out.KeyMetadata.KeyId == "" {
				return "", errors.New("CreateKey returned no key id")
			}
			return *out.KeyMetadata.KeyId, nil
		}
		lastErr = err
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("%w: %w", ctx.Err(), lastErr)
		case <-timer.C:
		}
	}
}

func publicKey(ctx context.Context, client *awskms.Client, keyID string) ([]byte, error) {
	out, err := client.GetPublicKey(ctx, &awskms.GetPublicKeyInput{KeyId: aws.String(keyID)})
	if err != nil {
		return nil, err
	}
	if len(out.PublicKey) == 0 {
		return nil, errors.New("GetPublicKey returned an empty public key")
	}
	if out.KeySpec != kmstypes.KeySpecEccSecgP256k1 {
		return nil, fmt.Errorf("KMS key %s has key spec %s, want %s", keyID, out.KeySpec, kmstypes.KeySpecEccSecgP256k1)
	}
	return out.PublicKey, nil
}

type subjectPublicKeyInfo struct {
	Algorithm        asn1.RawValue
	SubjectPublicKey asn1.BitString
}

func parseKMSPublicKey(der []byte) (*ecdsa.PublicKey, error) {
	var spki subjectPublicKeyInfo
	rest, err := asn1.Unmarshal(der, &spki)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("public key has trailing DER bytes")
	}
	if len(spki.SubjectPublicKey.Bytes) == 0 {
		return nil, errors.New("public key is empty")
	}
	return crypto.UnmarshalPubkey(spki.SubjectPublicKey.Bytes)
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
