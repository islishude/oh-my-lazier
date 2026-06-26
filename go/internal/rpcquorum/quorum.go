package rpcquorum

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var _ interface {
	BlockNumber(context.Context) (uint64, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
	CheckHead(context.Context) (HeadResult, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]gethtypes.Log, error)
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	SendTransaction(context.Context, *gethtypes.Transaction) error
	SuggestGasPrice(context.Context) (*big.Int, error)
	SubscribeFilterLogs(context.Context, ethereum.FilterQuery, chan<- gethtypes.Log) (ethereum.Subscription, error)
	TransactionReceipt(context.Context, common.Hash) (*gethtypes.Receipt, error)
} = (*Client)(nil)

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

// HeadConflictError reports a same-height block hash disagreement between RPC providers.
type HeadConflictError struct {
	ChainName string
	Number    *big.Int
	Details   []string
}

// Error returns the provider disagreement details.
func (e *HeadConflictError) Error() string {
	if e == nil {
		return "rpc head quorum conflict"
	}
	number := "<unknown>"
	if e.Number != nil {
		number = e.Number.String()
	}
	if len(e.Details) == 0 {
		return fmt.Sprintf("rpc head quorum conflict for chain %s at block %s", e.ChainName, number)
	}
	return fmt.Sprintf("rpc head quorum conflict for chain %s at block %s: %s", e.ChainName, number, strings.Join(e.Details, "; "))
}

// IsHeadConflict reports whether err is a head quorum conflict.
func IsHeadConflict(err error) bool {
	var conflict *HeadConflictError
	return errors.As(err, &conflict)
}

// ReceiptConflictError reports a source-chain receipt disagreement between RPC providers.
type ReceiptConflictError struct {
	TxHash  common.Hash
	Details []string
}

// Error returns the provider disagreement details.
func (e *ReceiptConflictError) Error() string {
	if e == nil {
		return "rpc receipt quorum conflict"
	}
	if len(e.Details) == 0 {
		return fmt.Sprintf("rpc receipt quorum conflict for tx %s", e.TxHash)
	}
	return fmt.Sprintf("rpc receipt quorum conflict for tx %s: %s", e.TxHash, strings.Join(e.Details, "; "))
}

// IsReceiptConflict reports whether err is a receipt quorum conflict.
func IsReceiptConflict(err error) bool {
	var conflict *ReceiptConflictError
	return errors.As(err, &conflict)
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
func (c *Client) CheckHead(ctx context.Context) (HeadResult, error) {
	if len(c.providers) == 0 {
		return HeadResult{}, errors.New("no rpc providers configured")
	}
	heads := make([]providerHead, 0, len(c.providers))
	var transientErrs []error
	for _, provider := range c.providers {
		if provider.Status != ProviderHealthy {
			continue
		}
		header, err := c.headerByNumberFromProvider(ctx, provider, nil)
		if err != nil {
			transientErrs = append(transientErrs, fmt.Errorf("%s: %w", provider.URL, err))
			continue
		}
		if header == nil || header.Number == nil {
			transientErrs = append(transientErrs, fmt.Errorf("%s: latest header is missing number", provider.URL))
			continue
		}
		heads = append(heads, providerHead{URL: provider.URL, Number: header.Number, Hash: header.Hash()})
	}
	result, err := selectCanonicalHead(c.chainName, heads)
	if err != nil {
		if len(heads) == 0 && len(transientErrs) > 0 {
			return HeadResult{}, errors.Join(transientErrs...)
		}
		return HeadResult{}, err
	}
	return result, nil
}

// CallContract performs an eth_call against the first currently healthy provider.
func (c *Client) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.CallContract(ctx, call, blockNumber)
}

// BlockNumber returns the latest block number from the first currently healthy provider.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return 0, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	return client.BlockNumber(ctx)
}

// FilterLogs returns logs from the first currently healthy provider for a bounded query.
func (c *Client) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.FilterLogs(ctx, query)
}

// SuggestGasPrice returns the first healthy provider's legacy gas price estimate.
func (c *Client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.SuggestGasPrice(ctx)
}

// PendingNonceAt returns the first healthy provider's pending account nonce.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return 0, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	return client.PendingNonceAt(ctx, account)
}

// SendTransaction broadcasts a signed transaction through the first healthy provider.
func (c *Client) SendTransaction(ctx context.Context, tx *gethtypes.Transaction) error {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.SendTransaction(ctx, tx)
}

