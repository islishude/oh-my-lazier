package indexer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
	"github.com/jackc/pgx/v5"
)

func TestIndexerProcessOnceBackfillsSourceExecutorAssignment(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	sourceLogs := testExecutorSourceLogs(t, executor, sendLib, big.NewInt(42))
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", executor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.pollInterval = time.Millisecond

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceFromBlock != 0 || result.SourceToBlock != 188 {
		t.Fatalf("source window = %d..%d, want 0..188", result.SourceFromBlock, result.SourceToBlock)
	}
	if result.SourceTransactions != 1 {
		t.Fatalf("SourceTransactions = %d, want 1", result.SourceTransactions)
	}
	if len(store.packets) != 1 || len(store.jobs) != 1 {
		t.Fatalf("stored packets/jobs = %d/%d, want 1/1", len(store.packets), len(store.jobs))
	}
	for guid, packet := range store.packets {
		if packet.Status != string(packets.ExecutorAssigned) {
			t.Fatalf("packet status = %q, want %q", packet.Status, packets.ExecutorAssigned)
		}
		if store.jobs[guid].AssignedFee.Cmp(big.NewInt(42)) != 0 {
			t.Fatalf("assigned fee = %s, want 42", store.jobs[guid].AssignedFee)
		}
	}
	if len(client.queries) != 2 {
		t.Fatalf("queries = %d, want source and destination queries", len(client.queries))
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want 188", store.cursors[cursorKey(40161, executorSourceStream)])
	}
	if store.cursors[cursorKey(40161, executorDestStream)] != 188 {
		t.Fatalf("destination cursor = %d, want 188", store.cursors[cursorKey(40161, executorDestStream)])
	}
}

func TestIndexerProcessOnceMarksUnsupportedExecutorOptionsManualReview(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	sourceLogs := []gethtypes.Log{
		testPacketSentLog(t, txHash, sendLib, 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(42), 1),
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(42), unsupportedExecutorOptions(), 2),
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", executor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceTransactions != 1 {
		t.Fatalf("SourceTransactions = %d, want 1", result.SourceTransactions)
	}
	for guid, packet := range store.packets {
		if packet.Status != string(packets.ExecutorManualReview) {
			t.Fatalf("packet status = %q, want %q", packet.Status, packets.ExecutorManualReview)
		}
		if store.jobs[guid].Status != string(packets.ExecutorManualReview) {
			t.Fatalf("job status = %q, want %q", store.jobs[guid].Status, packets.ExecutorManualReview)
		}
		if store.jobs[guid].LastError == "" {
			t.Fatal("job LastError is empty, want unsupported options detail")
		}
	}
}

func TestIndexerProcessOnceFiltersUnexpectedExecutorWorker(t *testing.T) {
	configuredExecutor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	otherExecutor := common.HexToAddress("0x2323232323232323232323232323232323232323")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testExecutorSourceLogs(t, otherExecutor, sendLib, big.NewInt(42)),
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", configuredExecutor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceTransactions != 0 {
		t.Fatalf("SourceTransactions = %d, want 0", result.SourceTransactions)
	}
	if len(store.packets) != 0 || len(store.jobs) != 0 {
		t.Fatalf("stored packets/jobs = %d/%d, want 0/0", len(store.packets), len(store.jobs))
	}
}

