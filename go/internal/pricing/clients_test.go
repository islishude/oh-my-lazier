package pricing

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func TestBinanceClientPriceUSD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/ticker/price" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("symbol") != "ETHUSDT" {
			t.Fatalf("symbol = %s", r.URL.Query().Get("symbol"))
		}
		_, _ = w.Write([]byte("{\"symbol\":\"ETHUSDT\",\"price\":\"2000.125\"}"))
	}))
	defer server.Close()

	client := NewBinanceClient(server.URL, server.Client())
	price, err := client.PriceUSD(context.Background(), "ethusdt")
	if err != nil {
		t.Fatalf("PriceUSD() error = %v", err)
	}
	if price.Source != "binance" {
		t.Fatalf("source = %q", price.Source)
	}
	if price.USD.Cmp(big.NewRat(16001, 8)) != 0 {
		t.Fatalf("price = %s, want 2000.125", price.USD)
	}
}

func TestUniswapV3ClientPriceUSD(t *testing.T) {
	amountOut := new(big.Int).Mul(big.NewInt(2000), big.NewInt(1_000_000))
	outputs := abi.Arguments{{Type: mustABIType(t, "uint256")}}
	encoded, err := outputs.Pack(amountOut)
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
}

type fakeCaller struct {
	to       *common.Address
	data     []byte
	response []byte
}

func (c *fakeCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.to = call.To
	c.data = append([]byte(nil), call.Data...)
	return c.response, nil
}

func mustABIType(t *testing.T, typ string) abi.Type {
	t.Helper()
	parsed, err := abi.NewType(typ, "", nil)
	if err != nil {
		t.Fatalf("NewType(%q) error = %v", typ, err)
	}
	return parsed
}
