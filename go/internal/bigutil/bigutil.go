package bigutil

import (
	"fmt"
	"math/big"
)

// Clone returns a copy of value, preserving nil.
func Clone(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}

// CloneRat returns a copy of value, preserving nil.
func CloneRat(value *big.Rat) *big.Rat {
	if value == nil {
		return nil
	}
	return new(big.Rat).Set(value)
}

// CeilRat returns the least integer greater than or equal to value, preserving nil.
func CeilRat(value *big.Rat) *big.Int {
	if value == nil {
		return nil
	}
	num := value.Num()
	den := value.Denom()
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() != 0 && value.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

// ParseDecimal parses a base-10 integer field.
func ParseDecimal(field, value string) (*big.Int, error) {
	parsed, ok := parseDecimal(value)
	if !ok {
		return nil, fmt.Errorf("%s is not a valid integer", field)
	}
	return parsed, nil
}

// ParseRequiredDecimal parses a required optional base-10 integer field.
func ParseRequiredDecimal(field string, value *string) (*big.Int, error) {
	if value == nil {
		return nil, fmt.Errorf("%s is required", field)
	}
	return ParseDecimal(field, *value)
}

// ParseOptionalDecimal parses an optional base-10 integer field.
func ParseOptionalDecimal(field string, value *string) (*big.Int, error) {
	if value == nil {
		return nil, nil
	}
	return ParseDecimal(field, *value)
}

// ParsePositiveDecimal parses a strictly positive base-10 integer field.
func ParsePositiveDecimal(field, value string) (*big.Int, error) {
	parsed, ok := parseDecimal(value)
	if !ok || parsed.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be a positive integer", field)
	}
	return parsed, nil
}

// ParseNonNegativeDecimal parses a non-negative base-10 integer field.
func ParseNonNegativeDecimal(field, value string) (*big.Int, error) {
	parsed, ok := parseDecimal(value)
	if !ok || parsed.Sign() < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer", field)
	}
	return parsed, nil
}

// Min returns a copy of the smaller value, preserving nil when both are nil.
func Min(left, right *big.Int) *big.Int {
	if left == nil {
		return Clone(right)
	}
	if right == nil {
		return Clone(left)
	}
	if left.Cmp(right) <= 0 {
		return Clone(left)
	}
	return Clone(right)
}

// Max returns a copy of the larger value, preserving nil when both are nil.
func Max(left, right *big.Int) *big.Int {
	if left == nil {
		return Clone(right)
	}
	if right == nil {
		return Clone(left)
	}
	if left.Cmp(right) >= 0 {
		return Clone(left)
	}
	return Clone(right)
}

func parseDecimal(value string) (*big.Int, bool) {
	return new(big.Int).SetString(value, 10)
}
