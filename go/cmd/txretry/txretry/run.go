// Package txretry implements operator diagnostics and durable recovery requests.
package txretry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Store is the command's persistence boundary. Inspection never calls a mutation.
type Store interface {
	InspectTx(context.Context, int64) (json.RawMessage, error)
	RetryFailedTx(context.Context, int64) (int64, error)
	RequestTxReplacement(context.Context, int64) error
	RequestTxCancel(context.Context, int64) error
	ResolveExternalNonceRetry(context.Context, int64) (int64, error)
	ResolveExternalNonceAbandon(context.Context, int64) error
}

// Options selects a read-only inspection or an explicit recovery request.
type Options struct {
	ID         int64
	Action     string
	Resolution string
}

// Run returns safe diagnostics; a registered request is not a signed transaction.
func Run(ctx context.Context, store Store, o Options) (json.RawMessage, error) {
	if o.ID <= 0 {
		return nil, errors.New("id must be positive")
	}
	before, err := store.InspectTx(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	if o.Action == "inspect" {
		return before, nil
	}
	id := o.ID
	switch o.Action {
	case "retry-failed":
		id, err = store.RetryFailedTx(ctx, id)
	case "replace":
		err = store.RequestTxReplacement(ctx, id)
	case "cancel-nonce":
		err = store.RequestTxCancel(ctx, id)
	case "resolve-external-nonce":
		switch o.Resolution {
		case "retry":
			id, err = store.ResolveExternalNonceRetry(ctx, id)
		case "abandon":
			err = store.ResolveExternalNonceAbandon(ctx, id)
		default:
			return nil, errors.New("resolution must be retry or abandon")
		}
	default:
		return nil, errors.New("action must be inspect, retry-failed, replace, cancel-nonce, or resolve-external-nonce")
	}
	if err != nil {
		return nil, err
	}
	after, err := store.InspectTx(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("request registered but inspection failed: %w", err)
	}
	return json.Marshal(struct {
		Action        string          `json:"action"`
		RequestStatus string          `json:"request_status"`
		Before        json.RawMessage `json:"before"`
		After         json.RawMessage `json:"after"`
	}{o.Action, "registered", before, after})
}
