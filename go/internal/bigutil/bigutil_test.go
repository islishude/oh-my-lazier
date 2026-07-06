package bigutil

import (
	"math/big"
	"strings"
	"testing"
)

func TestParseDecimal(t *testing.T) {
	got, err := ParseDecimal("fee", "12345678901234567890")
	if err != nil {
		t.Fatalf("ParseDecimal() error = %v", err)
	}
	if got.String() != "12345678901234567890" {
		t.Fatalf("ParseDecimal() = %s, want 12345678901234567890", got)
	}
	if _, err := ParseDecimal("fee", "abc"); err == nil || !strings.Contains(err.Error(), "fee is not a valid integer") {
		t.Fatalf("ParseDecimal() error = %v, want field parse error", err)
	}
}

func TestParseRequiredDecimal(t *testing.T) {
	value := "42"
	got, err := ParseRequiredDecimal("amount", &value)
	if err != nil {
		t.Fatalf("ParseRequiredDecimal() error = %v", err)
	}
	if got.String() != "42" {
		t.Fatalf("ParseRequiredDecimal() = %s, want 42", got)
	}
	if _, err := ParseRequiredDecimal("amount", nil); err == nil || !strings.Contains(err.Error(), "amount is required") {
		t.Fatalf("ParseRequiredDecimal() error = %v, want required error", err)
	}
}

func TestParseOptionalDecimal(t *testing.T) {
	got, err := ParseOptionalDecimal("amount", nil)
	if err != nil {
		t.Fatalf("ParseOptionalDecimal(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("ParseOptionalDecimal(nil) = %v, want nil", got)
	}
	value := "7"
	got, err = ParseOptionalDecimal("amount", &value)
	if err != nil {
		t.Fatalf("ParseOptionalDecimal(value) error = %v", err)
	}
	if got.String() != "7" {
		t.Fatalf("ParseOptionalDecimal(value) = %s, want 7", got)
	}
}

func TestParsePositiveDecimal(t *testing.T) {
	if got, err := ParsePositiveDecimal("threshold", "1"); err != nil || got.String() != "1" {
		t.Fatalf("ParsePositiveDecimal() = %v, %v; want 1, nil", got, err)
	}
	for _, value := range []string{"0", "-1", "abc"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParsePositiveDecimal("threshold", value); err == nil || !strings.Contains(err.Error(), "threshold must be a positive integer") {
				t.Fatalf("ParsePositiveDecimal() error = %v, want positive integer error", err)
			}
		})
	}
}

func TestParseNonNegativeDecimal(t *testing.T) {
	for _, value := range []string{"0", "1"} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseNonNegativeDecimal("fee", value)
			if err != nil {
				t.Fatalf("ParseNonNegativeDecimal() error = %v", err)
			}
			if got.String() != value {
				t.Fatalf("ParseNonNegativeDecimal() = %s, want %s", got, value)
			}
		})
	}
	for _, value := range []string{"-1", "abc"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseNonNegativeDecimal("fee", value); err == nil || !strings.Contains(err.Error(), "fee must be a non-negative integer") {
				t.Fatalf("ParseNonNegativeDecimal() error = %v, want non-negative integer error", err)
			}
		})
	}
}

func TestCloneMinMaxReturnCopies(t *testing.T) {
	left := big.NewInt(2)
	right := big.NewInt(5)

	if got := Clone(left); got == left || got.Cmp(left) != 0 {
		t.Fatalf("Clone() = %v, want equal copy", got)
	}
	if got := Min(left, right); got == left || got.String() != "2" {
		t.Fatalf("Min() = %v, want copied 2", got)
	}
	if got := Max(left, right); got == right || got.String() != "5" {
		t.Fatalf("Max() = %v, want copied 5", got)
	}
	if got := Max(nil, right); got == right || got.String() != "5" {
		t.Fatalf("Max(nil, right) = %v, want copied 5", got)
	}
	if got := Min(nil, nil); got != nil {
		t.Fatalf("Min(nil, nil) = %v, want nil", got)
	}
}
