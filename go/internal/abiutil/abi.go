package abiutil

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// MustParse parses an ABI definition and panics when the embedded ABI is invalid.
func MustParse(definition string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(definition))
	if err != nil {
		panic(err)
	}
	return parsed
}
