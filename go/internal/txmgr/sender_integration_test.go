package txmgr

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/islishude/oh-my-lazier/go/internal/signer/keystore"
)

func TestProcessNextSignsAndBroadcastsDynamicFeeTx(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 10, estimatedGas: 123_456, header: dynamicHeader(), suggestedGasTipCap: big.NewInt(1_000_000_000)}
	manager := New(store, discardLogger())

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "commit-verification",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x01, 0x02, 0x03},
		Value:    big.NewInt(123),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(client.sent))
	}
	sent := client.sent[0]
	if sent.Type() != types.DynamicFeeTxType {
		t.Fatalf("sent tx type = %d, want dynamic fee", sent.Type())
	}
	if sent.Nonce() != 10 {
		t.Fatalf("sent nonce = %d, want 10", sent.Nonce())
	}
	if sent.Gas() != 123_456 {
		t.Fatalf("sent gas = %d, want estimated gas", sent.Gas())
	}
	if sent.GasFeeCap().Cmp(big.NewInt(2_000_000_000)) != 0 {
		t.Fatalf("sent gas fee cap = %s", sent.GasFeeCap())
	}
	if sent.GasTipCap().Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("sent gas tip cap = %s", sent.GasTipCap())
	}
	if client.suggestGasPriceCalls != 0 {
		t.Fatalf("SuggestGasPrice() calls = %d, want 0", client.suggestGasPriceCalls)
	}
	if client.suggestGasTipCapCalls != 1 {
		t.Fatalf("SuggestGasTipCap() calls = %d, want 1", client.suggestGasTipCapCalls)
	}
	assertEstimateGasCall(t, client, signer.Address(), common.HexToAddress("0x2222222222222222222222222222222222222222"), big.NewInt(123), []byte{0x01, 0x02, 0x03})
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(11155111)), sent)
	if err != nil {
		t.Fatalf("Sender() error = %v", err)
	}
	if from != signer.Address() {
		t.Fatalf("sender = %s, want %s", from, signer.Address())
	}

	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Status != db.TxStatusBroadcast {
		t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusBroadcast)
	}
	if outboxTx.MaxFeePerGas.Cmp(big.NewInt(2_000_000_000)) != 0 {
		t.Fatalf("recorded max fee = %s", outboxTx.MaxFeePerGas)
	}
	if outboxTx.MaxPriorityFeePerGas.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("recorded priority fee = %s", outboxTx.MaxPriorityFeePerGas)
	}
	if outboxTx.GasLimit != 123_456 {
		t.Fatalf("recorded gas limit = %d, want estimated gas", outboxTx.GasLimit)
	}
}

func TestProcessNextSignsLegacyTxWithSuggestedGasPrice(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 12, estimatedGas: 98_765, header: legacyHeader(), suggestedGasPrice: big.NewInt(7_000_000_000)}
	manager := New(store, discardLogger())

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "commit-verification",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x01, 0x02, 0x03},
		Value:    big.NewInt(123),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if id == 0 {
		t.Fatal("ProcessNext() id = 0")
	}
	if client.suggestGasPriceCalls != 1 {
		t.Fatalf("SuggestGasPrice() calls = %d, want 1", client.suggestGasPriceCalls)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(client.sent))
	}
	sent := client.sent[0]
	if sent.Type() != types.LegacyTxType {
		t.Fatalf("sent tx type = %d, want legacy", sent.Type())
	}
	if sent.GasPrice().Cmp(big.NewInt(7_000_000_000)) != 0 {
		t.Fatalf("sent gas price = %s, want 7000000000", sent.GasPrice())
	}
	if sent.Gas() != 98_765 {
		t.Fatalf("sent gas = %d, want estimated gas", sent.Gas())
	}
	assertEstimateGasCall(t, client, signer.Address(), common.HexToAddress("0x2222222222222222222222222222222222222222"), big.NewInt(123), []byte{0x01, 0x02, 0x03})
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.MaxFeePerGas.Cmp(big.NewInt(7_000_000_000)) != 0 {
		t.Fatalf("recorded gas price = %s, want 7000000000", outboxTx.MaxFeePerGas)
	}
	if outboxTx.MaxPriorityFeePerGas != nil {
		t.Fatalf("recorded priority fee = %s, want nil", outboxTx.MaxPriorityFeePerGas)
	}
	if outboxTx.GasLimit != 98_765 {
		t.Fatalf("recorded gas limit = %d, want estimated gas", outboxTx.GasLimit)
	}
	from, err := types.Sender(types.LatestSignerForChainID(big.NewInt(11155111)), sent)
	if err != nil {
		t.Fatalf("Sender() error = %v", err)
	}
	if from != signer.Address() {
		t.Fatalf("sender = %s, want %s", from, signer.Address())
	}
}

