package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	defaultCoinMarketCapBaseURL = "https://pro-api.coinmarketcap.com"
	defaultCoinGeckoBaseURL     = "https://api.coingecko.com"
)

// CoinMarketCapClient reads public USD prices from CoinMarketCap quotes.
type CoinMarketCapClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// CoinMarketCapPriceReader binds a CoinMarketCap symbol to a reusable client.
type CoinMarketCapPriceReader struct {
	client *CoinMarketCapClient
	symbol string
}

// CoinGeckoClient reads public USD prices from CoinGecko simple price.
type CoinGeckoClient struct {
	baseURL    string
	httpClient *http.Client
}

// CoinGeckoPriceReader binds a CoinGecko coin id to a reusable client.
type CoinGeckoPriceReader struct {
	client *CoinGeckoClient
	coinID string
}

// NewCoinMarketCapClient creates a CoinMarketCap quote client.
func NewCoinMarketCapClient(baseURL, apiKeyEnv string, httpClient *http.Client) (*CoinMarketCapClient, error) {
	if baseURL == "" {
		baseURL = defaultCoinMarketCapBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	apiKey := ""
	if apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
		if apiKey == "" {
			return nil, fmt.Errorf("coinmarketcap api key env %s is empty", apiKeyEnv)
		}
	}
	return &CoinMarketCapClient{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, httpClient: httpClient}, nil
}

// NewCoinMarketCapPriceReader creates a configured CoinMarketCap symbol reader.
func NewCoinMarketCapPriceReader(client *CoinMarketCapClient, symbol string) (*CoinMarketCapPriceReader, error) {
	if client == nil {
		return nil, errors.New("coinmarketcap client is required")
	}
	if symbol == "" {
		return nil, errors.New("coinmarketcap symbol is required")
	}
	return &CoinMarketCapPriceReader{client: client, symbol: symbol}, nil
}

// PriceUSD fetches the configured symbol's latest USD/native price.
func (r *CoinMarketCapPriceReader) PriceUSD(ctx context.Context) (SourcePrice, error) {
	return r.client.PriceUSD(ctx, r.symbol)
}

// PriceUSD fetches the latest symbol price as USD/native.
func (c *CoinMarketCapClient) PriceUSD(ctx context.Context, symbol string) (SourcePrice, error) {
	if symbol == "" {
		return SourcePrice{}, errors.New("coinmarketcap symbol is required")
	}
	endpoint, err := url.Parse(c.baseURL + "/v2/cryptocurrency/quotes/latest")
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coinmarketcap", "build", err)
	}
	query := endpoint.Query()
	query.Set("symbol", strings.ToUpper(symbol))
	query.Set("convert", "USD")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coinmarketcap", "build", err)
	}
	if c.apiKey != "" {
		request.Header.Set("X-CMC_PRO_API_KEY", c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coinmarketcap", "execute", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return SourcePrice{}, fmt.Errorf("coinmarketcap price request returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		Data map[string][]struct {
			Quote map[string]struct {
				Price json.Number `json:"price"`
			} `json:"quote"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return SourcePrice{}, err
	}
	entries := payload.Data[strings.ToUpper(symbol)]
	if len(entries) == 0 {
		return SourcePrice{}, fmt.Errorf("coinmarketcap returned no price for %s", symbol)
	}
	priceText := entries[0].Quote["USD"].Price.String()
	price, ok := new(big.Rat).SetString(priceText)
	if !ok || price.Sign() <= 0 {
		return SourcePrice{}, fmt.Errorf("coinmarketcap returned invalid price %q", priceText)
	}
	return SourcePrice{Source: "coinmarketcap", USD: price}, nil
}

// NewCoinGeckoClient creates a CoinGecko simple-price client.
func NewCoinGeckoClient(baseURL string, httpClient *http.Client) *CoinGeckoClient {
	if baseURL == "" {
		baseURL = defaultCoinGeckoBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CoinGeckoClient{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

// NewCoinGeckoPriceReader creates a configured CoinGecko coin-id reader.
func NewCoinGeckoPriceReader(client *CoinGeckoClient, coinID string) (*CoinGeckoPriceReader, error) {
	if client == nil {
		return nil, errors.New("coingecko client is required")
	}
	if coinID == "" {
		return nil, errors.New("coingecko coin id is required")
	}
	return &CoinGeckoPriceReader{client: client, coinID: coinID}, nil
}

// PriceUSD fetches the configured coin id's latest USD/native price.
func (r *CoinGeckoPriceReader) PriceUSD(ctx context.Context) (SourcePrice, error) {
	return r.client.PriceUSD(ctx, r.coinID)
}

// PriceUSD fetches the latest coin id price as USD/native.
func (c *CoinGeckoClient) PriceUSD(ctx context.Context, coinID string) (SourcePrice, error) {
	if coinID == "" {
		return SourcePrice{}, errors.New("coingecko coin id is required")
	}
	endpoint, err := url.Parse(c.baseURL + "/api/v3/simple/price")
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coingecko", "build", err)
	}
	query := endpoint.Query()
	query.Set("ids", coinID)
	query.Set("vs_currencies", "usd")
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coingecko", "build", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return SourcePrice{}, wrapPriceSourceRequestError("coingecko", "execute", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return SourcePrice{}, fmt.Errorf("coingecko price request returned HTTP %d", response.StatusCode)
	}
	var payload map[string]map[string]json.Number
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return SourcePrice{}, err
	}
	priceText := payload[coinID]["usd"].String()
	price, ok := new(big.Rat).SetString(priceText)
	if !ok || price.Sign() <= 0 {
		return SourcePrice{}, fmt.Errorf("coingecko returned invalid price %q", priceText)
	}
	return SourcePrice{Source: "coingecko", USD: price}, nil
}
