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

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

func TestCoinMarketCapClientPriceUSD(t *testing.T) {
	t.Setenv("CMC_API_KEY", "test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/cryptocurrency/quotes/latest" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "ETH" {
			t.Fatalf("symbol = %s", r.URL.Query().Get("symbol"))
		}
		if r.URL.Query().Get("convert") != "USD" {
			t.Fatalf("convert = %s", r.URL.Query().Get("convert"))
		}
		if r.Header.Get("X-CMC_PRO_API_KEY") != "test-key" {
			t.Fatalf("api key header = %q", r.Header.Get("X-CMC_PRO_API_KEY"))
		}
		_, _ = w.Write([]byte(`{"data":{"ETH":[{"quote":{"USD":{"price":2000.125}}}]}}`))
	}))
	defer server.Close()

	client, err := NewCoinMarketCapClient(server.URL, "CMC_API_KEY", server.Client())
	if err != nil {
		t.Fatalf("NewCoinMarketCapClient() error = %v", err)
	}
	price, err := client.PriceUSD(context.Background(), "eth")
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.Source != "coinmarketcap" {
		t.Fatalf("source = %q", price.Source)
	}
	if price.USD.Cmp(big.NewRat(16001, 8)) != 0 {
		t.Fatalf("price = %s, want 2000.125", price.USD)
	}
}

func TestCoinGeckoClientPriceUSD(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"ethereum":{"usd":2000.125}}`))
	}))
	defer server.Close()

	client := NewCoinGeckoClient(server.URL, server.Client())
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
				_, err = client.PriceUSD(context.Background(), "ETH")
				return err
			},
		},
		{
			name:   "coingecko",
			source: "coingecko",
			price: func() error {
				_, err := NewCoinGeckoClient(secretBaseURL, httpClient).PriceUSD(context.Background(), "ethereum")
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

func TestUniswapV3ClientPriceUSD(t *testing.T) {
	amountOut := new(big.Int).Mul(big.NewInt(2000), big.NewInt(1_000_000))
	method := uniswapV3QuoterABI.Methods["quoteExactInputSingle"]
	if len(method.Inputs) != 1 || method.Inputs[0].Type.TupleElems == nil {
		t.Fatalf("quoteExactInputSingle input is not the QuoterV2 tuple shape")
	}
	if len(method.Outputs) != 4 {
		t.Fatalf("quoteExactInputSingle outputs = %d, want 4", len(method.Outputs))
	}
	encoded, err := method.Outputs.Pack(amountOut, new(big.Int), uint32(0), new(big.Int))
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}
	caller := &fakeCaller{response: encoded}
	client, err := NewUniswapV3Client(caller, UniswapV3Config{
		QuoterAddress:    common.HexToAddress("0x1111111111111111111111111111111111111111"),
		TokenIn:          common.HexToAddress("0x2222222222222222222222222222222222222222"),
		TokenOut:         common.HexToAddress("0x3333333333333333333333333333333333333333"),
		Fee:              500,
		AmountIn:         new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
		TokenOutDecimals: 6,
	})
	if err != nil {
		t.Fatalf("NewUniswapV3Client() error = %v", err)
	}

	price, err := client.PriceUSD(context.Background())
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.Source != "uniswap" {
		t.Fatalf("source = %q", price.Source)
	}
	if price.USD.Cmp(big.NewRat(2000, 1)) != 0 {
		t.Fatalf("price = %s, want 2000", price.USD)
	}
	if caller.to == nil || *caller.to != common.HexToAddress("0x1111111111111111111111111111111111111111") {
		t.Fatalf("call target = %v", caller.to)
	}
	if len(caller.data) == 0 {
		t.Fatal("call data is empty")
	}
	expectedCalldata, err := uniswapV3QuoterABI.Pack("quoteExactInputSingle", quoteExactInputSingleParams{
		TokenIn:           common.HexToAddress("0x2222222222222222222222222222222222222222"),
		TokenOut:          common.HexToAddress("0x3333333333333333333333333333333333333333"),
		AmountIn:          new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
		Fee:               big.NewInt(500),
		SqrtPriceLimitX96: new(big.Int),
	})
	if err != nil {
		t.Fatalf("Pack(expected calldata) error = %v", err)
	}
	if !bytes.Equal(caller.data, expectedCalldata) {
		t.Fatalf("calldata = %x, want %x", caller.data, expectedCalldata)
	}
}

type fakeCaller struct {
	to       *common.Address
	data     []byte
	response []byte
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (c *fakeCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.to = call.To
	c.data = bytes.Clone(call.Data)
	return c.response, nil
}
