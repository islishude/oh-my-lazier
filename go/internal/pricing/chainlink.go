package pricing

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/abiutil"
)

var (
	//go:embed abis/chainlink_aggregator_v3.json
	chainlinkAggregatorV3ABIJSON string

	chainlinkAggregatorV3ABI = abiutil.MustParse(chainlinkAggregatorV3ABIJSON)
)

// ChainlinkConfig configures one chain-local AggregatorV3 reader.
type ChainlinkConfig struct {
	FeedAddress         common.Address
	ExpectedDescription string
}

// ChainlinkClient reads one USD/native price from an AggregatorV3 proxy.
type ChainlinkClient struct {
	caller              CallContractReader
	feed                common.Address
	expectedDescription string
}

// NewChainlinkClient creates an AggregatorV3-backed price reader.
func NewChainlinkClient(caller CallContractReader, cfg ChainlinkConfig) (*ChainlinkClient, error) {
	if caller == nil {
		return nil, errors.New("chainlink caller is required")
	}
	if cfg.FeedAddress == (common.Address{}) {
		return nil, errors.New("chainlink feed address is required")
	}
	if cfg.ExpectedDescription == "" {
		return nil, errors.New("chainlink expected description is required")
	}
	return &ChainlinkClient{caller: caller, feed: cfg.FeedAddress, expectedDescription: cfg.ExpectedDescription}, nil
}

// PriceUSD reads and validates the latest AggregatorV3 USD/native observation.
func (c *ChainlinkClient) PriceUSD(ctx context.Context) (SourcePrice, error) {
	descriptionValues, err := c.call(ctx, "description")
	if err != nil {
		return SourcePrice{}, err
	}
	if len(descriptionValues) != 1 {
		return SourcePrice{}, fmt.Errorf("chainlink description returned %d values", len(descriptionValues))
	}
	description, ok := descriptionValues[0].(string)
	if !ok || description != c.expectedDescription {
		return SourcePrice{}, fmt.Errorf("chainlink description %q does not match expected %q", description, c.expectedDescription)
	}
	decimalValues, err := c.call(ctx, "decimals")
	if err != nil {
		return SourcePrice{}, err
	}
	if len(decimalValues) != 1 {
		return SourcePrice{}, fmt.Errorf("chainlink decimals returned %d values", len(decimalValues))
	}
	decimals, ok := decimalValues[0].(uint8)
	if !ok || decimals > 18 {
		return SourcePrice{}, errors.New("chainlink returned unsupported decimals")
	}
	roundValues, err := c.call(ctx, "latestRoundData")
	if err != nil {
		return SourcePrice{}, err
	}
	if len(roundValues) != 5 {
		return SourcePrice{}, fmt.Errorf("chainlink latestRoundData returned %d values", len(roundValues))
	}
	roundID, roundOK := roundValues[0].(*big.Int)
	answer, answerOK := roundValues[1].(*big.Int)
	updatedAt, updatedOK := roundValues[3].(*big.Int)
	answeredInRound, answeredOK := roundValues[4].(*big.Int)
	if !roundOK || !answerOK || !updatedOK || !answeredOK || answer.Sign() <= 0 || updatedAt.Sign() <= 0 {
		return SourcePrice{}, errors.New("chainlink returned invalid round data")
	}
	if answeredInRound.Cmp(roundID) < 0 {
		return SourcePrice{}, errors.New("chainlink returned incomplete round data")
	}
	if !updatedAt.IsInt64() {
		return SourcePrice{}, errors.New("chainlink returned invalid updatedAt")
	}
	price := new(big.Rat).SetInt(answer)
	price.Quo(price, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)))
	return SourcePrice{Source: "chainlink", USD: price, ObservedAt: time.Unix(updatedAt.Int64(), 0)}, nil
}

func (c *ChainlinkClient) call(ctx context.Context, method string) ([]any, error) {
	calldata, err := chainlinkAggregatorV3ABI.Pack(method)
	if err != nil {
		return nil, err
	}
	result, err := c.caller.CallContract(ctx, ethereum.CallMsg{To: &c.feed, Data: calldata}, nil)
	if err != nil {
		return nil, wrapPriceSourceRequestError("chainlink", "execute", err)
	}
	decoded, err := chainlinkAggregatorV3ABI.Unpack(method, result)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}