func TestProcessNextDefersFeeOverCapBeforeNonceAssignment(t *testing.T) {
	tests := []struct {
		name   string
		client *fakeChainClient
		policy FeePolicy
	}{
		{
			name: "dynamic",
			client: &fakeChainClient{
				pendingNonce:       13,
				header:             dynamicHeader(),
				suggestedGasTipCap: big.NewInt(1_000_000_000),
			},
			policy: FeePolicy{
				ConfiguredMaxFeePerGas:         big.NewInt(1_500_000_000),
				ConfiguredMaxPriorityFeePerGas: big.NewInt(1_000_000_000),
			},
		},
		{
			name: "legacy",
			client: &fakeChainClient{
				pendingNonce:      13,
				header:            legacyHeader(),
				suggestedGasPrice: big.NewInt(2_000_000_000),
			},
			policy: FeePolicy{
				ConfiguredMaxFeePerGas: big.NewInt(1_500_000_000),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t)
			signer := newTestKeystoreSigner(t)
			manager := New(store, discardLogger())

			queuedID, err := store.EnqueueTx(t.Context(), db.TxRequest{
				ChainEID: 40161,
				Purpose:  "commit-verification",
				To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
				Calldata: []byte{0x01, 0x02, 0x03},
				Value:    big.NewInt(123),
				SignerID: signer.Address().Hex(),
			})
			if err != nil {
				t.Fatalf("EnqueueTx() error = %v", err)
			}

			_, err = manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, tt.client, tt.policy))
			if !errors.Is(err, ErrTxDeferred) {
				t.Fatalf("ProcessNext() error = %v, want ErrTxDeferred", err)
			}
			outboxTx, err := store.GetOutboxTx(t.Context(), queuedID)
			if err != nil {
				t.Fatalf("GetOutboxTx() error = %v", err)
			}
			if outboxTx.Status != db.TxStatusQueued {
				t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusQueued)
			}
			if outboxTx.Nonce != 0 {
				t.Fatalf("outbox nonce = %d, want unassigned zero value", outboxTx.Nonce)
			}
			if outboxTx.Attempts != 0 {
				t.Fatalf("outbox attempts = %d, want 0", outboxTx.Attempts)
			}
			if outboxTx.MaxFeePerGas != nil || outboxTx.MaxPriorityFeePerGas != nil {
				t.Fatalf("recorded fees = %v/%v, want nil", outboxTx.MaxFeePerGas, outboxTx.MaxPriorityFeePerGas)
			}
			if tt.client.pendingNonceCalls != 0 {
				t.Fatalf("PendingNonceAt() calls = %d, want 0", tt.client.pendingNonceCalls)
			}
			if len(tt.client.estimateGasCalls) != 0 {
				t.Fatalf("EstimateGas() calls = %d, want 0", len(tt.client.estimateGasCalls))
			}
			if len(tt.client.sent) != 0 {
				t.Fatalf("sent tx count = %d, want 0", len(tt.client.sent))
			}
		})
	}
}

func TestProcessNextDefersEstimateGasNonRevertErrorBeforeNonceAssignment(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce:       13,
		estimateGasErr:     errors.New("rpc unavailable"),
		header:             dynamicHeader(),
		suggestedGasTipCap: big.NewInt(1_000_000_000),
	}
	manager := New(store, discardLogger())

	queuedID, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "commit-verification",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x01, 0x02, 0x03},
		Value:    big.NewInt(123),
		SignerID: signer.Address().Hex(),
	})
	if err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	_, err = manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if !errors.Is(err, ErrTxDeferred) {
		t.Fatalf("ProcessNext() error = %v, want ErrTxDeferred", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), queuedID)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Status != db.TxStatusQueued {
		t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusQueued)
	}
	if outboxTx.Nonce != 0 {
		t.Fatalf("outbox nonce = %d, want unassigned zero value", outboxTx.Nonce)
	}
	if outboxTx.Attempts != 0 {
		t.Fatalf("outbox attempts = %d, want 0", outboxTx.Attempts)
	}
	if client.pendingNonceCalls != 0 {
		t.Fatalf("PendingNonceAt() calls = %d, want 0", client.pendingNonceCalls)
	}
	if len(client.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(client.sent))
	}
	assertEstimateGasCall(t, client, signer.Address(), common.HexToAddress("0x2222222222222222222222222222222222222222"), big.NewInt(123), []byte{0x01, 0x02, 0x03})
}

