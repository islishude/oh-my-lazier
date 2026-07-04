package pricing

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestSelectPriceRejectsDeviationAboveThreshold(t *testing.T) {
	_, err := SelectPrice(
		SourcePrice{Source: "binance", USD: big.NewRat(2000, 1)},
		SourcePrice{Source: "uniswap", USD: big.NewRat(2101, 1)},
		500,
		true,
	)
	if err == nil {
		t.Fatal("SelectPrice() error = nil, want deviation error")
	}
}

func TestSelectPriceFallsBackWhenPrimaryUnavailable(t *testing.T) {
	price, err := SelectPrice(
		SourcePrice{Source: "binance"},
		SourcePrice{Source: "uniswap", USD: big.NewRat(2000, 1)},
		500,
		true,
	)
	if err != nil {
		t.Fatalf("SelectPrice() error = %v", err)
	}
	if price.Cmp(big.NewRat(2000, 1)) != 0 {
		t.Fatalf("price = %s, want 2000", price)
	}
}

func TestSelectPriceRejectsFallbackWhenDisabled(t *testing.T) {
	_, err := SelectPrice(
		SourcePrice{Source: "binance"},
		SourcePrice{Source: "uniswap", USD: big.NewRat(2000, 1)},
		500,
		false,
	)
	if err == nil {
		t.Fatal("SelectPrice() error = nil, want no healthy price source")
	}
}

func TestSelectPriceWithSanityRejectsAnyDeviatingSource(t *testing.T) {
	_, err := SelectPriceWithSanity(
		SourcePrice{Source: "coinmarketcap", USD: big.NewRat(2000, 1)},
		[]SourcePrice{
			{Source: "coingecko", USD: big.NewRat(2000, 1)},
			{Source: "uniswap", USD: big.NewRat(2200, 1)},
		},
		500,
		true,
	)
	if err == nil {
		t.Fatal("SelectPriceWithSanity() error = nil, want deviation error")
	}
}

func TestBuildPriceConfigConvertsDestinationGasPriceToSourceToken(t *testing.T) {
	config, err := BuildPriceConfig(PriceInputs{
		SrcNativeUSD:      big.NewRat(2000, 1),
		DstNativeUSD:      big.NewRat(1000, 1),
		DstGasPriceWei:    big.NewInt(2_000_000_000),
		Fee:               FeeModel{FixedFee: big.NewInt(1000), DstGasOverhead: 50_000, MarginBps: 100},
		UpdatedAtUnix:     1_700_000_000,
		StaleAfterSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("BuildPriceConfig() error = %v", err)
	}
	if config.DstGasPriceInSrcToken.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("dst gas price = %s, want 1000000000", config.DstGasPriceInSrcToken)
	}
	if config.BaseFee.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("base fee = %s, want 1000", config.BaseFee)
	}
	if config.DstGasOverhead != 50_000 {
		t.Fatalf("dst gas overhead = %d, want 50000", config.DstGasOverhead)
	}
	if config.MarginBps != 100 {
		t.Fatalf("margin bps = %d, want 100", config.MarginBps)
	}
}

func TestBuildPriceConfigRoundsUpFractionalWei(t *testing.T) {
	config, err := BuildPriceConfig(PriceInputs{
		SrcNativeUSD:      big.NewRat(3, 1),
		DstNativeUSD:      big.NewRat(2, 1),
		DstGasPriceWei:    big.NewInt(10),
		Fee:               FeeModel{FixedFee: big.NewInt(0)},
		UpdatedAtUnix:     1,
		StaleAfterSeconds: 2,
	})
	if err != nil {
		t.Fatalf("BuildPriceConfig() error = %v", err)
	}
	if config.DstGasPriceInSrcToken.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("dst gas price = %s, want rounded-up 7", config.DstGasPriceInSrcToken)
	}
}

func TestGasIncreaseBpsOnlyCountsUpwardMoves(t *testing.T) {
	if got := GasIncreaseBps(big.NewInt(100), big.NewInt(110)); got != 1000 {
		t.Fatalf("GasIncreaseBps() = %d, want 1000", got)
	}
	if got := GasIncreaseBps(big.NewInt(100), big.NewInt(90)); got != 0 {
		t.Fatalf("GasIncreaseBps() = %d, want 0 for gas decrease", got)
	}
}

func TestBuildSetPriceConfigCalldata(t *testing.T) {
	config := testPriceConfig()
	calldata, err := BuildSetPriceConfigCalldata(40449, config)
	if err != nil {
		t.Fatalf("BuildSetPriceConfigCalldata() error = %v", err)
	}
	if len(calldata) != 4+32*7 {
		t.Fatalf("calldata len = %d, want %d", len(calldata), 4+32*7)
	}
	method := priceConfigABI.Methods["setPriceConfig"]
	if string(calldata[:4]) != string(method.ID) {
		t.Fatalf("method id = 0x%x, want 0x%x", calldata[:4], method.ID)
	}
}

func TestBuildSetPriceConfigTx(t *testing.T) {
	request, err := BuildSetPriceConfigTx(
		40161,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		40449,
		TxPurposeSetExecutorPriceConfig,
		"0x9999999999999999999999999999999999999999",
		testPriceConfig(),
	)
	if err != nil {
		t.Fatalf("BuildSetPriceConfigTx() error = %v", err)
	}
	if request.ChainEID != 40161 {
		t.Fatalf("chain eid = %d, want 40161", request.ChainEID)
	}
	if request.Purpose != TxPurposeSetExecutorPriceConfig {
		t.Fatalf("purpose = %q", request.Purpose)
	}
	if len(request.Calldata) == 0 {
		t.Fatal("calldata is empty")
	}
	if request.Value.Sign() != 0 {
		t.Fatalf("value = %s, want 0", request.Value)
	}
}

func testPriceConfig() PriceConfig {
	return PriceConfig{
		BaseFee:               big.NewInt(1000),
		DstGasPriceInSrcToken: big.NewInt(2_000_000_000),
		DstGasOverhead:        50_000,
		MarginBps:             100,
		UpdatedAt:             1_700_000_000,
		StaleAfter:            1800,
	}
}