func TestIndexerProcessOnceBackfillsDestinationEvents(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Status = string(packets.ExecutorCommitTxEnqueued)
	store := newFakeIndexerStore()
	store.packets[packet.GUID] = packet
	store.jobs[packet.GUID] = db.ExecutorJobRecord{
		GUID:   packet.GUID,
		Status: string(packets.ExecutorCommitTxEnqueued),
	}
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)},
	}
	indexer := NewWithClient(
		testIndexerChain(packet.DstEID, "base-sepolia", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 1 {
		t.Fatalf("DestinationLogs = %d, want 1", result.DestinationLogs)
	}
	if store.committedGUID != packet.GUID {
		t.Fatalf("committed guid = %s, want %s", store.committedGUID, packet.GUID)
	}
}

func TestIndexerProcessOnceBackfillsDVNVerification(t *testing.T) {
	packet := testDestinationPacketRecord()
	store := newFakeIndexerStore()
	store.packets[packet.GUID] = packet
	store.dvnJobs[packet.GUID] = db.DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNVerifyTxEnqueued),
	}
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPayloadVerifiedLog(t, packet, common.HexToAddress("0x3333333333333333333333333333333333333333"))},
	}
	indexer := NewWithClient(
		testIndexerChain(packet.DstEID, "base-sepolia", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 1 {
		t.Fatalf("DestinationLogs = %d, want 1", result.DestinationLogs)
	}
	if store.dvnVerifiedGUID != packet.GUID {
		t.Fatalf("dvn verified guid = %s, want %s", store.dvnVerifiedGUID, packet.GUID)
	}
	if !queriesHaveAddress(client.queries, testIndexerPathway().ReceiveLib) {
		t.Fatal("destination query does not include receive lib")
	}
}

func TestIndexerProcessOnceBackfillsDVNAssignment(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testDVNSourceLogs(t, dvn, sendLib, big.NewInt(42)),
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DVNTransactions != 1 {
		t.Fatalf("DVNTransactions = %d, want 1", result.DVNTransactions)
	}
	if len(store.packets) != 1 || len(store.dvnJobs) != 1 {
		t.Fatalf("stored packets/dvn jobs = %d/%d, want 1/1", len(store.packets), len(store.dvnJobs))
	}
	for guid, job := range store.dvnJobs {
		if job.ConfirmationsRequired != 12 {
			t.Fatalf("confirmations = %d, want 12", job.ConfirmationsRequired)
		}
		if store.packets[guid].GUID != guid {
			t.Fatalf("packet for dvn job %s was not stored", guid)
		}
	}
}

func TestIndexerProcessOnceFiltersUnexpectedDVNWorker(t *testing.T) {
	otherDVN := common.HexToAddress("0x3434343434343434343434343434343434343434")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testDVNSourceLogs(t, otherDVN, sendLib, big.NewInt(42)),
	}
	indexer := NewWithClient(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DVNTransactions != 0 {
		t.Fatalf("DVNTransactions = %d, want 0", result.DVNTransactions)
	}
	if len(store.packets) != 0 || len(store.dvnJobs) != 0 {
		t.Fatalf("stored packets/dvn jobs = %d/%d, want 0/0", len(store.packets), len(store.dvnJobs))
	}
}

func TestIndexerProcessOnceWaitsForConfirmations(t *testing.T) {
	client := &fakeLogClient{head: 11}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		newFakeIndexerStore(),
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.ObservedHeadBlock != 11 {
		t.Fatalf("ObservedHeadBlock = %d, want 11", result.ObservedHeadBlock)
	}
	result.ObservedHeadBlock = 0
	if result != (ProcessResult{}) {
		t.Fatalf("result = %+v, want no indexed windows", result)
	}
	if len(client.queries) != 0 {
		t.Fatalf("queries = %d, want none before confirmations", len(client.queries))
	}
}

func TestIndexerProcessOnceUsesPersistedCursor(t *testing.T) {
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, executorSourceStream)] = 40
	store.cursors[cursorKey(40161, executorDestStream)] = 40
	client := &fakeLogClient{head: 65}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.backfillRange = 10

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceFromBlock != 41 || result.SourceToBlock != 50 {
		t.Fatalf("source window = %d..%d, want 41..50", result.SourceFromBlock, result.SourceToBlock)
	}
	if result.DestinationFromBlock != 41 || result.DestinationToBlock != 50 {
		t.Fatalf("destination window = %d..%d, want 41..50", result.DestinationFromBlock, result.DestinationToBlock)
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 50 {
		t.Fatalf("source cursor = %d, want 50", store.cursors[cursorKey(40161, executorSourceStream)])
	}
	if store.cursors[cursorKey(40161, executorDestStream)] != 50 {
		t.Fatalf("destination cursor = %d, want 50", store.cursors[cursorKey(40161, executorDestStream)])
	}
}

