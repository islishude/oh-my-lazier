package lz

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func TestDecodeExecutorOptionsAcceptsSingleLzReceiveOption(t *testing.T) {
	options := testLzReceiveOptions(t, 0)

	decoded, err := DecodeExecutorOptions(options)
	if err != nil {
		t.Fatalf("DecodeExecutorOptions() error = %v", err)
	}
	if decoded.LzReceiveGas.Cmp(big.NewInt(100_000)) != 0 {
		t.Fatalf("lzReceive gas = %s, want 100000", decoded.LzReceiveGas)
	}
}

func TestDecodeExecutorOptionsRejectsUnsupportedOption(t *testing.T) {
	options := testLzReceiveOptions(t, 0)
	options[5] = 2

	_, err := DecodeExecutorOptions(options)
	if err == nil || !strings.Contains(err.Error(), "unsupported executor option type") {
		t.Fatalf("DecodeExecutorOptions() error = %v, want unsupported option type", err)
	}
}

func TestDecodeExecutorOptionsRejectsNonZeroNativeValue(t *testing.T) {
	options := testLzReceiveOptions(t, 1)

	_, err := DecodeExecutorOptions(options)
	if err == nil || !strings.Contains(err.Error(), "non-zero lzReceive native value") {
		t.Fatalf("DecodeExecutorOptions() error = %v, want non-zero value", err)
	}
}

func TestDecodeExecutorOptionsRejectsDuplicateLzReceive(t *testing.T) {
	option := testLzReceiveOptions(t, 0)
	duplicated := append(append([]byte{}, option...), option[2:]...)

	_, err := DecodeExecutorOptions(duplicated)
	if err == nil || !strings.Contains(err.Error(), "duplicate lzReceive option") {
		t.Fatalf("DecodeExecutorOptions() error = %v, want duplicate lzReceive", err)
	}
}

func testLzReceiveOptions(t *testing.T, value uint64) []byte {
	t.Helper()
	payload := make([]byte, 32)
	new(big.Int).SetUint64(100_000).FillBytes(payload[:16])
	new(big.Int).SetUint64(value).FillBytes(payload[16:])

	options := make([]byte, 0, 38)
	options = append(options, 0x00, 0x03)
	options = append(options, 0x01)
	options = append(options, 0x00, 0x21)
	options = append(options, 0x01)
	options = append(options, payload...)

	roundTrip, err := hex.DecodeString(hex.EncodeToString(options))
	if err != nil {
		t.Fatalf("hex round trip error = %v", err)
	}
	return roundTrip
}
