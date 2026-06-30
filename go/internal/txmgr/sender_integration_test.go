package txmgr

import (
	"context"
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
	client := &fakeChainClient{pendingNonce: 10}
	manager := New(store, nil)

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID:             40161,
		Purpose:              "commit-verification",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x01, 0x02, 0x03},
		Value:                big.NewInt(123),
		GasLimit:             big.NewInt(100_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), 40161, big.NewInt(11155111), signer, client)
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
	if sent.GasFeeCap().Cmp(big.NewInt(2_000_000_000)) != 0 {
		t.Fatalf("sent gas fee cap = %s", sent.GasFeeCap())
	}
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
}

func TestPrepareReplacementTxPreservesNonceAndBumpsFees(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{pendingNonce: 21}
	manager := New(store, nil)

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID:             40161,
		Purpose:              "lz-receive",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x04, 0x05},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), 40161, big.NewInt(11155111), signer, client)
	if err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}
	if err := store.PrepareReplacementTx(t.Context(), id, big.NewInt(3_000_000_000), big.NewInt(1_500_000_000)); err != nil {
		t.Fatalf("PrepareReplacementTx() error = %v", err)
	}
	replacement, err := store.GetOutboxTx(t.Context(), id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if replacement.Nonce != 21 {
		t.Fatalf("replacement nonce = %d, want 21", replacement.Nonce)
	}
	if replacement.Status != db.TxStatusNonceAssigned {
		t.Fatalf("replacement status = %q, want %q", replacement.Status, db.TxStatusNonceAssigned)
	}
	if replacement.MaxFeePerGas.Cmp(big.NewInt(3_000_000_000)) != 0 {
		t.Fatalf("replacement max fee = %s", replacement.MaxFeePerGas)
	}
	if replacement.MaxPriorityFeePerGas.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("replacement priority fee = %s", replacement.MaxPriorityFeePerGas)
	}
	if replacement.Attempts != 1 {
		t.Fatalf("replacement attempts = %d, want 1", replacement.Attempts)
	}
}

func TestProcessReceiptsMarksBroadcastTxConfirmed(t *testing.T) {
	store := openTestStore(t)
	signer := newTestKeystoreSigner(t)
	client := &fakeChainClient{
		pendingNonce: 33,
		receipts:     make(map[common.Hash]*types.Receipt),
	}
	manager := New(store, nil)

	if _, err := store.EnqueueTx(t.Context(), db.TxRequest{
		ChainEID:             40161,
		Purpose:              "lz-receive",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x04, 0x05},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), 40161, big.NewInt(11155111), signer, client)
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
	manager := New(store, nil)
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
			ChainEID:             packet.DstEID,
			Purpose:              executorLzReceivePurpose,
			GUID:                 packet.GUID.Bytes(),
			To:                   packet.Receiver,
			Calldata:             []byte{0x04, 0x05},
			Value:                big.NewInt(0),
			GasLimit:             big.NewInt(150_000),
			MaxFeePerGas:         big.NewInt(2_000_000_000),
			MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
			SignerID:             signer.Address().Hex(),
		},
	); err != nil {
		t.Fatalf("EnqueueExecutorTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), packet.DstEID, big.NewInt(84532), signer, client)
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
	manager := New(store, nil)
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
			ChainEID:             packet.DstEID,
			Purpose:              executorLzReceivePurpose,
			GUID:                 packet.GUID.Bytes(),
			To:                   packet.Receiver,
			Calldata:             []byte{0x04, 0x05},
			Value:                big.NewInt(0),
			GasLimit:             big.NewInt(150_000),
			MaxFeePerGas:         big.NewInt(2_000_000_000),
			MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
			SignerID:             signer.Address().Hex(),
		},
	); err != nil {
		t.Fatalf("EnqueueExecutorTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), packet.DstEID, big.NewInt(84532), signer, client)
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
	manager := New(store, nil)
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
			ChainEID:             packet.DstEID,
			Purpose:              dvnVerifyPurpose,
			GUID:                 packet.GUID.Bytes(),
			To:                   packet.Receiver,
			Calldata:             []byte{0x06, 0x07},
			Value:                big.NewInt(0),
			GasLimit:             big.NewInt(120_000),
			MaxFeePerGas:         big.NewInt(2_000_000_000),
			MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
			SignerID:             signer.Address().Hex(),
		},
		[]byte(`{"status":"ready"}`),
	); err != nil {
		t.Fatalf("EnqueueDVNVerifyTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), packet.DstEID, big.NewInt(84532), signer, client)
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
	manager := New(store, nil)
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
		ChainEID:             packet.DstEID,
		Purpose:              dvnVerifyPurpose,
		GUID:                 packet.GUID.Bytes(),
		To:                   packet.Receiver,
		Calldata:             []byte{0x06, 0x07},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(120_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
	}, []byte(`{"status":"ready"}`)); err != nil {
		t.Fatalf("EnqueueDVNVerifyTx() error = %v", err)
	}

	id, err := manager.ProcessNext(t.Context(), packet.DstEID, big.NewInt(84532), signer, client)
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
	manager := New(store, nil)
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
		ChainEID:             packet.DstEID,
		Purpose:              dvnVerifyPurpose,
		GUID:                 packet.GUID.Bytes(),
		To:                   common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata:             []byte{0x06, 0x07},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(120_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
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
		ChainEID:             packet.DstEID,
		Purpose:              executorCommitVerificationPurpose,
		GUID:                 packet.GUID.Bytes(),
		To:                   common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata:             []byte{0x08, 0x09},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
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
		ChainEID:             packet.DstEID,
		Purpose:              executorLzReceivePurpose,
		GUID:                 packet.GUID.Bytes(),
		To:                   common.HexToAddress("0x4444444444444444444444444444444444444444"),
		Calldata:             []byte{0x0a, 0x0b},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signer.Address().Hex(),
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
	id, err := manager.ProcessNext(t.Context(), chainEID, chainID, signer, client)
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
	pendingNonce uint64
	sent         []*types.Transaction
	receipts     map[common.Hash]*types.Receipt
}

func (f *fakeChainClient) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return f.pendingNonce, nil
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
			ChainID:         11155111,
			EndpointAddress: "0x1111111111111111111111111111111111111111",
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8545"},
			Workers: config.WorkerContractsConfig{
				OpenExecutor: "0x2222222222222222222222222222222222222222",
				OpenDVN:      "0x3333333333333333333333333333333333333333",
			},
		},
		{
			EID:             40245,
			Name:            "base-sepolia",
			ChainID:         84532,
			EndpointAddress: "0x4444444444444444444444444444444444444444",
			Confirmations:   12,
			RPCURLs:         []string{"http://localhost:8546"},
			Workers: config.WorkerContractsConfig{
				OpenExecutor: "0x5555555555555555555555555555555555555555",
				OpenDVN:      "0x6666666666666666666666666666666666666666",
			},
		},
	}
}

func testPathways() []config.PathwayConfig {
	return []config.PathwayConfig{
		{
			SrcEID:         40161,
			DstEID:         40245,
			SrcOApp:        "0x7777777777777777777777777777777777777777",
			DstOApp:        "0x8888888888888888888888888888888888888888",
			SendLib:        "0x9999999999999999999999999999999999999999",
			ReceiveLib:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Enabled:        true,
			MaxMessageSize: 10000,
		},
	}
}
