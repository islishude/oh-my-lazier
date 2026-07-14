package pricing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

type priceSourceRequestError struct {
	source    string
	operation string
	cause     error
}

func (e *priceSourceRequestError) Error() string {
	if e == nil {
		return "price source request failed"
	}
	return fmt.Sprintf("%s price request %s failed", e.source, e.operation)
}

func (e *priceSourceRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func wrapPriceSourceRequestError(source, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &priceSourceRequestError{source: source, operation: operation, cause: err}
}

func isPriceSourceRequestError(err error) bool {
	var requestError *priceSourceRequestError
	return errors.As(err, &requestError)
}

func isPriceSourceContractRevert(err error) bool {
	var rpcError rpc.Error
	if errors.As(err, &rpcError) && rpcError.ErrorCode() == 3 {
		return true
	}
	var dataError rpc.DataError
	if errors.As(err, &dataError) {
		switch data := dataError.ErrorData().(type) {
		case string:
			normalized := strings.ToLower(strings.TrimSpace(data))
			if strings.HasPrefix(normalized, "0x") || strings.Contains(normalized, "execution reverted") {
				return true
			}
		case []byte:
			if len(data) > 0 {
				return true
			}
		}
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		if strings.Contains(strings.ToLower(current.Error()), "execution reverted") {
			return true
		}
	}
	return false
}
