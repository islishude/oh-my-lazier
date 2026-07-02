package bigutil

import "math/big"

// Clone returns a copy of value, preserving nil.
func Clone(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}