func TestProcessNextMarksEstimateGasRevertFailedBeforeNonceAssignment(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce:       13,
		estimateGasErr:     fakeRPCDataError{message: "execution reverted: denied", data: "0x08c379a0"},
		header:             dynamicHeader(),
		suggestedGasTipCap: big.NewInt(1_000_000_000),
	}
	manager := New(store, discardLogger())

	queuedID, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "commit-verification",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x01, 0x02, 0x03},
		Value:    big.NewInt(123),
		SignerID: signer.Address().Hex(),
	})
	if err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	if _, err = manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy())); err == nil {
		t.Fatal("ProcessNext() error = nil, want revert error")
	}
	if errors.Is(err, ErrTxDeferred) {
		t.Fatalf("ProcessNext() error = %v, want non-deferred revert error", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), queuedID)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Status != db.TxStatusFailed {
		t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusFailed)
	}
	if outboxTx.Nonce != 0 {
		t.Fatalf("outbox nonce = %d, want unassigned zero value", outboxTx.Nonce)
	}
	if outboxTx.Attempts != 0 {
		t.Fatalf("outbox attempts = %d, want 0", outboxTx.Attempts)
	}
	if client.pendingNonceCalls != 0 {
		t.Fatalf("PendingNonceAt() calls = %d, want 0", client.pendingNonceCalls)
	}
	if len(client.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(client.sent))
	}
	assertEstimateGasCall(t, client, signer.Address(), common.HexToAddress("0x2222222222222222222222222222222222222222"), big.NewInt(123), []byte{0x01, 0x02, 0x03})
}

func TestProcessNextLegacyGasPriceFailuresLeaveOutboxQueued(t *testing.T) {
	tests := []struct {
		name              string
		suggestedGasPrice *big.Int
		suggestErr        error
	}{
		{name: "rpc error", suggestErr: errors.New("gas price unavailable")},
		{name: "nil gas price"},
		{name: "zero gas price", suggestedGasPrice: new(big.Int)},
		{name: "negative gas price", suggestedGasPrice: big.NewInt(-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := openTestStore(t)
			signer := newTestKeystoreSigner(t)
			client := &fakeChainClient{
				pendingNonce:       13,
				header:             legacyHeader(),
				suggestedGasPrice:  tt.suggestedGasPrice,
				suggestGasPriceErr: tt.suggestErr,
			}
			manager := New(store, discardLogger())

			queuedID, err := store.EnqueueTx(t.Context(), db.TxRequest{
				ChainEID: 40161,
				Purpose:  "commit-verification",
				To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
				Calldata: []byte{0x01, 0x02, 0x03},
				Value:    big.NewInt(123),
				SignerID: signer.Address().Hex(),
			})
			if err != nil {
				t.Fatalf("EnqueueTx() error = %v", err)
			}

			if _, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy())); err == nil {
				t.Fatal("ProcessNext() error = nil, want gas price error")
			}
			outboxTx, err := store.GetOutboxTx(t.Context(), queuedID)
			if err != nil {
				t.Fatalf("GetOutboxTx() error = %v", err)
			}
			if outboxTx.Status != db.TxStatusQueued {
				t.Fatalf("outbox status = %q, want %q", outboxTx.Status, db.TxStatusQueued)
			}
			if outboxTx.Nonce != 0 {
				t.Fatalf("outbox nonce = %d, want unassigned zero value", outboxTx.Nonce)
			}
			if len(client.sent) != 0 {
				t.Fatalf("sent tx count = %d, want 0", len(client.sent))
			}
		})
	}
}

func TestPrepareReplacementTxPreservesNonceAndBumpsFees(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 21, estimatedGas: 111_111, header: dynamicHeader(), suggestedGasTipCap: big.NewInt(1_000_000_000)}
	manager := New(store, discardLogger())

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "lz-receive",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x04, 0x05},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if err := store.PrepareReplacementTx(t.Context(), id); err != nil {
		t.Fatalf("PrepareReplacementTx() error = %v", err)
	}
	replacement, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if replacement.Nonce != 21 {
		t.Fatalf("replacement nonce = %d, want 21", replacement.Nonce)
	}
	if replacement.Status != db.TxStatusQueued {
		t.Fatalf("replacement status = %q, want %q", replacement.Status, db.TxStatusQueued)
	}
	if replacement.MaxFeePerGas.Cmp(big.NewInt(2_000_000_000)) != 0 {
		t.Fatalf("replacement max fee = %s", replacement.MaxFeePerGas)
	}
	if replacement.MaxPriorityFeePerGas.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("replacement priority fee = %s", replacement.MaxPriorityFeePerGas)
	}
	if replacement.Attempts != 1 {
		t.Fatalf("replacement attempts = %d, want 1", replacement.Attempts)
	}
	client.estimatedGas = 222_222
	replacementID, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() replacement error = %v", err)
	}
	if replacementID != id {
		t.Fatalf("replacement id = %d, want %d", replacementID, id)
	}
	if len(client.sent) != 2 {
		t.Fatalf("sent tx count = %d, want 2", len(client.sent))
	}
	replacementTx := client.sent[1]
	if replacementTx.Nonce() != 21 {
		t.Fatalf("replacement tx nonce = %d, want 21", replacementTx.Nonce())
	}
	if replacementTx.Gas() != 222_222 {
		t.Fatalf("replacement tx gas = %d, want re-estimated gas", replacementTx.Gas())
	}
	if replacementTx.GasFeeCap().Cmp(big.NewInt(2_200_000_000)) != 0 {
		t.Fatalf("replacement tx max fee = %s", replacementTx.GasFeeCap())
	}
	if replacementTx.GasTipCap().Cmp(big.NewInt(1_100_000_000)) != 0 {
		t.Fatalf("replacement tx priority fee = %s", replacementTx.GasTipCap())
	}
	if len(client.estimateGasCalls) != 2 {
		t.Fatalf("EstimateGas() calls = %d, want 2", len(client.estimateGasCalls))
	}
}

