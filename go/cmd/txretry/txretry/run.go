// Package txretry implements operator diagnostics, durable recovery requests and RPC replays.
package txretry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

// Store is the command's persistence boundary. Inspection never calls a mutation.
type Store interface {
	txmgr.RebroadcastStore
	InspectTx(context.Context, int64) (json.RawMessage, error)
	RetryFailedTx(context.Context, int64) (int64, error)
	RequestTxReplacement(context.Context, int64) error
	RequestTxCancel(context.Context, int64) error
	ResolveExternalNonceRetry(context.Context, int64) (int64, error)
	ResolveExternalNonceAbandon(context.Context, int64) error
}

// Options selects an inspection, recovery request or immediate RPC rebroadcast.
type Options struct {
	ID         int64
	Action     string
	Resolution string
	RPCURL     string
}

// Run returns safe diagnostics for a request or immediate rebroadcast.
// Rebroadcast can return both JSON and an error; callers must emit that JSON.
func Run(ctx context.Context, store Store, o Options, deps Dependencies) (json.RawMessage, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	if o.Action == "rebroadcast" {
		return runRebroadcast(ctx, store, o, deps)
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
		return nil, errors.New("action must be inspect, retry-failed, replace, cancel-nonce, resolve-external-nonce, or rebroadcast")
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

// Validate rejects invalid options before connecting to the database or RPC.
func (o Options) Validate() error {
	if o.ID <= 0 {
		return errors.New("id must be positive")
	}
	switch o.Action {
	case "inspect", "retry-failed", "replace", "cancel-nonce", "resolve-external-nonce", "rebroadcast":
	default:
		return errors.New("action must be inspect, retry-failed, replace, cancel-nonce, resolve-external-nonce, or rebroadcast")
	}
	if o.Action == "rebroadcast" {
		if err := config.ValidateRPCURL(o.RPCURL); err != nil {
			return errors.New("rebroadcast requires a valid rpc-url (HTTP(S), WS(S), or absolute IPC path)")
		}
	} else if o.RPCURL != "" {
		return errors.New("rpc-url is only valid for rebroadcast")
	}
	if o.Action == "resolve-external-nonce" {
		if o.Resolution != "retry" && o.Resolution != "abandon" {
			return errors.New("resolution must be retry or abandon")
		}
	} else if o.Resolution != "" {
		return errors.New("resolution is only valid for resolve-external-nonce")
	}
	return nil
}
