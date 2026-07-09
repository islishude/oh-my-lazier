package rpcquorum

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var _ interface {
	BlockNumber(context.Context) (uint64, error)
	BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
	ChainID(context.Context) (*big.Int, error)
	CheckHead(context.Context) (HeadResult, error)
	CodeAt(context.Context, common.Address, *big.Int) ([]byte, error)
	EstimateGas(context.Context, ethereum.CallMsg) (uint64, error)
	FilterLogs(context.Context, ethereum.FilterQuery) ([]gethtypes.Log, error)
	HeaderByNumber(context.Context, *big.Int) (*gethtypes.Header, error)
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	SendTransaction(context.Context, *gethtypes.Transaction) error
	SuggestGasPrice(context.Context) (*big.Int, error)
	SuggestGasTipCap(context.Context) (*big.Int, error)
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
	client *ethclient.Client
}

// Client coordinates multiple RPC providers for one chain.
type Client struct {
	chainName string
	mu        sync.Mutex
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

// ChainIDMismatchError reports configured RPC providers that do not match the expected EVM chain ID.
type ChainIDMismatchError struct {
	ChainName string
	Expected  *big.Int
	Details   []string
}

// Error returns provider chain ID mismatch details.
func (e *ChainIDMismatchError) Error() string {
	if e == nil {
		return "rpc chain_id mismatch"
	}
	expected := "<unknown>"
	if e.Expected != nil {
		expected = e.Expected.String()
	}
	if len(e.Details) == 0 {
		return fmt.Sprintf("rpc chain_id mismatch for chain %s, expected %s", e.ChainName, expected)
	}
	return fmt.Sprintf("rpc chain_id mismatch for chain %s, expected %s: %s", e.ChainName, expected, strings.Join(e.Details, "; "))
}

// IsChainIDMismatch reports whether err is a provider chain ID mismatch.
func IsChainIDMismatch(err error) bool {
	var mismatch *ChainIDMismatchError
	return errors.As(err, &mismatch)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Provider, len(c.providers))
	copy(out, c.providers)
	for i := range out {
		out[i].client = nil
	}
	return out
}

// Close releases cached RPC provider connections.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.providers {
		if c.providers[i].client != nil {
			c.providers[i].client.Close()
			c.providers[i].client = nil
		}
	}
}

// CheckHead verifies provider head agreement and returns the selected head.
func (c *Client) CheckHead(ctx context.Context) (HeadResult, error) {
	if len(c.providers) == 0 {
		return HeadResult{}, errors.New("no rpc providers configured")
	}
	heads := make([]providerHead, 0, len(c.providers))
	var transientErrs []error
	for index, provider := range c.snapshotProviders() {
		header, err := c.headerByNumberFromProvider(ctx, index, nil)
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
		c.updateHeadProviderStatuses(heads, HeadResult{})
		if len(heads) == 0 && len(transientErrs) > 0 {
			return HeadResult{}, errors.Join(transientErrs...)
		}
		return HeadResult{}, err
	}
	c.updateHeadProviderStatuses(heads, result)
	return result, nil
}

// CallContract performs an eth_call against the first currently healthy provider.
func (c *Client) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.CallContract(ctx, call, blockNumber)
}

// BlockNumber returns the latest block number from the first currently healthy provider.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return 0, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return 0, err
	}
	return client.BlockNumber(ctx)
}

// ChainID returns the first healthy provider's native EVM chain ID.
func (c *Client) ChainID(ctx context.Context) (*big.Int, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.ChainID(ctx)
}

// BalanceAt returns the first healthy provider's native token balance for an account.
func (c *Client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.BalanceAt(ctx, account, blockNumber)
}

// ValidateChainID verifies every configured provider reports the expected EVM chain ID.
func (c *Client) ValidateChainID(ctx context.Context, expected *big.Int) error {
	if expected == nil {
		return errors.New("expected chain id is required")
	}
	providers := c.snapshotProviders()
	if len(providers) == 0 {
		return errors.New("no rpc providers configured")
	}
	ids := make([]providerChainID, 0, len(providers))
	var providerErrs []error
	for index, provider := range providers {
		chainID, err := c.chainIDFromProvider(ctx, index)
		if err != nil {
			providerErrs = append(providerErrs, fmt.Errorf("%s: %w", provider.URL, err))
			continue
		}
		ids = append(ids, providerChainID{URL: provider.URL, ChainID: chainID})
	}
	if len(providerErrs) > 0 {
		return errors.Join(providerErrs...)
	}
	return validateProviderChainIDs(c.chainName, expected, ids)
}

// CodeAt returns contract code at the first healthy provider's selected block.
func (c *Client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.CodeAt(ctx, account, blockNumber)
}

// EstimateGas returns the first healthy provider's gas limit estimate.
func (c *Client) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return 0, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return 0, err
	}
	return client.EstimateGas(ctx, call)
}

// FilterLogs returns logs from the first currently healthy provider for a bounded query.
func (c *Client) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.FilterLogs(ctx, query)
}

// SuggestGasPrice returns the first healthy provider's legacy gas price estimate.
func (c *Client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.SuggestGasPrice(ctx)
}

// SuggestGasTipCap returns the first healthy provider's EIP-1559 priority-fee estimate.
func (c *Client) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.SuggestGasTipCap(ctx)
}

// HeaderByNumber returns a block header from the first currently healthy provider.
func (c *Client) HeaderByNumber(ctx context.Context, number *big.Int) (*gethtypes.Header, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return nil, err
	}
	return c.headerByNumberFromProvider(ctx, index, number)
}