func TestPrepareReplacementTxPreservesNonceAndRefreshesGasPrice(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 22, estimatedGas: 111_111, header: legacyHeader(), suggestedGasPrice: big.NewInt(4_000_000_000)}
	manager := New(store, discardLogger())

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "lz-receive",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x04, 0x05},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if err := store.PrepareReplacementTx(t.Context(), id); err != nil {
		t.Fatalf("PrepareReplacementTx() error = %v", err)
	}
	replacement, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if replacement.Nonce != 22 {
		t.Fatalf("replacement nonce = %d, want 22", replacement.Nonce)
	}
	if replacement.Status != db.TxStatusQueued {
		t.Fatalf("replacement status = %q, want %q", replacement.Status, db.TxStatusQueued)
	}
	if replacement.Attempts != 1 {
		t.Fatalf("replacement attempts = %d, want 1", replacement.Attempts)
	}

	client.suggestedGasPrice = big.NewInt(5_000_000_000)
	client.estimatedGas = 222_222
	replacementID, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() replacement error = %v", err)
	}
	if replacementID != id {
		t.Fatalf("replacement id = %d, want %d", replacementID, id)
	}
	if len(client.sent) != 2 {
		t.Fatalf("sent tx count = %d, want 2", len(client.sent))
	}
	replacementTx := client.sent[1]
	if replacementTx.Type() != types.LegacyTxType {
		t.Fatalf("replacement tx type = %d, want legacy", replacementTx.Type())
	}
	if replacementTx.Nonce() != 22 {
		t.Fatalf("replacement tx nonce = %d, want 22", replacementTx.Nonce())
	}
	if replacementTx.Gas() != 222_222 {
		t.Fatalf("replacement tx gas = %d, want re-estimated gas", replacementTx.Gas())
	}
	if replacementTx.GasPrice().Cmp(big.NewInt(5_000_000_000)) != 0 {
		t.Fatalf("replacement tx gas price = %s", replacementTx.GasPrice())
	}
	if len(client.estimateGasCalls) != 2 {
		t.Fatalf("EstimateGas() calls = %d, want 2", len(client.estimateGasCalls))
	}
}

func TestProcessReceiptsMarksBroadcastTxConfirmed(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 33,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID: 40161,
		Purpose:  "lz-receive",
		To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata: []byte{0x04, 0x05},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(40161, big.NewInt(11155111), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Nonce >= client.pendingNonce {
		client.pendingNonce = outboxTx.Nonce + 1
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusSuccessful}

	processedID, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: 40161,
		ChainID:  big.NewInt(11155111),
		Signer:   signer,
		Client:   client,
	}, 1)
	if err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
	if processedID != id {
		t.Fatalf("processed id = %d, want %d", processedID, id)
	}
	confirmed, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() after receipt error = %v", err)
	}
	if confirmed.Status != db.TxStatusConfirmed {
		t.Fatalf("status = %q, want %q", confirmed.Status, db.TxStatusConfirmed)
	}
}

