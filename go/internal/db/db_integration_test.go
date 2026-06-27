package db

import (
	"context"
	"errors"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

func TestMigrateAndSyncConfig(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()

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

	var chains, pathways int
	if err := store.pool.QueryRow(ctx, "SELECT count(*) FROM chains WHERE eid IN (40161, 40245)").Scan(&chains); err != nil {
		t.Fatalf("count chains: %v", err)
	}
	if err := store.pool.QueryRow(ctx, "SELECT count(*) FROM pathways WHERE src_eid = 40161 AND dst_eid = 40245").Scan(&pathways); err != nil {
		t.Fatalf("count pathways: %v", err)
	}
	if chains != 2 {
		t.Fatalf("chains = %d, want 2", chains)
	}
	if pathways != 1 {
		t.Fatalf("pathways = %d, want 1", pathways)
	}
}

func TestPausePathwayForPacketAndChain(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	cleanPacketRows(ctx, t, store, packet.GUID)
	if _, err := store.pool.Exec(ctx, "UPDATE chains SET paused = false WHERE eid = $1", packet.SrcEID); err != nil {
		t.Fatalf("reset chain pause: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
		UPDATE pathways
		SET paused = false
		WHERE src_eid = $1 AND dst_eid = $2 AND src_oapp = $3 AND dst_oapp = $4
	`, packet.SrcEID, packet.DstEID, addressBytes(packet.Sender), addressBytes(packet.Receiver)); err != nil {
		t.Fatalf("reset pathway pause: %v", err)
	}
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}

	if err := store.PausePathwayForPacket(ctx, packet.GUID); err != nil {
		t.Fatalf("PausePathwayForPacket() error = %v", err)
	}
	if err := store.PauseChain(ctx, packet.SrcEID); err != nil {
		t.Fatalf("PauseChain() error = %v", err)
	}

	var pathwayPaused, chainPaused bool
	if err := store.pool.QueryRow(ctx, `
		SELECT paused
		FROM pathways
		WHERE src_eid = $1 AND dst_eid = $2 AND src_oapp = $3 AND dst_oapp = $4
	`, packet.SrcEID, packet.DstEID, addressBytes(packet.Sender), addressBytes(packet.Receiver)).Scan(&pathwayPaused); err != nil {
		t.Fatalf("select pathway paused: %v", err)
	}
	if err := store.pool.QueryRow(ctx, "SELECT paused FROM chains WHERE eid = $1", packet.SrcEID).Scan(&chainPaused); err != nil {
		t.Fatalf("select chain paused: %v", err)
	}
	if !pathwayPaused {
		t.Fatal("pathway paused = false, want true")
	}
	if !chainPaused {
		t.Fatal("chain paused = false, want true")
	}
}

func TestClaimNextNonceAvoidsCollisions(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()

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

	const signerID = "0x9999999999999999999999999999999999999999"
	if _, err := store.pool.Exec(ctx, "DELETE FROM tx_outbox WHERE signer_id = $1", signerID); err != nil {
		t.Fatalf("delete test rows: %v", err)
	}
	for range 5 {
		if _, err := store.EnqueueTx(ctx, TxRequest{
			ChainEID: 40161,
			Purpose:  "test",
			To:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
			Calldata: []byte{0x01, 0x02},
			Value:    big.NewInt(0),
			SignerID: signerID,
		}); err != nil {
			t.Fatalf("EnqueueTx() error = %v", err)
		}
	}

	nonces := make(chan uint64, 5)
	errs := make(chan error, 5)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			claimed, err := store.ClaimNextNonce(ctx, 40161, signerID, 42)
			if err != nil {
				errs <- err
				return
			}
			nonces <- claimed.Nonce
		})
	}
	wg.Wait()
	close(nonces)
	close(errs)

	for err := range errs {
		t.Fatalf("ClaimNextNonce() error = %v", err)
	}
	seen := make(map[uint64]struct{}, 5)
	for nonce := range nonces {
		seen[nonce] = struct{}{}
	}
	for nonce := uint64(42); nonce < 47; nonce++ {
		if _, ok := seen[nonce]; !ok {
			t.Fatalf("nonce %d was not assigned; assigned=%v", nonce, seen)
		}
	}
}

func TestRetryFailedTxRequeuesWithFreshNonce(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()

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

	const signerID = "0x8888888888888888888888888888888888888888"
	if _, err := store.pool.Exec(ctx, "DELETE FROM tx_outbox WHERE signer_id = $1", signerID); err != nil {
		t.Fatalf("delete test rows: %v", err)
	}
	id, err := store.EnqueueTx(ctx, TxRequest{
		ChainEID:             40161,
		Purpose:              "retry-test",
		To:                   common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Calldata:             []byte{0x01, 0x02},
		Value:                big.NewInt(0),
		GasLimit:             big.NewInt(150_000),
		MaxFeePerGas:         big.NewInt(2_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		SignerID:             signerID,
	})
	if err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}
	claimed, err := store.ClaimNextNonce(ctx, 40161, signerID, 42)
	if err != nil {
		t.Fatalf("ClaimNextNonce() error = %v", err)
	}
	if claimed.Nonce != 42 {
		t.Fatalf("initial nonce = %d, want 42", claimed.Nonce)
	}
	txHash := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	if err := store.MarkTxBroadcast(ctx, id, txHash); err != nil {
		t.Fatalf("MarkTxBroadcast() error = %v", err)
	}
	if err := store.MarkTxFailed(ctx, id, errors.New("receipt reverted")); err != nil {
		t.Fatalf("MarkTxFailed() error = %v", err)
	}

	if err := store.RetryFailedTx(ctx, id, big.NewInt(3_000_000_000), big.NewInt(1_500_000_000)); err != nil {
		t.Fatalf("RetryFailedTx() error = %v", err)
	}
	reclaimed, err := store.ClaimNextNonce(ctx, 40161, signerID, 43)
	if err != nil {
		t.Fatalf("ClaimNextNonce() after retry error = %v", err)
	}
	if reclaimed.ID != id {
		t.Fatalf("reclaimed id = %d, want %d", reclaimed.ID, id)
	}
	if reclaimed.Nonce != 43 {
		t.Fatalf("retry nonce = %d, want 43", reclaimed.Nonce)
	}
	retryTx, err := store.GetOutboxTx(ctx, id)
	if err != nil {
		t.Fatalf("GetOutboxTx() error = %v", err)
	}
	if retryTx.Status != TxStatusNonceAssigned {
		t.Fatalf("status = %q, want %q", retryTx.Status, TxStatusNonceAssigned)
	}
	if retryTx.TxHash != (common.Hash{}) {
		t.Fatalf("tx hash = %s, want zero hash", retryTx.TxHash)
	}
	if retryTx.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", retryTx.Attempts)
	}
	if retryTx.MaxFeePerGas.Cmp(big.NewInt(3_000_000_000)) != 0 {
		t.Fatalf("max fee = %s, want 3000000000", retryTx.MaxFeePerGas)
	}
	if retryTx.MaxPriorityFeePerGas.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("priority fee = %s, want 1500000000", retryTx.MaxPriorityFeePerGas)
	}
}

func TestUpsertPacketPersistsIndexedPacket(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() insert error = %v", err)
	}
	packet.Status = string(packets.ExecutorAssigned)
	packet.SrcBlockNumber = 124
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() update error = %v", err)
	}

	var status string
	var blockNumber uint64
	if err := store.pool.QueryRow(ctx, "SELECT status, src_block_number FROM packets WHERE guid = $1", packet.GUID.Bytes()).Scan(&status, &blockNumber); err != nil {
		t.Fatalf("select packet: %v", err)
	}
	if status != string(packets.ExecutorAssigned) {
		t.Fatalf("status = %q, want %q", status, packets.ExecutorAssigned)
	}
	if blockNumber != 124 {
		t.Fatalf("src_block_number = %d, want 124", blockNumber)
	}
	byGUID, err := store.GetPacket(ctx, packet.GUID)
	if err != nil {
		t.Fatalf("GetPacket() error = %v", err)
	}
	if byGUID.GUID != packet.GUID {
		t.Fatalf("GetPacket() guid = %s, want %s", byGUID.GUID, packet.GUID)
	}
	byDestination, err := store.GetPacketByDestination(ctx, packet.DstEID, packet.SrcEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64())
	if err != nil {
		t.Fatalf("GetPacketByDestination() error = %v", err)
	}
	if byDestination.GUID != packet.GUID {
		t.Fatalf("GetPacketByDestination() guid = %s, want %s", byDestination.GUID, packet.GUID)
	}
}

func TestUpsertExecutorJobPersistsAssignment(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(ctx, ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorAssigned),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}

	var assigned bool
	var fee string
	var status string
	if err := store.pool.QueryRow(ctx, "SELECT assigned, assigned_fee::text, status FROM executor_jobs WHERE guid = $1", packet.GUID.Bytes()).Scan(&assigned, &fee, &status); err != nil {
		t.Fatalf("select executor job: %v", err)
	}
	if !assigned {
		t.Fatal("assigned = false, want true")
	}
	if fee != "42" {
		t.Fatalf("assigned_fee = %q, want 42", fee)
	}
	if status != string(packets.ExecutorAssigned) {
		t.Fatalf("status = %q, want %q", status, packets.ExecutorAssigned)
	}
}

func TestIndexerCursorPersistsMonotonicProgress(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	const stream = "executor_source"
	if _, err := store.pool.Exec(ctx, "DELETE FROM indexer_cursors WHERE chain_eid = $1 AND stream = $2", 40161, stream); err != nil {
		t.Fatalf("delete cursor: %v", err)
	}
	if err := store.UpdateIndexerCursor(ctx, 40161, stream, 100); err != nil {
		t.Fatalf("UpdateIndexerCursor() insert error = %v", err)
	}
	if err := store.UpdateIndexerCursor(ctx, 40161, stream, 90); err != nil {
		t.Fatalf("UpdateIndexerCursor() lower update error = %v", err)
	}
	cursor, err := store.GetIndexerCursor(ctx, 40161, stream)
	if err != nil {
		t.Fatalf("GetIndexerCursor() error = %v", err)
	}
	if cursor != 100 {
		t.Fatalf("cursor = %d, want 100", cursor)
	}
	if err := store.UpdateIndexerCursor(ctx, 40161, stream, 101); err != nil {
		t.Fatalf("UpdateIndexerCursor() advance error = %v", err)
	}
	cursor, err = store.GetIndexerCursor(ctx, 40161, stream)
	if err != nil {
		t.Fatalf("GetIndexerCursor() after advance error = %v", err)
	}
	if cursor != 101 {
		t.Fatalf("cursor = %d, want 101", cursor)
	}
}

func TestUpsertDVNJobPersistsAssignment(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertDVNJob(ctx, DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNAssigned),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}
	work, err := store.ListDVNWork(ctx, string(packets.DVNAssigned), 10)
	if err != nil {
		t.Fatalf("ListDVNWork() error = %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("work length = %d, want 1", len(work))
	}
	if work[0].Packet.GUID != packet.GUID {
		t.Fatalf("work packet guid = %s, want %s", work[0].Packet.GUID, packet.GUID)
	}
	if work[0].Job.ConfirmationsRequired != 12 {
		t.Fatalf("confirmations = %d, want 12", work[0].Job.ConfirmationsRequired)
	}
	if err := store.MarkDVNWaitingConfirmations(ctx, packet.GUID, string(packets.DVNAssigned)); err != nil {
		t.Fatalf("MarkDVNWaitingConfirmations() error = %v", err)
	}
	if err := store.MarkDVNQuorumChecking(ctx, packet.GUID, string(packets.DVNWaitingConfirmations)); err != nil {
		t.Fatalf("MarkDVNQuorumChecking() error = %v", err)
	}
	work, err = store.ListDVNWork(ctx, string(packets.DVNQuorumChecking), 10)
	if err != nil {
		t.Fatalf("ListDVNWork() quorum error = %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("quorum work length = %d, want 1", len(work))
	}
	report := []byte(`{"status":"would_verify"}`)
	if err := store.MarkDVNWouldVerify(ctx, packet.GUID, string(packets.DVNQuorumChecking), report); err != nil {
		t.Fatalf("MarkDVNWouldVerify() error = %v", err)
	}
	var status string
	var quorumResult string
	if err := store.pool.QueryRow(ctx, "SELECT status, quorum_result::text FROM dvn_jobs WHERE guid = $1", packet.GUID.Bytes()).Scan(&status, &quorumResult); err != nil {
		t.Fatalf("select dvn report: %v", err)
	}
	if status != string(packets.DVNWouldVerify) {
		t.Fatalf("status = %q, want %q", status, packets.DVNWouldVerify)
	}
	if quorumResult == "" {
		t.Fatal("quorum_result is empty")
	}
}

func TestExecutorWorkEnqueueAdvancesStatusAtomically(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorVerifiable)
	if _, err := store.pool.Exec(ctx, "DELETE FROM tx_outbox WHERE guid = $1", packet.GUID.Bytes()); err != nil {
		t.Fatalf("delete tx_outbox: %v", err)
	}
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(ctx, ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorVerifiable),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}

	work, err := store.ListExecutorWork(ctx, string(packets.ExecutorVerifiable), 10)
	if err != nil {
		t.Fatalf("ListExecutorWork() error = %v", err)
	}
	if len(work) != 1 {
		t.Fatalf("work len = %d, want 1", len(work))
	}
	if work[0].Packet.GUID != packet.GUID {
		t.Fatalf("work guid = %s, want %s", work[0].Packet.GUID, packet.GUID)
	}

	id, err := store.EnqueueExecutorTx(ctx, packet.GUID, string(packets.ExecutorVerifiable), string(packets.ExecutorCommitTxEnqueued), TxRequest{
		ChainEID: 40245,
		Purpose:  "executor_commit_verification",
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata: []byte{0x01, 0x02},
		Value:    big.NewInt(0),
		SignerID: "0x9999999999999999999999999999999999999999",
	})
	if err != nil {
		t.Fatalf("EnqueueExecutorTx() error = %v", err)
	}
	if id == 0 {
		t.Fatal("outbox id = 0, want persisted id")
	}

	var packetStatus, jobStatus, purpose string
	if err := store.pool.QueryRow(ctx, `
		SELECT p.status, ej.status, tx.purpose
		FROM packets p
		JOIN executor_jobs ej ON ej.guid = p.guid
		JOIN tx_outbox tx ON tx.guid = p.guid
		WHERE p.guid = $1
	`, packet.GUID.Bytes()).Scan(&packetStatus, &jobStatus, &purpose); err != nil {
		t.Fatalf("select transitioned rows: %v", err)
	}
	if packetStatus != string(packets.ExecutorCommitTxEnqueued) {
		t.Fatalf("packet status = %q, want %q", packetStatus, packets.ExecutorCommitTxEnqueued)
	}
	if jobStatus != string(packets.ExecutorCommitTxEnqueued) {
		t.Fatalf("job status = %q, want %q", jobStatus, packets.ExecutorCommitTxEnqueued)
	}
	if purpose != "executor_commit_verification" {
		t.Fatalf("purpose = %q, want executor_commit_verification", purpose)
	}
}

func TestExecutorReceiptTransitionsPersistTxHashes(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorCommitTxEnqueued)
	if _, err := store.pool.Exec(ctx, "DELETE FROM tx_outbox WHERE guid = $1", packet.GUID.Bytes()); err != nil {
		t.Fatalf("delete tx_outbox: %v", err)
	}
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(ctx, ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorCommitTxEnqueued),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}

	commitHash := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	if err := store.MarkExecutorCommitted(ctx, packet.GUID, commitHash); err != nil {
		t.Fatalf("MarkExecutorCommitted() error = %v", err)
	}
	if err := store.MarkExecutorExecutable(ctx, packet.GUID); err != nil {
		t.Fatalf("MarkExecutorExecutable() error = %v", err)
	}
	receiveHash := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
	if err := store.MarkExecutorReceiveFailed(ctx, packet.GUID, receiveHash, "lzReceive reverted"); err == nil {
		t.Fatal("MarkExecutorReceiveFailed() error = nil, want wrong-state error")
	}
	if _, err := store.pool.Exec(ctx, "UPDATE executor_jobs SET status = $1 WHERE guid = $2", string(packets.ExecutorLzReceiveTxEnqueued), packet.GUID.Bytes()); err != nil {
		t.Fatalf("force receive status: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "UPDATE packets SET status = $1 WHERE guid = $2", string(packets.ExecutorLzReceiveTxEnqueued), packet.GUID.Bytes()); err != nil {
		t.Fatalf("force packet receive status: %v", err)
	}
	if err := store.MarkExecutorDelivered(ctx, packet.GUID, receiveHash); err != nil {
		t.Fatalf("MarkExecutorDelivered() error = %v", err)
	}

	var packetStatus, jobStatus string
	var commitBytes, receiveBytes []byte
	if err := store.pool.QueryRow(ctx, `
		SELECT p.status, ej.status, ej.commit_tx_hash, ej.receive_tx_hash
		FROM packets p
		JOIN executor_jobs ej ON ej.guid = p.guid
		WHERE p.guid = $1
	`, packet.GUID.Bytes()).Scan(&packetStatus, &jobStatus, &commitBytes, &receiveBytes); err != nil {
		t.Fatalf("select receipt rows: %v", err)
	}
	if packetStatus != string(packets.ExecutorDelivered) {
		t.Fatalf("packet status = %q, want %q", packetStatus, packets.ExecutorDelivered)
	}
	if jobStatus != string(packets.ExecutorDelivered) {
		t.Fatalf("job status = %q, want %q", jobStatus, packets.ExecutorDelivered)
	}
	if common.BytesToHash(commitBytes) != commitHash {
		t.Fatalf("commit tx hash = %s, want %s", common.BytesToHash(commitBytes), commitHash)
	}
	if common.BytesToHash(receiveBytes) != receiveHash {
		t.Fatalf("receive tx hash = %s, want %s", common.BytesToHash(receiveBytes), receiveHash)
	}
}

func TestCheckDrainStatusReportsPendingWork(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorExecutable)
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(ctx, ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorExecutable),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}
	if err := store.UpsertDVNJob(ctx, DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNWaitingConfirmations),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}
	if _, err := store.EnqueueTx(ctx, TxRequest{
		ChainEID: 40245,
		Purpose:  "executor_lz_receive",
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata: []byte{0x01, 0x02},
		Value:    big.NewInt(0),
		SignerID: "0x9999999999999999999999999999999999999999",
	}); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}

	status, err := store.CheckDrainStatus(ctx, packet.SrcEID, packet.DstEID)
	if err != nil {
		t.Fatalf("CheckDrainStatus() error = %v", err)
	}
	if status.Ready {
		t.Fatal("ready = true, want false")
	}
	if status.PacketsTotal != 1 {
		t.Fatalf("packets total = %d, want 1", status.PacketsTotal)
	}
	if got := statusCount(status.ExecutorPending, string(packets.ExecutorExecutable)); got != 1 {
		t.Fatalf("executor pending executable = %d, want 1", got)
	}
	if got := statusCount(status.DVNPending, string(packets.DVNWaitingConfirmations)); got != 1 {
		t.Fatalf("dvn pending waiting confirmations = %d, want 1", got)
	}
	if got := statusCount(status.OutboxPending, TxStatusQueued); got != 1 {
		t.Fatalf("outbox pending queued = %d, want 1", got)
	}
	if status.VerifiedButUndeliveredCount != 1 {
		t.Fatalf("verified but undelivered = %d, want 1", status.VerifiedButUndeliveredCount)
	}
}

func TestCheckDrainStatusAcceptsDeliveredShadowPathway(t *testing.T) {
	databaseURL := os.Getenv("TEST_POSTGRES_URL")
	if databaseURL == "" {
		t.Skip("TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer store.Close()
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

	packet := testPacketRecord()
	packet.Status = string(packets.ExecutorDelivered)
	cleanPacketRows(ctx, t, store, packet.GUID)
	if err := store.UpsertPacket(ctx, packet); err != nil {
		t.Fatalf("UpsertPacket() error = %v", err)
	}
	if err := store.UpsertExecutorJob(ctx, ExecutorJobRecord{
		GUID:        packet.GUID,
		AssignedFee: big.NewInt(42),
		Status:      string(packets.ExecutorDelivered),
	}); err != nil {
		t.Fatalf("UpsertExecutorJob() error = %v", err)
	}
	if err := store.UpsertDVNJob(ctx, DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNWouldVerify),
	}); err != nil {
		t.Fatalf("UpsertDVNJob() error = %v", err)
	}
	id, err := store.EnqueueTx(ctx, TxRequest{
		ChainEID: 40245,
		Purpose:  "executor_lz_receive",
		GUID:     packet.GUID.Bytes(),
		To:       common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Calldata: []byte{0x01, 0x02},
		Value:    big.NewInt(0),
		SignerID: "0x9999999999999999999999999999999999999999",
	})
	if err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}
	if err := store.MarkTxConfirmed(ctx, id, common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")); err != nil {
		t.Fatalf("MarkTxConfirmed() error = %v", err)
	}

	status, err := store.CheckDrainStatus(ctx, packet.SrcEID, packet.DstEID)
	if err != nil {
		t.Fatalf("CheckDrainStatus() error = %v", err)
	}
	if !status.Ready {
		t.Fatalf("ready = false, status = %+v", status)
	}
	if len(status.ExecutorPending) != 0 || len(status.DVNPending) != 0 || len(status.OutboxPending) != 0 {
		t.Fatalf("pending counts are not empty: %+v", status)
	}
}

func statusCount(counts []StatusCount, status string) int64 {
	for _, count := range counts {
		if count.Status == status {
			return count.Count
		}
	}
	return 0
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

func testPacketRecord() PacketRecord {
	return PacketRecord{
		GUID:           common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SrcEID:         40161,
		DstEID:         40245,
		Nonce:          big.NewInt(7),
		Sender:         common.HexToAddress("0x7777777777777777777777777777777777777777"),
		Receiver:       common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:        common.HexToAddress("0x9999999999999999999999999999999999999999"),
		SrcTxHash:      common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		SrcBlockNumber: 123,
		SrcLogIndex:    4,
		EncodedPacket:  []byte{0x01, 0x02},
		PacketHeader:   []byte{0x03, 0x04},
		Message:        []byte{0x05, 0x06},
		PayloadHash:    common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
		Options:        []byte{0x07, 0x08},
		Status:         string(packets.ExecutorNew),
	}
}

func cleanPacketRows(ctx context.Context, t *testing.T, store *Store, guid common.Hash) {
	t.Helper()
	if _, err := store.pool.Exec(ctx, "DELETE FROM tx_outbox WHERE guid = $1", guid.Bytes()); err != nil {
		t.Fatalf("delete tx_outbox: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM dvn_jobs WHERE guid = $1", guid.Bytes()); err != nil {
		t.Fatalf("delete dvn job: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM executor_jobs WHERE guid = $1", guid.Bytes()); err != nil {
		t.Fatalf("delete executor job: %v", err)
	}
	if _, err := store.pool.Exec(ctx, "DELETE FROM packets WHERE guid = $1", guid.Bytes()); err != nil {
		t.Fatalf("delete packet: %v", err)
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
