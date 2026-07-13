package pricing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestCoinMarketCapClientPriceUSD(t *testing.T) {
	t.Setenv("CMC_API_KEY", "test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/cryptocurrency/quotes/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("id") != "1027" {
			t.Fatalf("id = %s", r.URL.Query().Get("id"))
		}
		if r.URL.Query().Get("convert") != "USD" {
			t.Fatalf("convert = %s", r.URL.Query().Get("convert"))
		}
		if r.Header.Get("X-CMC_PRO_API_KEY") != "test-key" {
			t.Fatalf("api key header = %q", r.Header.Get("X-CMC_PRO_API_KEY"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":1027,"quote":[{"symbol":"USD","price":2000.125,"last_updated":"2023-11-14T22:13:20Z"}]}]}`))
	}))
	defer server.Close()

	client, err := NewCoinMarketCapClient(server.URL, "CMC_API_KEY", server.Client())
	if err != nil {
		t.Fatalf("NewCoinMarketCapClient() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background(), 1027)
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.Source != "coinmarketcap" {
		t.Fatalf("source = %q", price.Source)
	}
	if price.USD.Cmp(big.NewRat(16001, 8)) != 0 {
		t.Fatalf("price = %s, want 2000.125", price.USD)
	}
	if !price.ObservedAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("observed at = %s", price.ObservedAt)
	}
}

func TestCoinGeckoClientPriceUSD(t *testing.T) {
	t.Setenv("CG_API_KEY", "test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/simple/price" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("ids") != "ethereum" {
			t.Fatalf("ids = %s", r.URL.Query().Get("ids"))
		}
		if r.URL.Query().Get("vs_currencies") != "usd" {
			t.Fatalf("vs_currencies = %s", r.URL.Query().Get("vs_currencies"))
		}
		if r.URL.Query().Get("include_last_updated_at") != "true" {
			t.Fatalf("include_last_updated_at = %s", r.URL.Query().Get("include_last_updated_at"))
		}
		if r.Header.Get("x-cg-pro-api-key") != "test-key" {
			t.Fatalf("api key header = %q", r.Header.Get("x-cg-pro-api-key"))
		}
		_, _ = w.Write([]byte(`{"ethereum":{"usd":2000.125,"last_updated_at":1700000000}}`))
	}))
	defer server.Close()

	client, err := NewCoinGeckoClient(server.URL, "CG_API_KEY", server.Client())
	if err != nil {
		t.Fatalf("NewCoinGeckoClient() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background(), "ethereum")
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.Source != "coingecko" {
		t.Fatalf("source = %q", price.Source)
	}
	if price.USD.Cmp(big.NewRat(16001, 8)) != 0 {
		t.Fatalf("price = %s, want 2000.125", price.USD)
	}
	if !price.ObservedAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("observed at = %s", price.ObservedAt)
	}
}

func TestNewCoinGeckoClientSelectsEndpointForAuthenticationMode(t *testing.T) {
	t.Setenv("CG_API_KEY", "test-key")
	tests := []struct {
		name      string
		apiKeyEnv string
		wantURL   string
	}{
		{name: "public without key", wantURL: defaultCoinGeckoBaseURL},
		{name: "pro with key", apiKeyEnv: "CG_API_KEY", wantURL: defaultCoinGeckoProBaseURL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewCoinGeckoClient("", test.apiKeyEnv, http.DefaultClient)
			if err != nil {
				t.Fatalf("NewCoinGeckoClient() error = %v", err)
			}
			if client.baseURL != test.wantURL {
				t.Fatalf("base URL = %q, want %q", client.baseURL, test.wantURL)
			}
		})
	}
}

func TestMarketDataClientTransportErrorsRedactConfiguredBaseURL(t *testing.T) {
	secretBaseURL := "https://pricing-user:pricing-password@pricing-secret.example/private-api-key"
	transportCause := errors.New("transport cause")
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport failed for %s: %w", request.URL.String(), transportCause)
	})}
	tests := []struct {
		name   string
		source string
		price  func() error
	}{
		{
			name:   "coinmarketcap",
			source: "coinmarketcap",
			price: func() error {
				client, err := NewCoinMarketCapClient(secretBaseURL, "", httpClient)
				if err != nil {
					return err
				}
				_, err = client.PriceUSD(context.Background(), 1027)
				return err
			},
		},
		{
			name:   "coingecko",
			source: "coingecko",
			price: func() error {
				client, err := NewCoinGeckoClient(secretBaseURL, "", httpClient)
				if err != nil {
					return err
				}
				_, err = client.PriceUSD(context.Background(), "ethereum")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.price()
			if err == nil {
				t.Fatal("PriceUSD() error = nil, want transport error")
			}
			if !errors.Is(err, transportCause) {
				t.Fatalf("errors.Is(%v, transportCause) = false", err)
			}
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			logger.Error("pricing request failed", "error", err)
			output := err.Error() + logs.String()
			for _, secret := range []string{
				secretBaseURL,
				"pricing-user",
				"pricing-password",
				"pricing-secret.example",
				"private-api-key",
				"transport failed",
			} {
				if strings.Contains(output, secret) {
					t.Fatalf("pricing error leaked %q: %s", secret, output)
				}
			}
			for _, want := range []string{test.source, "price request execute failed"} {
				if !strings.Contains(output, want) {
					t.Fatalf("pricing error = %q, want %q", output, want)
				}
			}
		})
	}
}