func TestProcessReceiptsMarksExecutorLzReceiveDelivered(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 44,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())
	packet := testExecutorPacket(t)
	packet.Status = string(packets.ExecutorExecutable)
	if err := store.UpsertPacket(t.Context(), packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(t.Context(), db.ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorExecutable),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}
	if _, err := store.EnqueueExecutorTx(
		t.Context(),
		packet.GUID,
		string(packets.ExecutorExecutable),
		string(packets.ExecutorLzReceiveTxEnqueued),
		db.TxRequest{
			ChainEID: packet.DstEID,
			Purpose:  executorLzReceivePurpose,
			GUID:     packet.GUID.Bytes(),
			To:       packet.Receiver,
			Calldata: []byte{0x04, 0x05},
			Value:    big.NewInt(0),
			SignerID: signer.Address().Hex(),
		},
	); err != nil {
		t.Fatalf("EnqueueExecutorTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(packet.DstEID, big.NewInt(84532), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Nonce >= client.pendingNonce {
		client.pendingNonce = outboxTx.Nonce + 1
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusSuccessful}

	if _, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: packet.DstEID,
		ChainID:  big.NewInt(84532),
		Signer:   signer,
		Client:   client,
	}, 1); err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
	delivered, err := store.GetPacket(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetPacket() error = %v", err)
	}
	if delivered.Status != string(packets.ExecutorDelivered) {
		t.Fatalf("packet status = %q, want %q", delivered.Status, packets.ExecutorDelivered)
	}
}

func TestProcessReceiptsMarksExecutorLzReceiveFailed(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 55,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())
	packet := testExecutorPacket(t)
	packet.Status = string(packets.ExecutorExecutable)
	if err := store.UpsertPacket(t.Context(), packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(t.Context(), db.ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorExecutable),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}
	if _, err := store.EnqueueExecutorTx(
		t.Context(),
		packet.GUID,
		string(packets.ExecutorExecutable),
		string(packets.ExecutorLzReceiveTxEnqueued),
		db.TxRequest{
			ChainEID: packet.DstEID,
			Purpose:  executorLzReceivePurpose,
			GUID:     packet.GUID.Bytes(),
			To:       packet.Receiver,
			Calldata: []byte{0x04, 0x05},
			Value:    big.NewInt(0),
			SignerID: signer.Address().Hex(),
		},
	); err != nil {
		t.Fatalf("EnqueueExecutorTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(packet.DstEID, big.NewInt(84532), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusFailed}

	if _, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: packet.DstEID,
		ChainID:  big.NewInt(84532),
		Signer:   signer,
		Client:   client,
	}, 1); err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
	failedPacket, err := store.GetPacket(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetPacket() error = %v", err)
	}
	if failedPacket.Status != string(packets.ExecutorLzReceiveFailed) {
		t.Fatalf("packet status = %q, want %q", failedPacket.Status, packets.ExecutorLzReceiveFailed)
	}
	failedTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() after receipt error = %v", err)
	}
	if failedTx.Status != db.TxStatusFailed {
		t.Fatalf("tx status = %q, want %q", failedTx.Status, db.TxStatusFailed)
	}
}

func TestProcessReceiptsMarksDVNVerifyTxVerified(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 66,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())
	packet := testExecutorPacket(t)
	if err := store.UpsertPacket(t.Context(), packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertDVNJob(t.Context(), db.DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNQuorumChecking),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}
	if _, err := store.EnqueueDVNVerifyTx(
		t.Context(),
		packet.GUID,
		string(packets.DVNQuorumChecking),
		string(packets.DVNVerifyTxEnqueued),
		db.TxRequest{
			ChainEID: packet.DstEID,
			Purpose:  dvnVerifyPurpose,
			GUID:     packet.GUID.Bytes(),
			To:       packet.Receiver,
			Calldata: []byte{0x06, 0x07},
			Value:    big.NewInt(0),
			SignerID: signer.Address().Hex(),
		},
		[]byte(`{"status":"ready"}`),
	); err != nil {
		t.Fatalf("EnqueueDVNVerifyTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(packet.DstEID, big.NewInt(84532), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusSuccessful}

	if _, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: packet.DstEID,
		ChainID:  big.NewInt(84532),
		Signer:   signer,
		Client:   client,
	}, 1); err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
	job, err := store.GetDVNJob(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetDVNJob() error = %v", err)
	}
	if job.Status != string(packets.DVNVerified) {
		t.Fatalf("dvn job status = %q, want %q", job.Status, packets.DVNVerified)
	}
}

func TestProcessReceiptsFailedDVNVerifyOnlyFailsOutbox(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 67,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())
	packet := testExecutorPacket(t)
	if err := store.UpsertPacket(t.Context(), packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertDVNJob(t.Context(), db.DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNReadyToVerify),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}
	if _, err := store.EnqueueDVNVerifyTx(t.Context(), packet.GUID, string(packets.DVNReadyToVerify), string(packets.DVNVerifyTxEnqueued), db.TxRequest{
		ChainEID: packet.DstEID,
		Purpose:  dvnVerifyPurpose,
		GUID:     packet.GUID.Bytes(),
		To:       packet.Receiver,
		Calldata: []byte{0x06, 0x07},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}, []byte(`{"status":"ready"}`)); err != nil {
		t.Fatalf("EnqueueDVNVerifyTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), testTarget(packet.DstEID, big.NewInt(84532), signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusFailed}

	if _, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: packet.DstEID,
		ChainID:  big.NewInt(84532),
		Signer:   signer,
		Client:   client,
	}, 1); err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
	failedTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() after receipt error = %v", err)
	}
	if failedTx.Status != db.TxStatusFailed {
		t.Fatalf("tx status = %q, want %q", failedTx.Status, db.TxStatusFailed)
	}
	job, err := store.GetDVNJob(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetDVNJob() error = %v", err)
	}
	if job.Status != string(packets.DVNVerifyTxEnqueued) {
		t.Fatalf("dvn status = %q, want %q", job.Status, packets.DVNVerifyTxEnqueued)
	}
}

