package pricing

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/abiutil"
)

var (
	//go:embed abis/uniswap_v3_quoter.json
	uniswapV3QuoterABIJSON string

	uniswapV3QuoterABI = abiutil.MustParse(uniswapV3QuoterABIJSON)
)

// CallContractReader reads EVM call results.
type CallContractReader interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// UniswapV3Client reads native/USD sanity prices from a configured V3 QuoterV2.
type UniswapV3Client struct {
	caller      CallContractReader
	quoter      common.Address
	tokenIn     common.Address
	tokenOut    common.Address
	fee         uint32
	amountIn    *big.Int
	tokenOutDec uint8
}

// UniswapV3Config configures one native-token to stablecoin quote path.
type UniswapV3Config struct {
	QuoterAddress    common.Address
	TokenIn          common.Address
	TokenOut         common.Address
	Fee              uint32
	AmountIn         *big.Int
	TokenOutDecimals uint8
}

type quoteExactInputSingleParams struct {
	TokenIn           common.Address
	TokenOut          common.Address
	AmountIn          *big.Int
	Fee               *big.Int
	SqrtPriceLimitX96 *big.Int
}

// NewUniswapV3Client creates a Uniswap V3 QuoterV2-backed price client.
func NewUniswapV3Client(caller CallContractReader, cfg UniswapV3Config) (*UniswapV3Client, error) {
	if caller == nil {
		return nil, errors.New("uniswap caller is required")
	}
	if cfg.QuoterAddress == (common.Address{}) {
		return nil, errors.New("uniswap quoter address is required")
	}
	if cfg.TokenIn == (common.Address{}) || cfg.TokenOut == (common.Address{}) {
		return nil, errors.New("uniswap token addresses are required")
	}
	if cfg.Fee > (1<<24)-1 {
		return nil, errors.New("uniswap fee exceeds uint24")
	}
	if cfg.AmountIn == nil || cfg.AmountIn.Sign() <= 0 {
		return nil, errors.New("uniswap amount_in must be positive")
	}
	if cfg.TokenOutDecimals == 0 {
		return nil, errors.New("uniswap token_out decimals is required")
	}
	return &UniswapV3Client{
		caller:      caller,
		quoter:      cfg.QuoterAddress,
		tokenIn:     cfg.TokenIn,
		tokenOut:    cfg.TokenOut,
		fee:         cfg.Fee,
		amountIn:    new(big.Int).Set(cfg.AmountIn),
		tokenOutDec: cfg.TokenOutDecimals,
	}, nil
}

// PriceUSD fetches a native-token USD sanity price through QuoterV2 quoteExactInputSingle.
func (c *UniswapV3Client) PriceUSD(ctx context.Context) (SourcePrice, error) {
	calldata, err := c.quoteCalldata()
	if err != nil {
		return SourcePrice{}, err
	}
	result, err := c.caller.CallContract(ctx, ethereum.CallMsg{To: &c.quoter, Data: calldata}, nil)
	if err != nil {
		return SourcePrice{}, err
	}
	decoded, err := uniswapV3QuoterABI.Unpack("quoteExactInputSingle", result)
	if err != nil {
		return SourcePrice{}, err
	}
	if len(decoded) != 4 {
		return SourcePrice{}, fmt.Errorf("uniswap quote returned %d values", len(decoded))
	}
	amountOut, ok := decoded[0].(*big.Int)
	if !ok || amountOut.Sign() <= 0 {
		return SourcePrice{}, errors.New("uniswap quote returned invalid amount")
	}
	price := new(big.Rat).SetInt(amountOut)
	price.Quo(price, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(c.tokenOutDec)), nil)))
	price.Quo(price, new(big.Rat).SetInt(c.amountIn))
	price.Mul(price, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)))
	return SourcePrice{Source: "uniswap", USD: price}, nil
}

func (c *UniswapV3Client) quoteCalldata() ([]byte, error) {
	return uniswapV3QuoterABI.Pack("quoteExactInputSingle", quoteExactInputSingleParams{
		TokenIn:           c.tokenIn,
		TokenOut:          c.tokenOut,
		AmountIn:          c.amountIn,
		Fee:               new(big.Int).SetUint64(uint64(c.fee)),
		SqrtPriceLimitX96: new(big.Int),
	})
}