func TestChainlinkClientPriceUSD(t *testing.T) {
	caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
	caller.addResponse(t, chainlinkAggregatorV3ABI.Methods["description"].ID, chainlinkAggregatorV3ABI.Methods["description"].Outputs, "ETH / USD")
	caller.addResponse(t, chainlinkAggregatorV3ABI.Methods["decimals"].ID, chainlinkAggregatorV3ABI.Methods["decimals"].Outputs, uint8(8))
	caller.addResponse(t, chainlinkAggregatorV3ABI.Methods["latestRoundData"].ID, chainlinkAggregatorV3ABI.Methods["latestRoundData"].Outputs,
		big.NewInt(10), big.NewInt(200_000_000_000), big.NewInt(1_699_999_900), big.NewInt(1_700_000_000), big.NewInt(10))
	client, err := NewChainlinkClient(caller, ChainlinkConfig{
		FeedAddress:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ExpectedDescription: "ETH / USD",
	})
	if err != nil {
		t.Fatalf("NewChainlinkClient() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background())
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.USD.Cmp(big.NewRat(2000, 1)) != 0 || !price.ObservedAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("price = %s at %s", price.USD, price.ObservedAt)
	}
}

func TestUniswapV3ClientPriceUSD(t *testing.T) {
	tokenIn := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
	caller.addResponse(t, uniswapV3PoolABI.Methods["token0"].ID, uniswapV3PoolABI.Methods["token0"].Outputs, tokenIn)
	caller.addResponse(t, uniswapV3PoolABI.Methods["token1"].ID, uniswapV3PoolABI.Methods["token1"].Outputs, tokenOut)
	caller.addResponse(t, erc20MetadataABI.Methods["decimals"].ID, erc20MetadataABI.Methods["decimals"].Outputs, uint8(18))
	caller.addResponse(t, uniswapV3PoolABI.Methods["observe"].ID, uniswapV3PoolABI.Methods["observe"].Outputs,
		[]*big.Int{big.NewInt(0), big.NewInt(0)}, []*big.Int{big.NewInt(0), big.NewInt(1)})
	client, err := NewUniswapV3Client(caller, caller, UniswapV3Config{
		PoolAddress:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		TokenIn:                  tokenIn,
		TokenOut:                 tokenOut,
		TWAPWindowSeconds:        60,
		MinHarmonicMeanLiquidity: big.NewInt(1),
	})
	if err != nil {
		t.Fatalf("NewUniswapV3Client() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background())
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.USD.Cmp(big.NewRat(1, 1)) != 0 || !price.ObservedAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("price = %s at %s, want 1", price.USD, price.ObservedAt)
	}
}

func TestQuoteAtTickHandlesTokenOrder(t *testing.T) {
	lower := common.HexToAddress("0x1111111111111111111111111111111111111111")
	higher := common.HexToAddress("0x2222222222222222222222222222222222222222")
	amount := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	for _, pair := range [][2]common.Address{{lower, higher}, {higher, lower}} {
		quote, err := quoteAtTick(0, amount, pair[0], pair[1])
		if err != nil {
			t.Fatalf("quoteAtTick() error = %v", err)
		}
		if quote.Cmp(amount) != 0 {
			t.Fatalf("quote = %s, want %s", quote, amount)
		}
	}
}

func TestUniswapObserveRoundsNegativeTickDown(t *testing.T) {
	caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
	caller.addResponse(t, uniswapV3PoolABI.Methods["observe"].ID, uniswapV3PoolABI.Methods["observe"].Outputs,
		[]*big.Int{big.NewInt(0), big.NewInt(-61)}, []*big.Int{big.NewInt(0), big.NewInt(1)})
	client := &UniswapV3Client{
		caller: caller,
		pool:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
		window: 60,
	}
	meanTick, _, err := client.observe(context.Background())
	if err != nil {
		t.Fatalf("observe() error = %v", err)
	}
	if meanTick != -2 {
		t.Fatalf("mean tick = %d, want -2", meanTick)
	}
}

func TestQuoteAtTickOrdersPositiveAndNegativeTicks(t *testing.T) {
	lower := common.HexToAddress("0x1111111111111111111111111111111111111111")
	higher := common.HexToAddress("0x2222222222222222222222222222222222222222")
	amount := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	negative, err := quoteAtTick(-1, amount, lower, higher)
	if err != nil {
		t.Fatalf("quoteAtTick(-1) error = %v", err)
	}
	positive, err := quoteAtTick(1, amount, lower, higher)
	if err != nil {
		t.Fatalf("quoteAtTick(1) error = %v", err)
	}
	if negative.Cmp(amount) >= 0 || positive.Cmp(amount) <= 0 {
		t.Fatalf("negative=%s amount=%s positive=%s", negative, amount, positive)
	}
}

func TestUniswapV3ClientAppliesTokenDecimals(t *testing.T) {
	tokenIn := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
	caller.addResponse(t, uniswapV3PoolABI.Methods["token0"].ID, uniswapV3PoolABI.Methods["token0"].Outputs, tokenIn)
	caller.addResponse(t, uniswapV3PoolABI.Methods["token1"].ID, uniswapV3PoolABI.Methods["token1"].Outputs, tokenOut)
	caller.addResponseTo(t, tokenIn, erc20MetadataABI.Methods["decimals"].ID, erc20MetadataABI.Methods["decimals"].Outputs, uint8(18))
	caller.addResponseTo(t, tokenOut, erc20MetadataABI.Methods["decimals"].ID, erc20MetadataABI.Methods["decimals"].Outputs, uint8(6))
	caller.addResponse(t, uniswapV3PoolABI.Methods["observe"].ID, uniswapV3PoolABI.Methods["observe"].Outputs,
		[]*big.Int{big.NewInt(0), big.NewInt(0)}, []*big.Int{big.NewInt(0), big.NewInt(1)})
	client, err := NewUniswapV3Client(caller, caller, UniswapV3Config{
		PoolAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenIn: tokenIn, TokenOut: tokenOut,
		TWAPWindowSeconds: 60, MinHarmonicMeanLiquidity: big.NewInt(1),
	})
	if err != nil {
		t.Fatalf("NewUniswapV3Client() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background())
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.USD.Cmp(big.NewRat(1_000_000_000_000, 1)) != 0 {
		t.Fatalf("price = %s, want 1000000000000", price.USD)
	}
}

func TestUniswapV3ClientRejectsLowHarmonicLiquidity(t *testing.T) {
	tokenIn := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenOut := common.HexToAddress("0x3333333333333333333333333333333333333333")
	caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
	caller.addResponse(t, uniswapV3PoolABI.Methods["token0"].ID, uniswapV3PoolABI.Methods["token0"].Outputs, tokenIn)
	caller.addResponse(t, uniswapV3PoolABI.Methods["token1"].ID, uniswapV3PoolABI.Methods["token1"].Outputs, tokenOut)
	caller.addResponse(t, erc20MetadataABI.Methods["decimals"].ID, erc20MetadataABI.Methods["decimals"].Outputs, uint8(18))
	maxUint160 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1))
	liquidityDelta := new(big.Int).Rsh(new(big.Int).Set(maxUint160), 32)
	caller.addResponse(t, uniswapV3PoolABI.Methods["observe"].ID, uniswapV3PoolABI.Methods["observe"].Outputs,
		[]*big.Int{big.NewInt(0), big.NewInt(0)}, []*big.Int{big.NewInt(0), liquidityDelta})
	client, err := NewUniswapV3Client(caller, caller, UniswapV3Config{
		PoolAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"), TokenIn: tokenIn, TokenOut: tokenOut,
		TWAPWindowSeconds: 60, MinHarmonicMeanLiquidity: big.NewInt(61),
	})
	if err != nil {
		t.Fatalf("NewUniswapV3Client() error = %v", err)
	}
	if _, err := client.PriceUSD(context.Background()); err == nil || !strings.Contains(err.Error(), "harmonic mean liquidity") {
		t.Fatalf("PriceUSD() error = %v, want liquidity error", err)
	}
}

type fakeEVMReader struct {
	responses       map[string][]byte
	targetResponses map[string][]byte
	header          *gethtypes.Header
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func newFakeEVMReader(t *testing.T, observedAt time.Time) *fakeEVMReader {
	t.Helper()
	return &fakeEVMReader{
		responses: make(map[string][]byte), targetResponses: make(map[string][]byte),
		header: &gethtypes.Header{Time: uint64(observedAt.Unix())},
	}
}

func (c *fakeEVMReader) addResponseTo(t *testing.T, target common.Address, selector []byte, outputs interface {
	Pack(...any) ([]byte, error)
}, values ...any) {
	t.Helper()
	encoded, err := outputs.Pack(values...)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	c.targetResponses[target.Hex()+string(selector)] = encoded
}

func (c *fakeEVMReader) addResponse(t *testing.T, selector []byte, outputs interface {
	Pack(...any) ([]byte, error)
}, values ...any) {
	t.Helper()
	encoded, err := outputs.Pack(values...)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	c.responses[string(selector)] = encoded
}

func (c *fakeEVMReader) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if len(call.Data) < 4 {
		return nil, errors.New("missing selector")
	}
	response, ok := c.targetResponses[call.To.Hex()+string(call.Data[:4])]
	if !ok {
		response, ok = c.responses[string(call.Data[:4])]
	}
	if !ok {
		return nil, fmt.Errorf("unexpected selector 0x%x", call.Data[:4])
	}
	return bytes.Clone(response), nil
}

func (c *fakeEVMReader) HeaderByNumber(context.Context, *big.Int) (*gethtypes.Header, error) {
	return c.header, nil
}