func TestSyntheticActiveFlowVerifiesCommitsAndDelivers(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 77,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, discardLogger())
	packet := testExecutorPacket(t)
	packet.Status = string(packets.ExecutorAssigned)
	if err := store.UpsertPacket(t.Context(), packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(t.Context(), db.ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorAssigned),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}
	if err := store.UpsertDVNJob(t.Context(), db.DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNReadyToVerify),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}

	if _, err := store.EnqueueDVNVerifyTx(t.Context(), packet.GUID, string(packets.DVNReadyToVerify), string(packets.DVNVerifyTxEnqueued), db.TxRequest{
		ChainEID: packet.DstEID,
		Purpose:  dvnVerifyPurpose,
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata: []byte{0x06, 0x07},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}, []byte(`{"status":"ready"}`)); err != nil {
		t.Fatalf("EnqueueDVNVerifyTx() error = %v", err)
	}
	processQueuedSuccess(t, manager, store, client, signer, packet.DstEID, big.NewInt(84532))
	job, err := store.GetDVNJob(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetDVNJob() error = %v", err)
	}
	if job.Status != string(packets.DVNVerified) {
		t.Fatalf("dvn status = %q, want %q", job.Status, packets.DVNVerified)
	}

	if err := store.MarkExecutorWaitingDVNVerification(t.Context(), packet.GUID, string(packets.ExecutorAssigned)); err != nil {
		t.Fatalf("MarkExecutorWaitingDVNVerification() error = %v", err)
	}
	if err := store.MarkExecutorVerifiable(t.Context(), packet.GUID, string(packets.ExecutorWaitingDVNVerification)); err != nil {
		t.Fatalf("MarkExecutorVerifiable() error = %v", err)
	}
	if _, err := store.EnqueueExecutorTx(t.Context(), packet.GUID, string(packets.ExecutorVerifiable), string(packets.ExecutorCommitTxEnqueued), db.TxRequest{
		ChainEID: packet.DstEID,
		Purpose:  executorCommitVerificationPurpose,
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata: []byte{0x08, 0x09},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueExecutorTx(commit) error = %v", err)
	}
	processQueuedSuccess(t, manager, store, client, signer, packet.DstEID, big.NewInt(84532))
	committed, err := store.GetPacket(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetPacket() after commit error = %v", err)
	}
	if committed.Status != string(packets.ExecutorCommitted) {
		t.Fatalf("packet status = %q, want %q", committed.Status, packets.ExecutorCommitted)
	}

	if err := store.MarkExecutorExecutable(t.Context(), packet.GUID); err != nil {
		t.Fatalf("MarkExecutorExecutable() error = %v", err)
	}
	if _, err := store.EnqueueExecutorTx(t.Context(), packet.GUID, string(packets.ExecutorExecutable), string(packets.ExecutorLzReceiveTxEnqueued), db.TxRequest{
		ChainEID: packet.DstEID,
		Purpose:  executorLzReceivePurpose,
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0x4444444444444444444444444444444444444444"),
		Calldata: []byte{0x0a, 0x0b},
		Value:    big.NewInt(0),
		SignerID: signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueExecutorTx(lzReceive) error = %v", err)
	}
	processQueuedSuccess(t, manager, store, client, signer, packet.DstEID, big.NewInt(84532))
	delivered, err := store.GetPacket(t.Context(), packet.GUID)
	if err != nil {
		t.Fatalf("GetPacket() after delivery error = %v", err)
	}
	if delivered.Status != string(packets.ExecutorDelivered) {
		t.Fatalf("packet status = %q, want %q", delivered.Status, packets.ExecutorDelivered)
	}
}

func processQueuedSuccess(t *testing.T, manager *Manager, store *db.Store, client *fakeChainClient, signer *keystore.Signer, chainEID uint32, chainID *big.Int) {
	t.Helper()
	id, err := manager.ProcessNext(t.Context(), testTarget(chainEID, chainID, signer, client, defaultFeePolicy()))
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	outboxTx, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if outboxTx.Nonce >= client.pendingNonce {
		client.pendingNonce = outboxTx.Nonce + 1
	}
	client.receipts[outboxTx.TxHash] = &types.Receipt{TxHash: outboxTx.TxHash, Status: types.ReceiptStatusSuccessful}
	if _, err := manager.ProcessReceipts(t.Context(), Target{
		ChainEID: chainEID,
		ChainID:  chainID,
		Signer:   signer,
		Client:   client,
	}, 1); err != nil {
		t.Fatalf("ProcessReceipts() error = %v", err)
	}
}

func assertEstimateGasCall(t *testing.T, client *fakeChainClient, from, to common.Address, value *big.Int, data []byte) {
	t.Helper()
	if len(client.estimateGasCalls) != 1 {
		t.Fatalf("EstimateGas() calls = %d, want 1", len(client.estimateGasCalls))
	}
	call := client.estimateGasCalls[0]
	if call.From != from {
		t.Fatalf("EstimateGas() from = %s, want %s", call.From, from)
	}
	if call.To == nil || *call.To != to {
		t.Fatalf("EstimateGas() to = %v, want %s", call.To, to)
	}
	if call.Value.Cmp(value) != 0 {
		t.Fatalf("EstimateGas() value = %s, want %s", call.Value, value)
	}
	if !bytes.Equal(call.Data, data) {
		t.Fatalf("EstimateGas() data = %x, want %x", call.Data, data)
	}
	if call.Gas != 0 {
		t.Fatalf("EstimateGas() gas = %d, want 0", call.Gas)
	}
	if call.GasPrice != nil || call.GasFeeCap != nil || call.GasTipCap != nil {
		t.Fatalf("EstimateGas() fee fields = %v/%v/%v, want nil", call.GasPrice, call.GasFeeCap, call.GasTipCap)
	}
}

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	store, err := db.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	registry, err := chain.NewRegistry(testChains(), testPathways())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := store.SyncConfig(ctx, registry); err != nil {
		t.Fatalf("SyncConfig() error = %v", err)
	}
	return store
}