func TestIndexerProcessOnceSplitsSourceQueriesByConfiguredRange(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 37}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.IndexerQueryBlockRange = 10
	indexer := NewWithClient(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceFromBlock != 0 || result.SourceToBlock != 25 {
		t.Fatalf("source window = %d..%d, want 0..25", result.SourceFromBlock, result.SourceToBlock)
	}
	var got [][2]uint64
	for _, query := range client.queries {
		if queryHasTopic(query, lzabi.PacketSentTopic()) {
			got = append(got, [2]uint64{query.FromBlock.Uint64(), query.ToBlock.Uint64()})
		}
	}
	want := [][2]uint64{{0, 9}, {10, 19}, {20, 25}}
	if !slices.Equal(got, want) {
		t.Fatalf("source query ranges = %v, want %v", got, want)
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 25 {
		t.Fatalf("source cursor = %d, want 25", store.cursors[cursorKey(40161, executorSourceStream)])
	}
}

func TestIndexerProcessOnceSplitsDestinationQueriesByConfiguredRange(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 37}
	configuredChain := testIndexerChain(40245, "base-sepolia", common.HexToAddress("0x5555555555555555555555555555555555555555"))
	configuredChain.IndexerQueryBlockRange = 10
	indexer := NewWithClient(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationFromBlock != 0 || result.DestinationToBlock != 25 {
		t.Fatalf("destination window = %d..%d, want 0..25", result.DestinationFromBlock, result.DestinationToBlock)
	}
	var got [][2]uint64
	for _, query := range client.queries {
		if queryHasTopic(query, lzabi.PacketVerifiedTopic()) {
			got = append(got, [2]uint64{query.FromBlock.Uint64(), query.ToBlock.Uint64()})
		}
	}
	want := [][2]uint64{{0, 9}, {10, 19}, {20, 25}}
	if !slices.Equal(got, want) {
		t.Fatalf("destination query ranges = %v, want %v", got, want)
	}
	if store.cursors[cursorKey(40245, executorDestStream)] != 25 {
		t.Fatalf("destination cursor = %d, want 25", store.cursors[cursorKey(40245, executorDestStream)])
	}
}

func TestIndexerProcessOnceProcessesSourceLogsAsContiguousTransactions(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	firstTx := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secondTx := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	sourceLogs := []gethtypes.Log{
		testPacketSentLog(t, firstTx, sendLib, 0),
		testExecutorFeePaidLog(t, firstTx, sendLib, executor, big.NewInt(42), 1),
		testExecutorJobAssignedLogWithOptions(t, firstTx, executor, sendLib, big.NewInt(42), validExecutorOptions(), 2),
		testPacketSentLog(t, secondTx, sendLib, 3),
		testExecutorFeePaidLog(t, secondTx, sendLib, executor, big.NewInt(42), 4),
		testExecutorJobAssignedLogWithOptions(t, secondTx, executor, sendLib, big.NewInt(42), validExecutorOptions(), 5),
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", executor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceTransactions != 2 {
		t.Fatalf("SourceTransactions = %d, want 2", result.SourceTransactions)
	}
}

func TestIndexerProcessOnceUsesConfiguredStartBlockWhenCursorMissing(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.StartBlockNumber = 150
	indexer := NewWithClient(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceFromBlock != 150 || result.SourceToBlock != 188 {
		t.Fatalf("source window = %d..%d, want 150..188", result.SourceFromBlock, result.SourceToBlock)
	}
	if result.DestinationFromBlock != 150 || result.DestinationToBlock != 188 {
		t.Fatalf("destination window = %d..%d, want 150..188", result.DestinationFromBlock, result.DestinationToBlock)
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want 188", store.cursors[cursorKey(40161, executorSourceStream)])
	}
	if store.cursors[cursorKey(40161, executorDestStream)] != 188 {
		t.Fatalf("destination cursor = %d, want 188", store.cursors[cursorKey(40161, executorDestStream)])
	}
}

func TestIndexerProcessOnceSkipsUntilStartBlockIsConfirmed(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.StartBlockNumber = 250
	indexer := NewWithClient(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.SourceFromBlock != 0 || result.SourceToBlock != 0 || result.DestinationFromBlock != 0 || result.DestinationToBlock != 0 {
		t.Fatalf("result windows = %+v, want no indexed windows", result)
	}
	if len(client.queries) != 0 {
		t.Fatalf("queries = %d, want none before configured start block is confirmed", len(client.queries))
	}
	if len(store.cursors) != 0 {
		t.Fatalf("cursors = %d, want none before configured start block is confirmed", len(store.cursors))
	}
}

func TestIndexerProcessOnceFailureDoesNotAdvanceCursor(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200, filterErr: errors.New("filter unavailable")}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	if _, err := indexer.ProcessOnce(context.Background()); err == nil {
		t.Fatal("ProcessOnce() error = nil, want filter error")
	}
	if len(store.cursors) != 0 {
		t.Fatalf("cursors = %d, want none after failed poll", len(store.cursors))
	}
}

func TestIndexerRunPollsImmediatelyAndOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var blockCalls atomic.Int32
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head: 200,
		onBlock: func() {
			if blockCalls.Add(1) == 2 {
				cancel()
			}
		},
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.pollInterval = time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- indexer.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not poll on interval")
	}
	if got := blockCalls.Load(); got < 2 {
		t.Fatalf("BlockNumber calls = %d, want at least 2", got)
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want 188", store.cursors[cursorKey(40161, executorSourceStream)])
	}
}

