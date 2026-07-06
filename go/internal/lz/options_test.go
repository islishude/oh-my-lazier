package lz

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func TestDecodeExecutorOptionsAcceptsSingleLzReceiveOption(t *testing.T) {
	tests := map[string][]byte{
		"type3":    testLzReceiveOptions(t, 0),
		"stripped": testLzReceiveOptions(t, 0)[2:],
	}

	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			decoded, err := DecodeExecutorOptions(options)
			if err != nil {
				t.Fatalf("DecodeExecutorOptions() error = %v", err)
			}
			if decoded.LzReceiveGas.Cmp(big.NewInt(100_000)) != 0 {
				t.Fatalf("lzReceive gas = %s, want 100000", decoded.LzReceiveGas)
			}
		})
	}
}

func TestDecodeExecutorOptionsRejectsUnsupportedOptions(t *testing.T) {
	tests := []struct {
		name       string
		optionType byte
		want       string
	}{
		{name: "native drop", optionType: optionTypeNativeDrop, want: "unsupported native drop executor option"},
		{name: "lz compose", optionType: optionTypeLzCompose, want: "unsupported lzCompose executor option"},
		{name: "ordered execution", optionType: optionTypeOrdered, want: "unsupported ordered execution executor option"},
		{name: "lz read", optionType: optionTypeLzRead, want: "unsupported lzRead executor option"},
		{name: "unknown", optionType: 99, want: "unsupported executor option type 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := testLzReceiveOptions(t, 0)
			options[5] = tt.optionType

			_, err := DecodeExecutorOptions(options)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeExecutorOptions() error = %v, want %q", err, tt.want)
			}
		})
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
