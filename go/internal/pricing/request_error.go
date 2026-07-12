package pricing

import "fmt"

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
