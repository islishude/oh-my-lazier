package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

const defaultBinanceBaseURL = "https://api.binance.com"

// BinanceClient reads public spot prices from Binance market-data endpoints.
type BinanceClient struct {
	baseURL    string
	httpClient *http.Client
}

// BinancePriceReader binds a Binance symbol to a reusable client.
type BinancePriceReader struct {
	client *BinanceClient
	symbol string
}

// NewBinanceClient creates a Binance public market-data client.
func NewBinanceClient(baseURL string, httpClient *http.Client) *BinanceClient {
	if baseURL == "" {
		baseURL = defaultBinanceBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BinanceClient{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

// NewBinancePriceReader creates a configured Binance symbol reader.
func NewBinancePriceReader(client *BinanceClient, symbol string) (*BinancePriceReader, error) {
	if client == nil {
		return nil, errors.New("binance client is required")
	}
	if symbol == "" {
		return nil, errors.New("binance symbol is required")
	}
	return &BinancePriceReader{client: client, symbol: symbol}, nil
}

// PriceUSD fetches the configured symbol's latest USD/native price.
func (r *BinancePriceReader) PriceUSD(ctx context.Context) (SourcePrice, error) {
	return r.client.PriceUSD(ctx, r.symbol)
}

// PriceUSD fetches the latest symbol price as USD/native.
func (c *BinanceClient) PriceUSD(ctx context.Context, symbol string) (SourcePrice, error) {
	if symbol == "" {
		return SourcePrice{}, errors.New("binance symbol is required")
	}
	endpoint, err := url.Parse(c.baseURL + "/api/v3/ticker/price")
	if err != nil {
		return SourcePrice{}, err
	}
	query := endpoint.Query()
	query.Set("symbol", strings.ToUpper(symbol))
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return SourcePrice{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return SourcePrice{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return SourcePrice{}, fmt.Errorf("binance price request returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return SourcePrice{}, err
	}
	price, ok := new(big.Rat).SetString(payload.Price)
	if !ok || price.Sign() <= 0 {
		return SourcePrice{}, fmt.Errorf("binance returned invalid price %q", payload.Price)
	}
	return SourcePrice{Source: "binance", USD: price}, nil
}
