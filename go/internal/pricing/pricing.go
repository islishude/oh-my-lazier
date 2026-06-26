package pricing

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

const (
	// TxPurposeSetExecutorPriceConfig identifies OpenExecutor.setPriceConfig updates.
	TxPurposeSetExecutorPriceConfig = "pricing_set_executor_price_config"
	// TxPurposeSetDVNPriceConfig identifies OpenDVN.setPriceConfig updates.
	TxPurposeSetDVNPriceConfig = "pricing_set_dvn_price_config"
)

var (
	//go:embed abis/price_config.json
	priceConfigABIJSON string

	priceConfigABI = mustParseABI(priceConfigABIJSON)
)

// Bot updates worker contract price configuration.
type Bot struct {
	logger *slog.Logger
}

// New creates a price bot.
func New(logger *slog.Logger) *Bot {
	return &Bot{logger: logger}
}

// Run starts the price update loop until the context is canceled.
func (b *Bot) Run(ctx context.Context) error {
	b.logger.Info("price bot loop started")
	<-ctx.Done()
	return ctx.Err()
}

// SourcePrice is one USD/native price observed from a configured data source.
type SourcePrice struct {
	Source string
	USD    *big.Rat
}

// SelectPrice applies the phase-1 Binance-primary, Uniswap-sanity policy.
func SelectPrice(primary, sanity SourcePrice, maxDeviationBps uint64, allowFallback bool) (*big.Rat, error) {
	if primary.USD != nil && primary.USD.Sign() > 0 {
		if sanity.USD != nil && sanity.USD.Sign() > 0 {
			deviation := DeviationBps(primary.USD, sanity.USD)
			if deviation > maxDeviationBps {
				return nil, fmt.Errorf("price deviation %d bps exceeds limit %d bps", deviation, maxDeviationBps)
			}
		}
		return cloneRat(primary.USD), nil
	}
	if allowFallback && sanity.USD != nil && sanity.USD.Sign() > 0 {
		return cloneRat(sanity.USD), nil
	}
	return nil, errors.New("no healthy price source")
}

// DeviationBps returns abs(left-right)/left in basis points.
func DeviationBps(left, right *big.Rat) uint64 {
	if left == nil || right == nil || left.Sign() <= 0 || right.Sign() <= 0 {
		return ^uint64(0)
	}
	diff := new(big.Rat).Sub(left, right)
	if diff.Sign() < 0 {
		diff.Neg(diff)
	}
	diff.Mul(diff, big.NewRat(10_000, 1))
	diff.Quo(diff, left)
	return ceilRat(diff).Uint64()
}

// PriceInputs are the source data used to construct WorkerTypes.PriceConfig.
type PriceInputs struct {
	SrcNativeUSD      *big.Rat
	DstNativeUSD      *big.Rat
	DstGasPriceWei    *big.Int
	BaseFee           *big.Int
	BufferBps         uint16
	UpdatedAtUnix     uint64
	StaleAfterSeconds uint64
}

// PriceConfig mirrors WorkerTypes.PriceConfig for ABI encoding.
type PriceConfig struct {
	BaseFee               *big.Int `abi:"baseFee"`
	DstGasPriceInSrcToken *big.Int `abi:"dstGasPriceInSrcToken"`
	BufferBps             uint16   `abi:"bufferBps"`
	UpdatedAt             uint64   `abi:"updatedAt"`
	StaleAfter            uint64   `abi:"staleAfter"`
}

