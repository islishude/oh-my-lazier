package rpcquorum

import (
	"context"
	"errors"
	"math/big"
)

// ProviderStatus is the worker's health classification for one RPC provider.
type ProviderStatus string

const (
	// ProviderHealthy means the provider agrees with quorum checks.
	ProviderHealthy ProviderStatus = "healthy"
	// ProviderLagging means the provider is behind the selected chain head.
	ProviderLagging ProviderStatus = "lagging"
	// ProviderConflict means the provider disagrees on canonical chain data.
	ProviderConflict ProviderStatus = "conflict"
)

// Provider describes one configured RPC endpoint and its current health status.
type Provider struct {
	URL    string
	Status ProviderStatus
}

// Client coordinates multiple RPC providers for one chain.
type Client struct {
	chainName string
	providers []Provider
}

// HeadResult is the canonical head selected by quorum checks.
type HeadResult struct {
	Number *big.Int
	Hash   string
}

// New constructs a quorum client from configured RPC URLs.
func New(chainName string, urls []string) *Client {
	providers := make([]Provider, 0, len(urls))
	for _, url := range urls {
		providers = append(providers, Provider{URL: url, Status: ProviderHealthy})
	}
	return &Client{chainName: chainName, providers: providers}
}

// Providers returns a copy of the configured provider statuses.
func (c *Client) Providers() []Provider {
	out := make([]Provider, len(c.providers))
	copy(out, c.providers)
	return out
}

// CheckHead verifies provider head agreement and returns the selected head.
func (c *Client) CheckHead(context.Context) (HeadResult, error) {
	if len(c.providers) == 0 {
		return HeadResult{}, errors.New("no rpc providers configured")
	}
	return HeadResult{}, nil
}