func newTestKeystoreSigner(t *testing.T) *keystore.Signer {
	t.Helper()
	dir := t.TempDir()
	const password = "test-password"
	account, err := gethkeystore.StoreKey(dir, password, gethkeystore.StandardScryptN, gethkeystore.StandardScryptP)
	if err != nil {
		t.Fatalf("StoreKey() error = %v", err)
	}
	signer, err := keystore.LoadWithPasswordSource(filepath.Clean(account.URL.Path), keystore.PasswordSource{Value: password})
	if err != nil {
		t.Fatalf("LoadWithPasswordSource() error = %v", err)
	}
	return signer
}

func testExecutorPacket(t *testing.T) db.PacketRecord {
	t.Helper()
	message := []byte{0x03, 0x04}
	guid := crypto.Keccak256Hash([]byte(t.Name()))
	nonce := new(big.Int).SetBytes(guid[:8])
	return db.PacketRecord{
		GUID:           guid,
		SrcEID:         40161,
		DstEID:         40245,
		Nonce:          nonce,
		Sender:         common.HexToAddress("0x7777777777777777777777777777777777777777"),
		Receiver:       common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:        common.HexToAddress("0x9999999999999999999999999999999999999999"),
		SrcTxHash:      crypto.Keccak256Hash([]byte(t.Name() + ":source")),
		SrcBlockNumber: 123,
		SrcLogIndex:    4,
		EncodedPacket:  append([]byte{0x01, 0x02}, message...),
		PacketHeader:   []byte{0x01, 0x02},
		Message:        message,
		PayloadHash:    crypto.Keccak256Hash(message),
		Options:        []byte{0x07, 0x08},
		Status:         string(packets.ExecutorNew),
	}
}

type fakeChainClient struct {
	pendingNonce          uint64
	pendingNonceCalls     int
	estimatedGas          uint64
	estimateGasErr        error
	estimateGasCalls      []ethereum.CallMsg
	header                *types.Header
	headerErr             error
	suggestedGasPrice     *big.Int
	suggestGasPriceErr    error
	suggestGasPriceCalls  int
	suggestedGasTipCap    *big.Int
	suggestGasTipCapErr   error
	suggestGasTipCapCalls int
	sent                  []*types.Transaction
	receipts              map[common.Hash]*types.Receipt
}

func (f *fakeChainClient) EstimateGas(_ context.Context, call ethereum.CallMsg) (uint64, error) {
	f.estimateGasCalls = append(f.estimateGasCalls, call)
	if f.estimateGasErr != nil {
		return 0, f.estimateGasErr
	}
	if f.estimatedGas == 0 {
		return 150_000, nil
	}
	return f.estimatedGas, nil
}