// BuildPriceConfig converts destination gas cost into source native-token units.
func BuildPriceConfig(inputs PriceInputs) (PriceConfig, error) {
	if inputs.SrcNativeUSD == nil || inputs.SrcNativeUSD.Sign() <= 0 {
		return PriceConfig{}, errors.New("source native USD price must be positive")
	}
	if inputs.DstNativeUSD == nil || inputs.DstNativeUSD.Sign() <= 0 {
		return PriceConfig{}, errors.New("destination native USD price must be positive")
	}
	if inputs.DstGasPriceWei == nil || inputs.DstGasPriceWei.Sign() <= 0 {
		return PriceConfig{}, errors.New("destination gas price must be positive")
	}
	if inputs.BaseFee == nil || inputs.BaseFee.Sign() < 0 {
		return PriceConfig{}, errors.New("base fee must be non-negative")
	}
	if inputs.UpdatedAtUnix == 0 {
		return PriceConfig{}, errors.New("updated_at is required")
	}
	if inputs.StaleAfterSeconds == 0 {
		return PriceConfig{}, errors.New("stale_after is required")
	}
	dstGasPriceInSrcToken := new(big.Rat).SetInt(inputs.DstGasPriceWei)
	dstGasPriceInSrcToken.Mul(dstGasPriceInSrcToken, inputs.DstNativeUSD)
	dstGasPriceInSrcToken.Quo(dstGasPriceInSrcToken, inputs.SrcNativeUSD)
	return PriceConfig{
		BaseFee:               new(big.Int).Set(inputs.BaseFee),
		DstGasPriceInSrcToken: ceilRat(dstGasPriceInSrcToken),
		BufferBps:             inputs.BufferBps,
		UpdatedAt:             inputs.UpdatedAtUnix,
		StaleAfter:            inputs.StaleAfterSeconds,
	}, nil
}

// TxFees carries optional EIP-1559 transaction fee settings for an outbox request.
type TxFees struct {
	GasLimit             *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
}

// BuildSetPriceConfigCalldata ABI-encodes OpenExecutor/OpenDVN setPriceConfig.
func BuildSetPriceConfigCalldata(dstEID uint32, config PriceConfig) ([]byte, error) {
	if dstEID == 0 {
		return nil, errors.New("destination eid is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return priceConfigABI.Pack("setPriceConfig", dstEID, config)
}

// BuildSetPriceConfigTx creates an outbox transaction for a worker setPriceConfig call.
func BuildSetPriceConfigTx(chainEID uint32, worker common.Address, dstEID uint32, purpose, signerID string, config PriceConfig, fees TxFees) (db.TxRequest, error) {
	if chainEID == 0 {
		return db.TxRequest{}, errors.New("chain eid is required")
	}
	if worker == (common.Address{}) {
		return db.TxRequest{}, errors.New("worker address is required")
	}
	if purpose != TxPurposeSetExecutorPriceConfig && purpose != TxPurposeSetDVNPriceConfig {
		return db.TxRequest{}, fmt.Errorf("unsupported price config purpose %q", purpose)
	}
	if signerID == "" {
		return db.TxRequest{}, errors.New("signer id is required")
	}
	calldata, err := BuildSetPriceConfigCalldata(dstEID, config)
	if err != nil {
		return db.TxRequest{}, err
	}
	return db.TxRequest{
		ChainEID:             chainEID,
		Purpose:              purpose,
		To:                   worker,
		Calldata:             calldata,
		Value:                new(big.Int),
		GasLimit:             cloneBigInt(fees.GasLimit),
		MaxFeePerGas:         cloneBigInt(fees.MaxFeePerGas),
		MaxPriorityFeePerGas: cloneBigInt(fees.MaxPriorityFeePerGas),
		SignerID:             signerID,
	}, nil
}

// Validate checks the on-chain price config invariants the worker can know before sending.
func (c PriceConfig) Validate() error {
	if c.BaseFee == nil || c.BaseFee.Sign() < 0 {
		return errors.New("price config base fee must be non-negative")
	}
	if c.DstGasPriceInSrcToken == nil || c.DstGasPriceInSrcToken.Sign() <= 0 {
		return errors.New("price config destination gas price must be positive")
	}
	if c.BufferBps > 10_000 {
		return errors.New("price config buffer bps exceeds 10000")
	}
	if c.UpdatedAt == 0 {
		return errors.New("price config updated_at is required")
	}
	if c.StaleAfter == 0 {
		return errors.New("price config stale_after is required")
	}
	return nil
}

func mustParseABI(definition string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(definition))
	if err != nil {
		panic(err)
	}
	return parsed
}

func ceilRat(value *big.Rat) *big.Int {
	if value == nil {
		return nil
	}
	num := value.Num()
	den := value.Denom()
	quotient, remainder := new(big.Int).QuoRem(num, den, new(big.Int))
	if remainder.Sign() != 0 && value.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func cloneRat(value *big.Rat) *big.Rat {
	if value == nil {
		return nil
	}
	return new(big.Rat).Set(value)
}

func cloneBigInt(value *big.Int) *big.Int {
	if value == nil {
		return nil
	}
	return new(big.Int).Set(value)
}
