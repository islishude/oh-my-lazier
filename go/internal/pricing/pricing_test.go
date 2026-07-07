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

func TestBuildPriceSnapshotConvertsDestinationGasPriceToSourceToken(t *testing.T) {
	snapshot, err := BuildPriceSnapshot(PriceInputs{
		SrcNativeUSD:      big.NewRat(2000, 1),
		DstNativeUSD:      big.NewRat(1000, 1),
		DstGasPriceWei:    big.NewInt(2_000_000_000),
		UpdatedAtUnix:     1_700_000_000,
		StaleAfterSeconds: 1800,
	})
	if err != nil {
		t.Fatalf("BuildPriceSnapshot() error = %v", err)
	}
	if snapshot.DstGasPriceInSrcToken.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("dst gas price = %s, want 1000000000", snapshot.DstGasPriceInSrcToken)
	}
}

func TestBuildPriceSnapshotRoundsUpFractionalWei(t *testing.T) {
	snapshot, err := BuildPriceSnapshot(PriceInputs{
		SrcNativeUSD:      big.NewRat(3, 1),
		DstNativeUSD:      big.NewRat(2, 1),
		DstGasPriceWei:    big.NewInt(10),
		UpdatedAtUnix:     1,
		StaleAfterSeconds: 2,
	})
	if err != nil {
		t.Fatalf("BuildPriceSnapshot() error = %v", err)
	}
	if snapshot.DstGasPriceInSrcToken.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("dst gas price = %s, want rounded-up 7", snapshot.DstGasPriceInSrcToken)
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

func TestBuildSetPriceSnapshotCalldata(t *testing.T) {
	snapshot := testPriceSnapshot()
	calldata, err := BuildSetPriceSnapshotCalldata([]PriceSnapshotUpdate{{DstEid: 40449, Snapshot: snapshot}})
	if err != nil {
		t.Fatalf("BuildSetPriceSnapshotCalldata() error = %v", err)
	}
	if len(calldata) == 0 {
		t.Fatal("calldata is empty")
	}
	method := priceSnapshotABI.Methods["setPriceSnapshot"]
	if string(calldata[:4]) != string(method.ID) {
		t.Fatalf("method id = 0x%x, want 0x%x", calldata[:4], method.ID)
	}
}

func TestBuildSetPriceSnapshotCalldataRejectsEmptyUpdates(t *testing.T) {
	if _, err := BuildSetPriceSnapshotCalldata(nil); err == nil {
		t.Fatal("BuildSetPriceSnapshotCalldata() error = nil, want empty updates error")
	}
}

func TestBuildSetPriceSnapshotTx(t *testing.T) {
	request, err := BuildSetPriceSnapshotTx(
		40161,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		"0x9999999999999999999999999999999999999999",
		[]PriceSnapshotUpdate{{DstEid: 40449, Snapshot: testPriceSnapshot()}},
	)
	if err != nil {
		t.Fatalf("BuildSetPriceSnapshotTx() error = %v", err)
	}
	if request.ChainEID != 40161 {
		t.Fatalf("chain eid = %d, want 40161", request.ChainEID)
	}
	if request.Purpose != TxPurposeSetPriceSnapshot {
		t.Fatalf("purpose = %q", request.Purpose)
	}
	if len(request.Calldata) == 0 {
		t.Fatal("calldata is empty")
	}
	if request.Value.Sign() != 0 {
		t.Fatalf("value = %s, want 0", request.Value)
	}
}

func testPriceSnapshot() PriceSnapshot {
	return PriceSnapshot{
		DstGasPriceInSrcToken: big.NewInt(2_000_000_000),
		UpdatedAt:             1_700_000_000,
		StaleAfter:            1800,
	}
}