// PendingNonceAt returns the first healthy provider's pending account nonce.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return 0, err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return 0, err
	}
	return client.PendingNonceAt(ctx, account)
}

// SendTransaction broadcasts a signed transaction through the first healthy provider.
func (c *Client) SendTransaction(ctx context.Context, tx *gethtypes.Transaction) error {
	index, err := c.firstHealthyProvider()
	if err != nil {
		return err
	}
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return err
	}
	return client.SendTransaction(ctx, tx)
}

// TransactionReceipt returns a receipt only when healthy providers agree on the receipt.
func (c *Client) TransactionReceipt(ctx context.Context, txHash common.Hash) (*gethtypes.Receipt, error) {
	var canonical *gethtypes.Receipt
	var canonicalFingerprint string
	var transientErrs []error
	var notFoundProviders []string
	for index, provider := range c.snapshotProviders() {
		if provider.Status != ProviderHealthy {
			continue
		}
		receipt, err := c.transactionReceiptFromProvider(ctx, index, txHash)
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

func (c *Client) transactionReceiptFromProvider(ctx context.Context, index int, txHash common.Hash) (*gethtypes.Receipt, error) {
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.TransactionReceipt(ctx, txHash)
}

func (c *Client) chainIDFromProvider(ctx context.Context, index int) (*big.Int, error) {
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.ChainID(ctx)
}

func (c *Client) headerByNumberFromProvider(ctx context.Context, index int, number *big.Int) (*gethtypes.Header, error) {
	client, err := c.providerClient(ctx, index)
	if err != nil {
		return nil, err
	}
	return client.HeaderByNumber(ctx, number)
}

func (c *Client) firstHealthyProvider() (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, provider := range c.providers {
		if provider.Status == ProviderHealthy {
			return index, nil
		}
	}
	return 0, errors.New("no healthy rpc providers configured")
}

func (c *Client) snapshotProviders() []Provider {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Provider, len(c.providers))
	copy(out, c.providers)
	return out
}

func (c *Client) providerClient(ctx context.Context, index int) (*ethclient.Client, error) {
	c.mu.Lock()
	if index < 0 || index >= len(c.providers) {
		c.mu.Unlock()
		return nil, fmt.Errorf("provider index %d out of range", index)
	}
	if c.providers[index].client != nil {
		client := c.providers[index].client
		c.mu.Unlock()
		return client, nil
	}
	url := c.providers[index].URL
	c.mu.Unlock()

	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.providers[index].client != nil {
		client.Close()
		return c.providers[index].client, nil
	}
	c.providers[index].client = client
	return client, nil
}

func (c *Client) updateHeadProviderStatuses(heads []providerHead, canonical HeadResult) {
	if c == nil {
		return
	}
	statusByURL := classifyHeadProviderStatuses(heads, canonical)
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, provider := range c.providers {
		status, ok := statusByURL[provider.URL]
		if !ok {
			continue
		}
		c.providers[index].Status = status
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

type providerChainID struct {
	URL     string
	ChainID *big.Int
}

func validateProviderChainIDs(chainName string, expected *big.Int, ids []providerChainID) error {
	if expected == nil {
		return errors.New("expected chain id is required")
	}
	if len(ids) == 0 {
		return errors.New("no rpc providers configured")
	}
	var details []string
	for _, item := range ids {
		switch {
		case item.ChainID == nil:
			details = append(details, fmt.Sprintf("provider %s returned <nil>", item.URL))
		case item.ChainID.Cmp(expected) != 0:
			details = append(details, fmt.Sprintf("provider %s returned %s", item.URL, item.ChainID))
		}
	}
	if len(details) > 0 {
		return &ChainIDMismatchError{
			ChainName: chainName,
			Expected:  new(big.Int).Set(expected),
			Details:   details,
		}
	}
	return nil
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

func classifyHeadProviderStatuses(heads []providerHead, canonical HeadResult) map[string]ProviderStatus {
	statuses := make(map[string]ProviderStatus, len(heads))
	if len(heads) == 0 {
		return statuses
	}
	if canonical.Number != nil && canonical.Hash != "" {
		for _, head := range heads {
			if head.Number == nil {
				continue
			}
			switch {
			case head.Number.Cmp(canonical.Number) < 0:
				statuses[head.URL] = ProviderLagging
			case head.Number.Cmp(canonical.Number) == 0 && head.Hash.Hex() == canonical.Hash:
				statuses[head.URL] = ProviderHealthy
			case head.Number.Cmp(canonical.Number) == 0:
				statuses[head.URL] = ProviderConflict
			default:
				statuses[head.URL] = ProviderHealthy
			}
		}
		return statuses
	}
	var maxNumber *big.Int
	hashesAtMax := make(map[common.Hash]struct{})
	for _, head := range heads {
		if head.Number == nil {
			continue
		}
		if maxNumber == nil || head.Number.Cmp(maxNumber) > 0 {
			maxNumber = new(big.Int).Set(head.Number)
			hashesAtMax = map[common.Hash]struct{}{head.Hash: {}}
			continue
		}
		if head.Number.Cmp(maxNumber) == 0 {
			hashesAtMax[head.Hash] = struct{}{}
		}
	}
	for _, head := range heads {
		if head.Number == nil || maxNumber == nil {
			continue
		}
		if head.Number.Cmp(maxNumber) < 0 {
			statuses[head.URL] = ProviderLagging
			continue
		}
		if len(hashesAtMax) > 1 {
			statuses[head.URL] = ProviderConflict
			continue
		}
		statuses[head.URL] = ProviderHealthy
	}
	return statuses
}