// TransactionReceipt returns a receipt only when healthy providers agree on the receipt.
func (c *Client) TransactionReceipt(ctx context.Context, txHash common.Hash) (*gethtypes.Receipt, error) {
	var canonical *gethtypes.Receipt
	var canonicalFingerprint string
	var transientErrs []error
	var notFoundProviders []string
	for _, provider := range c.providers {
		if provider.Status != ProviderHealthy {
			continue
		}
		receipt, err := c.transactionReceiptFromProvider(ctx, provider, txHash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				notFoundProviders = append(notFoundProviders, provider.URL)
				continue
			}
			transientErrs = append(transientErrs, fmt.Errorf("%s: %w", provider.URL, err))
			continue
		}
		fingerprint := receiptFingerprint(receipt)
		if canonical == nil {
			canonical = receipt
			canonicalFingerprint = fingerprint
			continue
		}
		if fingerprint != canonicalFingerprint {
			return nil, &ReceiptConflictError{
				TxHash: txHash,
				Details: []string{
					fmt.Sprintf("provider %s returned %s", provider.URL, fingerprint),
					fmt.Sprintf("canonical %s", canonicalFingerprint),
				},
			}
		}
	}
	if canonical != nil && len(notFoundProviders) > 0 {
		return nil, &ReceiptConflictError{
			TxHash:  txHash,
			Details: []string{fmt.Sprintf("providers missing mined receipt: %s", strings.Join(notFoundProviders, ", "))},
		}
	}
	if len(transientErrs) > 0 {
		return nil, errors.Join(transientErrs...)
	}
	if canonical == nil {
		if len(notFoundProviders) > 0 {
			return nil, ethereum.NotFound
		}
		return nil, errors.New("no healthy rpc providers configured")
	}
	return canonical, nil
}

func (c *Client) transactionReceiptFromProvider(ctx context.Context, provider Provider, txHash common.Hash) (*gethtypes.Receipt, error) {
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.TransactionReceipt(ctx, txHash)
}

func (c *Client) headerByNumberFromProvider(ctx context.Context, provider Provider, number *big.Int) (*gethtypes.Header, error) {
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.HeaderByNumber(ctx, number)
}

// SubscribeFilterLogs subscribes to live logs on the first currently healthy provider.
func (c *Client) SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- gethtypes.Log) (ethereum.Subscription, error) {
	provider, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, provider.URL)
	if err != nil {
		return nil, err
	}
	subscription, err := client.SubscribeFilterLogs(ctx, query, ch)
	if err != nil {
		client.Close()
		return nil, err
	}
	return &managedSubscription{Subscription: subscription, close: client.Close}, nil
}

func (c *Client) firstHealthyProvider() (Provider, error) {
	for _, provider := range c.providers {
		if provider.Status == ProviderHealthy {
			return provider, nil
		}
	}
	return Provider{}, errors.New("no healthy rpc providers configured")
}

type managedSubscription struct {
	ethereum.Subscription
	close func()
}

func (s *managedSubscription) Unsubscribe() {
	s.Subscription.Unsubscribe()
	if s.close != nil {
		s.close()
	}
}

func receiptFingerprint(receipt *gethtypes.Receipt) string {
	if receipt == nil {
		return "<nil>"
	}
	var builder strings.Builder
	builder.WriteString(receipt.TxHash.Hex())
	builder.WriteString("|status=")
	fmt.Fprint(&builder, receipt.Status)
	builder.WriteString("|block=")
	if receipt.BlockNumber != nil {
		builder.WriteString(receipt.BlockNumber.String())
	}
	builder.WriteString("|block_hash=")
	builder.WriteString(receipt.BlockHash.Hex())
	builder.WriteString("|logs=")
	fmt.Fprint(&builder, len(receipt.Logs))
	for _, log := range receipt.Logs {
		if log == nil {
			builder.WriteString("|<nil>")
			continue
		}
		builder.WriteString("|")
		builder.WriteString(log.Address.Hex())
		builder.WriteString("/")
		builder.WriteString(log.TxHash.Hex())
		builder.WriteString("/")
		fmt.Fprint(&builder, log.Index)
		builder.WriteString("/")
		fmt.Fprint(&builder, log.BlockNumber)
		builder.WriteString("/")
		builder.WriteString(log.BlockHash.Hex())
		builder.WriteString("/")
		builder.WriteString(common.Bytes2Hex(log.Data))
		for _, topic := range log.Topics {
			builder.WriteString("/")
			builder.WriteString(topic.Hex())
		}
	}
	return builder.String()
}

type providerHead struct {
	URL    string
	Number *big.Int
	Hash   common.Hash
}

func selectCanonicalHead(chainName string, heads []providerHead) (HeadResult, error) {
	if len(heads) == 0 {
		return HeadResult{}, errors.New("no healthy rpc providers configured")
	}
	var canonical providerHead
	for _, head := range heads {
		if head.Number == nil {
			return HeadResult{}, fmt.Errorf("provider %s returned head without number", head.URL)
		}
		if canonical.Number == nil || head.Number.Cmp(canonical.Number) > 0 {
			canonical = head
			continue
		}
		if head.Number.Cmp(canonical.Number) == 0 && head.Hash != canonical.Hash {
			return HeadResult{}, &HeadConflictError{
				ChainName: chainName,
				Number:    new(big.Int).Set(head.Number),
				Details: []string{
					fmt.Sprintf("provider %s returned %s", head.URL, head.Hash),
					fmt.Sprintf("provider %s returned %s", canonical.URL, canonical.Hash),
				},
			}
		}
	}
	for _, head := range heads {
		if head.Number.Cmp(canonical.Number) == 0 && head.Hash != canonical.Hash {
			return HeadResult{}, &HeadConflictError{
				ChainName: chainName,
				Number:    new(big.Int).Set(head.Number),
				Details: []string{
					fmt.Sprintf("provider %s returned %s", head.URL, head.Hash),
					fmt.Sprintf("provider %s returned %s", canonical.URL, canonical.Hash),
				},
			}
		}
	}
	return HeadResult{Number: new(big.Int).Set(canonical.Number), Hash: canonical.Hash.Hex()}, nil
}
