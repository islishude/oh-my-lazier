package lzabi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
)

func TestDecodeExecutorFeePaid(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fee := big.NewInt(123)
	data, err := sendLibBaseABI.Events["ExecutorFeePaid"].Inputs.Pack(executor, fee)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodeExecutorFeePaid(gethtypes.Log{
		Topics: []common.Hash{ExecutorFeePaidTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodeExecutorFeePaid() error = %v", err)
	}
	if event.Executor != executor {
		t.Fatalf("Executor = %s, want %s", event.Executor, executor)
	}
	if event.Fee.Cmp(fee) != 0 {
		t.Fatalf("Fee = %s, want %s", event.Fee, fee)
	}
}

func TestDecodeExecutorFeePaidRejectsWrongTopic(t *testing.T) {
	if _, err := DecodeExecutorFeePaid(gethtypes.Log{Topics: []common.Hash{common.HexToHash("0x01")}}); err == nil {
		t.Fatal("DecodeExecutorFeePaid() error = nil, want topic error")
	}
}

func TestDecodeDVNFeePaid(t *testing.T) {
	requiredDVNs := []common.Address{common.HexToAddress("0x3333333333333333333333333333333333333333")}
	optionalDVNs := []common.Address{common.HexToAddress("0x4444444444444444444444444444444444444444")}
	fees := []*big.Int{big.NewInt(123), big.NewInt(456)}
	data, err := sendLibBaseABI.Events["DVNFeePaid"].Inputs.Pack(requiredDVNs, optionalDVNs, fees)
	if err != nil {
		t.Fatalf("Pack() error = %v", err)
	}

	event, err := DecodeDVNFeePaid(gethtypes.Log{
		Topics: []common.Hash{DVNFeePaidTopic()},
		Data:   data,
	})
	if err != nil {
		t.Fatalf("DecodeDVNFeePaid() error = %v", err)
	}
	if event.RequiredDVNs[0] != requiredDVNs[0] {
		t.Fatalf("required dvn = %s, want %s", event.RequiredDVNs[0], requiredDVNs[0])
	}
	if event.OptionalDVNs[0] != optionalDVNs[0] {
		t.Fatalf("optional dvn = %s, want %s", event.OptionalDVNs[0], optionalDVNs[0])
	}
	if event.Fees[0].Cmp(fees[0]) != 0 || event.Fees[1].Cmp(fees[1]) != 0 {
		t.Fatalf("fees = %v, want %v", event.Fees, fees)
	}
}
