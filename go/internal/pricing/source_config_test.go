package pricing

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func TestValidateSourceConfigurationsRejectsDeterministicMismatches(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T) ConfiguredPriceReader
		want  string
	}{
		{
			name: "coinmarketcap rejected id",
			build: func(t *testing.T) ConfiguredPriceReader {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				}))
				t.Cleanup(server.Close)
				client, err := NewCoinMarketCapClient(server.URL, "", server.Client())
				if err != nil {
					t.Fatalf("NewCoinMarketCapClient() error = %v", err)
				}
				reader, err := NewCoinMarketCapPriceReader(client, 999999999)
				if err != nil {
					t.Fatalf("NewCoinMarketCapPriceReader() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "coinmarketcap", Reader: reader}
			},
			want: "coinmarketcap price request returned HTTP 400",
		},
		{
			name: "coingecko missing id",
			build: func(t *testing.T) ConfiguredPriceReader {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(`{}`))
				}))
				t.Cleanup(server.Close)
				client, err := NewCoinGeckoClient(server.URL, "", server.Client())
				if err != nil {
					t.Fatalf("NewCoinGeckoClient() error = %v", err)
				}
				reader, err := NewCoinGeckoPriceReader(client, "missing-asset")
				if err != nil {
					t.Fatalf("NewCoinGeckoPriceReader() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "coingecko", Reader: reader}
			},
			want: "coingecko returned no price for missing-asset",
		},
		{
			name: "chainlink description mismatch",
			build: func(t *testing.T) ConfiguredPriceReader {
				caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
				caller.addResponse(t, chainlinkAggregatorV3ABI.Methods["description"].ID, chainlinkAggregatorV3ABI.Methods["description"].Outputs, "BTC / USD")
				reader, err := NewChainlinkClient(caller, caller, ChainlinkConfig{
					FeedAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"), ExpectedDescription: "ETH / USD",
				})
				if err != nil {
					t.Fatalf("NewChainlinkClient() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "chainlink", Reader: reader}
			},
			want: `chainlink description "BTC / USD" does not match expected "ETH / USD"`,
		},
		{
			name: "uniswap token pair mismatch",
			build: func(t *testing.T) ConfiguredPriceReader {
				caller := newFakeEVMReader(t, time.Unix(1_700_000_000, 0))
				caller.addResponse(t, uniswapV3PoolABI.Methods["token0"].ID, uniswapV3PoolABI.Methods["token0"].Outputs, common.HexToAddress("0x4444444444444444444444444444444444444444"))
				caller.addResponse(t, uniswapV3PoolABI.Methods["token1"].ID, uniswapV3PoolABI.Methods["token1"].Outputs, common.HexToAddress("0x5555555555555555555555555555555555555555"))
				reader, err := NewUniswapV3Client(caller, caller, UniswapV3Config{
					PoolAddress:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
					TokenIn:                  common.HexToAddress("0x2222222222222222222222222222222222222222"),
					TokenOut:                 common.HexToAddress("0x3333333333333333333333333333333333333333"),
					TWAPWindowSeconds:        uint32(config.MinUniswapTWAPWindowSeconds),
					MinHarmonicMeanLiquidity: big.NewInt(1),
				})
				if err != nil {
					t.Fatalf("NewUniswapV3Client() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "uniswap", Reader: reader}
			},
			want: "uniswap pool tokens do not match configured token pair",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := test.build(t)
			err := ValidateSourceConfigurations(t.Context(), map[uint32]ChainSources{
				40161: {Primary: reader},
			}, PriceSelectionPolicy{SourceRequestTimeout: time.Second})
			if err == nil {
				t.Fatal("ValidateSourceConfigurations() error = nil, want deterministic configuration error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSourceConfigurations() error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestValidateSourceConfigurationsDefersTransientFailures(t *testing.T) {
	tests := []struct {
		name    string
		build   func(*testing.T) ConfiguredPriceReader
		timeout time.Duration
		want    string
	}{
		{
			name: "coinmarketcap server failure",
			build: func(t *testing.T) ConfiguredPriceReader {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				t.Cleanup(server.Close)
				client, err := NewCoinMarketCapClient(server.URL, "", server.Client())
				if err != nil {
					t.Fatalf("NewCoinMarketCapClient() error = %v", err)
				}
				reader, err := NewCoinMarketCapPriceReader(client, 1027)
				if err != nil {
					t.Fatalf("NewCoinMarketCapPriceReader() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "coinmarketcap", Reader: reader}
			},
			timeout: time.Second,
			want:    "unavailable",
		},
		{
			name: "coingecko rate limit",
			build: func(t *testing.T) ConfiguredPriceReader {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
				}))
				t.Cleanup(server.Close)
				client, err := NewCoinGeckoClient(server.URL, "", server.Client())
				if err != nil {
					t.Fatalf("NewCoinGeckoClient() error = %v", err)
				}
				reader, err := NewCoinGeckoPriceReader(client, "ethereum")
				if err != nil {
					t.Fatalf("NewCoinGeckoPriceReader() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "coingecko", Reader: reader}
			},
			timeout: time.Second,
			want:    "unavailable",
		},
		{
			name: "chainlink rpc failure",
			build: func(t *testing.T) ConfiguredPriceReader {
				reader, err := NewChainlinkClient(unavailableEVMReader{}, unavailableEVMReader{}, ChainlinkConfig{
					FeedAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"), ExpectedDescription: "ETH / USD",
				})
				if err != nil {
					t.Fatalf("NewChainlinkClient() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "chainlink", Reader: reader}
			},
			timeout: time.Second,
			want:    "unavailable",
		},
		{
			name: "uniswap rpc failure",
			build: func(t *testing.T) ConfiguredPriceReader {
				reader, err := NewUniswapV3Client(unavailableEVMReader{}, unavailableEVMReader{}, UniswapV3Config{
					PoolAddress:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
					TokenIn:                  common.HexToAddress("0x2222222222222222222222222222222222222222"),
					TokenOut:                 common.HexToAddress("0x3333333333333333333333333333333333333333"),
					TWAPWindowSeconds:        uint32(config.MinUniswapTWAPWindowSeconds),
					MinHarmonicMeanLiquidity: big.NewInt(1),
				})
				if err != nil {
					t.Fatalf("NewUniswapV3Client() error = %v", err)
				}
				return ConfiguredPriceReader{Name: "uniswap", Reader: reader}
			},
			timeout: time.Second,
			want:    "unavailable",
		},
		{
			name: "timeout",
			build: func(*testing.T) ConfiguredPriceReader {
				return ConfiguredPriceReader{Name: "blocking", Reader: blockingSourceConfigurationReader{}}
			},
			timeout: 20 * time.Millisecond,
			want:    "timeout",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := test.build(t)
			var failures []PriceSourceFailure
			err := ValidateSourceConfigurations(t.Context(), map[uint32]ChainSources{
				40161: {Primary: reader},
			}, PriceSelectionPolicy{
				SourceRequestTimeout: test.timeout,
				OnSourceFailure: func(failure PriceSourceFailure) {
					failures = append(failures, failure)
				},
			})
			if err != nil {
				t.Fatalf("ValidateSourceConfigurations() error = %v, want transient failure deferred", err)
			}
			if len(failures) != 1 || failures[0].Source != reader.Name || failures[0].Category != test.want {
				t.Fatalf("source failures = %#v, want source=%s category=%s", failures, reader.Name, test.want)
			}
		})
	}
}

func TestValidateSourceConfigurationsPropagatesParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := ValidateSourceConfigurations(ctx, map[uint32]ChainSources{
		40161: {Primary: ConfiguredPriceReader{Name: "blocking", Reader: blockingSourceConfigurationReader{}}},
	}, PriceSelectionPolicy{SourceRequestTimeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateSourceConfigurations() error = %v, want context canceled", err)
	}
}

type unavailableEVMReader struct{}

func (unavailableEVMReader) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return nil, errors.New("rpc unavailable")
}

func (unavailableEVMReader) HeaderByNumber(context.Context, *big.Int) (*gethtypes.Header, error) {
	return nil, errors.New("rpc unavailable")
}

type blockingSourceConfigurationReader struct{}

func (blockingSourceConfigurationReader) PriceUSD(context.Context) (SourcePrice, error) {
	return SourcePrice{}, errors.New("price read is not used by source configuration validation")
}

func (blockingSourceConfigurationReader) validateSourceConfiguration(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