func (f *fakeChainClient) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	if f.headerErr != nil {
		return nil, f.headerErr
	}
	if f.header != nil {
		copied := *f.header
		return &copied, nil
	}
	return dynamicHeader(), nil
}

func (f *fakeChainClient) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	f.pendingNonceCalls++
	return f.pendingNonce, nil
}

func (f *fakeChainClient) SuggestGasPrice(context.Context) (*big.Int, error) {
	f.suggestGasPriceCalls++
	if f.suggestGasPriceErr != nil {
		return nil, f.suggestGasPriceErr
	}
	if f.suggestedGasPrice == nil {
		return nil, nil
	}
	return new(big.Int).Set(f.suggestedGasPrice), nil
}

func (f *fakeChainClient) SuggestGasTipCap(context.Context) (*big.Int, error) {
	f.suggestGasTipCapCalls++
	if f.suggestGasTipCapErr != nil {
		return nil, f.suggestGasTipCapErr
	}
	if f.suggestedGasTipCap == nil {
		return big.NewInt(1_000_000_000), nil
	}
	return new(big.Int).Set(f.suggestedGasTipCap), nil
}

func (f *fakeChainClient) SendTransaction(_ context.Context, tx *types.Transaction) error {
	f.sent = append(f.sent, tx)
	return nil
}

func (f *fakeChainClient) TransactionReceipt(_ context.Context, txHash common.Hash) (*types.Receipt, error) {
	receipt, ok := f.receipts[txHash]
	if !ok {
		return nil, ethereum.NotFound
	}
	return receipt, nil
}

func testChains() []config.ChainConfig {
	return []config.ChainConfig{
		{
			EID:             40161,
			Name:            "ethereum-sepolia",
			Family:          config.ChainFamilyEVM,
			ChainID:         11155111,
			EndpointAddress: config.MustEVMAddress("0x1111111111111111111111111111111111111111"),
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8545"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: testExecutorRole(),
			},
		},
		{
			EID:             40245,
			Name:            "base-sepolia",
			Family:          config.ChainFamilyEVM,
			ChainID:         84532,
			EndpointAddress: config.MustEVMAddress("0x4444444444444444444444444444444444444444"),
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8546"},
			TxRoles: config.ChainTxRolesConfig{
				Executor: testExecutorRole(),
			},
		},
	}
}

func testTarget(chainEID uint32, chainID *big.Int, signer *keystore.Signer, client *fakeChainClient, policy FeePolicy) Target {
	return Target{
		ChainEID: chainEID,
		ChainID:  chainID,
		Signer:   signer,
		Client:   client,
		FeePolicies: map[string]FeePolicy{
			"commit-verification":             policy,
			"lz-receive":                      policy,
			executorCommitVerificationPurpose: policy,
			executorLzReceivePurpose:          policy,
			dvnVerifyPurpose:                  policy,
		},
	}
}

func defaultFeePolicy() FeePolicy {
	return FeePolicy{
		ConfiguredMaxFeePerGas:         big.NewInt(10_000_000_000),
		ConfiguredMaxPriorityFeePerGas: big.NewInt(2_000_000_000),
	}
}

func dynamicHeader() *types.Header {
	return &types.Header{BaseFee: big.NewInt(500_000_000)}
}

func legacyHeader() *types.Header {
	return &types.Header{}
}

func testExecutorRole() config.ExecutorTxRoleConfig {
	return config.ExecutorTxRoleConfig{
		Signer:                  config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
		MaxFeePerGasWei:         "2000000000",
		MaxPriorityFeePerGasWei: "1000000000",
	}
}

type fakeRPCDataError struct {
	message string
	data    any
}

func (e fakeRPCDataError) Error() string {
	return e.message
}

func (e fakeRPCDataError) ErrorCode() int {
	return 3
}

func (e fakeRPCDataError) ErrorData() any {
	return e.data
}

func testPathways() []config.PathwayConfig {
	return []config.PathwayConfig{
		{
			SrcEID:     40161,
			DstEID:     40245,
			SrcOApp:    config.MustEVMAddress("0x7777777777777777777777777777777777777777"),
			DstOApp:    config.MustEVMAddress("0x8888888888888888888888888888888888888888"),
			SendLib:    config.MustEVMAddress("0x9999999999999999999999999999999999999999"),
			ReceiveLib: config.MustEVMAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			SourceWorkers: config.WorkerContractsConfig{
				OpenExecutor: config.MustEVMAddress("0x2222222222222222222222222222222222222222"),
				OpenDVN:      config.MustEVMAddress("0x3333333333333333333333333333333333333333"),
			},
			DVN:            config.PathwayDVNConfig{Mode: config.DVNModeShadow},
			Enabled:        true,
			MaxMessageSize: 10000,
		},
	}
}