func TestIndexerRunRetriesAfterPollError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var blockCalls atomic.Int32
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200}
	client.onBlock = func() {
		switch blockCalls.Add(1) {
		case 1:
			client.blockErr = errors.New("rpc unavailable")
		case 2:
			client.blockErr = nil
		case 3:
			cancel()
		}
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.pollInterval = time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- indexer.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not continue after the first poll error")
	}
	if got := blockCalls.Load(); got < 3 {
		t.Fatalf("BlockNumber calls = %d, want at least 3", got)
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want retry to advance to 188", store.cursors[cursorKey(40161, executorSourceStream)])
	}
}

func TestIndexerRunContinuesPollingAfterImmatureHead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var blockCalls atomic.Int32
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 5}
	client.onBlock = func() {
		switch blockCalls.Add(1) {
		case 2:
			client.head = 200
		case 3:
			cancel()
		}
	}
	indexer := NewWithClient(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.pollInterval = time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- indexer.Run(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not continue polling after immature head")
	}
	if len(client.queries) == 0 {
		t.Fatal("queries = 0, want polling to resume after the configured confirmations are available")
	}
	if store.cursors[cursorKey(40161, executorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want 188", store.cursors[cursorKey(40161, executorSourceStream)])
	}
}

func TestIndexerRunFailsFastForLocalSetupErrors(t *testing.T) {
	tests := []struct {
		name      string
		indexer   *Indexer
		wantError string
	}{
		{
			name: "poll interval",
			indexer: func() *Indexer {
				indexer := NewWithClient(
					testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
					[]chain.Pathway{testIndexerPathway()},
					newFakeIndexerStore(),
					&fakeLogClient{head: 200},
					discardLogger(),
				)
				indexer.pollInterval = 0
				return indexer
			}(),
			wantError: "poll interval",
		},
		{
			name: "store",
			indexer: NewWithClient(
				testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
				[]chain.Pathway{testIndexerPathway()},
				nil,
				&fakeLogClient{head: 200},
				discardLogger(),
			),
			wantError: "store is required",
		},
		{
			name: "log client",
			indexer: NewWithClient(
				testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
				[]chain.Pathway{testIndexerPathway()},
				newFakeIndexerStore(),
				nil,
				discardLogger(),
			),
			wantError: "log client is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.indexer.Run(context.Background())
			if err == nil {
				t.Fatal("Run() error = nil, want setup error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

type fakeLogClient struct {
	head            uint64
	blockErr        error
	filterErr       error
	sourceLogs      []gethtypes.Log
	destinationLogs []gethtypes.Log
	queries         []ethereum.FilterQuery
	onBlock         func()
}

func (c *fakeLogClient) BlockNumber(context.Context) (uint64, error) {
	if c.onBlock != nil {
		c.onBlock()
	}
	if c.blockErr != nil {
		return 0, c.blockErr
	}
	return c.head, nil
}

func (c *fakeLogClient) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error) {
	if c.filterErr != nil {
		return nil, c.filterErr
	}
	c.queries = append(c.queries, query)
	if queryHasTopic(query, lzabi.PacketSentTopic()) {
		return append([]gethtypes.Log(nil), c.sourceLogs...), nil
	}
	if queryHasTopic(query, lzabi.PacketVerifiedTopic()) {
		return append([]gethtypes.Log(nil), c.destinationLogs...), nil
	}
	return nil, nil
}

type fakeIndexerStore struct {
	packets         map[common.Hash]db.PacketRecord
	jobs            map[common.Hash]db.ExecutorJobRecord
	dvnJobs         map[common.Hash]db.DVNJobRecord
	cursors         map[string]uint64
	committedGUID   common.Hash
	dvnVerifiedGUID common.Hash
}

func newFakeIndexerStore() *fakeIndexerStore {
	return &fakeIndexerStore{
		packets: make(map[common.Hash]db.PacketRecord),
		jobs:    make(map[common.Hash]db.ExecutorJobRecord),
		dvnJobs: make(map[common.Hash]db.DVNJobRecord),
		cursors: make(map[string]uint64),
	}
}

func (s *fakeIndexerStore) GetIndexerCursor(_ context.Context, chainEID uint32, stream string) (uint64, error) {
	cursor, ok := s.cursors[cursorKey(chainEID, stream)]
	if !ok {
		return 0, pgx.ErrNoRows
	}
	return cursor, nil
}

func (s *fakeIndexerStore) UpdateIndexerCursor(_ context.Context, chainEID uint32, stream string, lastBlock uint64) error {
	key := cursorKey(chainEID, stream)
	if s.cursors[key] < lastBlock {
		s.cursors[key] = lastBlock
	}
	return nil
}

func (s *fakeIndexerStore) UpsertPacket(_ context.Context, packet db.PacketRecord) error {
	s.packets[packet.GUID] = packet
	return nil
}

func (s *fakeIndexerStore) UpsertExecutorJob(_ context.Context, job db.ExecutorJobRecord) error {
	s.jobs[job.GUID] = job
	return nil
}

func (s *fakeIndexerStore) UpsertDVNJob(_ context.Context, job db.DVNJobRecord) error {
	s.dvnJobs[job.GUID] = job
	return nil
}

func (s *fakeIndexerStore) GetPacket(_ context.Context, guid common.Hash) (db.PacketRecord, error) {
	packet, ok := s.packets[guid]
	if !ok {
		return db.PacketRecord{}, pgx.ErrNoRows
	}
	return packet, nil
}

func (s *fakeIndexerStore) GetExecutorJob(_ context.Context, guid common.Hash) (db.ExecutorJobRecord, error) {
	job, ok := s.jobs[guid]
	if !ok {
		return db.ExecutorJobRecord{}, pgx.ErrNoRows
	}
	return job, nil
}

func (s *fakeIndexerStore) GetPacketByDestination(_ context.Context, dstEID, srcEID uint32, sender, receiver common.Address, nonce uint64) (db.PacketRecord, error) {
	for _, packet := range s.packets {
		if packet.DstEID == dstEID && packet.SrcEID == srcEID && packet.Sender == sender && packet.Receiver == receiver && packet.Nonce.Uint64() == nonce {
			return packet, nil
		}
	}
	return db.PacketRecord{}, pgx.ErrNoRows
}

func (s *fakeIndexerStore) GetPacketByVerification(_ context.Context, dstEID uint32, packetHeader []byte, payloadHash common.Hash) (db.PacketRecord, error) {
	for _, packet := range s.packets {
		if packet.DstEID == dstEID && string(packet.PacketHeader) == string(packetHeader) && packet.PayloadHash == payloadHash {
			return packet, nil
		}
	}
	return db.PacketRecord{}, pgx.ErrNoRows
}

func (s *fakeIndexerStore) GetDVNJob(_ context.Context, guid common.Hash) (db.DVNJobRecord, error) {
	job, ok := s.dvnJobs[guid]
	if !ok {
		return db.DVNJobRecord{}, pgx.ErrNoRows
	}
	return job, nil
}

func (s *fakeIndexerStore) MarkExecutorCommitted(_ context.Context, guid, _ common.Hash) error {
	s.committedGUID = guid
	return nil
}

func (s *fakeIndexerStore) MarkExecutorDelivered(context.Context, common.Hash, common.Hash) error {
	return nil
}

func (s *fakeIndexerStore) MarkExecutorReceiveFailed(context.Context, common.Hash, common.Hash, string) error {
	return nil
}

func (s *fakeIndexerStore) MarkDVNVerified(_ context.Context, guid, _ common.Hash) error {
	s.dvnVerifiedGUID = guid
	if job, ok := s.dvnJobs[guid]; ok {
		job.Status = string(packets.DVNVerified)
		s.dvnJobs[guid] = job
	}
	return nil
}

func testIndexerChain(eid uint32, name string, executor common.Address) chain.Chain {
	return chain.Chain{
		EID:             eid,
		Name:            name,
		EndpointAddress: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Confirmations:   12,
		TxRoles: chain.TxRoles{
			Executor: chain.ExecutorTxRole{SignerID: executor.Hex()},
		},
	}
}

func testIndexerPathway() chain.Pathway {
	return chain.Pathway{
		SrcEID:     40161,
		DstEID:     40245,
		SrcOApp:    common.HexToAddress("0x7777777777777777777777777777777777777777"),
		DstOApp:    common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:    common.HexToAddress("0x9999999999999999999999999999999999999999"),
		ReceiveLib: common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SourceWorkers: chain.WorkerContracts{
			OpenExecutor: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			OpenDVN:      common.HexToAddress("0x3333333333333333333333333333333333333333"),
		},
		DVNMode:        "shadow",
		Enabled:        true,
		MaxMessageSize: 10000,
	}
}

func queryHasTopic(query ethereum.FilterQuery, topic common.Hash) bool {
	for _, group := range query.Topics {
		if slices.Contains(group, topic) {
			return true
		}
	}
	return false
}

func queriesHaveAddress(queries []ethereum.FilterQuery, address common.Address) bool {
	for _, query := range queries {
		if slices.Contains(query.Addresses, address) {
			return true
		}
	}
	return false
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func cursorKey(chainEID uint32, stream string) string {
	return stream + ":" + new(big.Int).SetUint64(uint64(chainEID)).String()
}
