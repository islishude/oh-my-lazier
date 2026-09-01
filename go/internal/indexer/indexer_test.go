package indexer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"reflect"
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
	"github.com/islishude/oh-my-lazier/go/internal/rpcquorum"
	"github.com/jackc/pgx/v5"
)

func newExecutorSourceIndexer(configuredChain chain.Chain, pathways []chain.Pathway, store Store, client LogClient, logger *slog.Logger) *Indexer {
	return NewWithClient(configuredChain, pathways, ExecutorSourceStream, store, client, logger)
}

func (i *Indexer) withTestStream(stream Stream) *Indexer {
	i.stream = stream
	return i
}

func TestStreamsForRoles(t *testing.T) {
	tests := []struct {
		name     string
		executor bool
		dvn      bool
		want     []Stream
	}{
		{name: "none"},
		{name: "executor", executor: true, want: []Stream{ExecutorSourceStream, ExecutorDestinationStream}},
		{name: "dvn", dvn: true, want: []Stream{DVNSourceStream, DVNDestinationStream}},
		{name: "both", executor: true, dvn: true, want: []Stream{ExecutorSourceStream, ExecutorDestinationStream, DVNSourceStream, DVNDestinationStream}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StreamsForRoles(test.executor, test.dvn); !slices.Equal(got, test.want) {
				t.Fatalf("StreamsForRoles() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestIndexerProcessOnceBackfillsSourceExecutorAssignment(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	sourceLogs := testExecutorSourceLogs(t, executor, sendLib, big.NewInt(42))
	logger, logs := captureLogger(slog.LevelInfo)
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		safe:       188,
		safeSet:    true,
		sourceLogs: sourceLogs,
	}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", executor)
	configuredChain.Confirmations = 99
	indexer := newExecutorSourceIndexer(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	)
	indexer.pollInterval = time.Millisecond

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.FromBlock != 0 || result.ToBlock != 188 {
		t.Fatalf("source window = %d..%d, want 0..188", result.FromBlock, result.ToBlock)
	}
	if result.ObservedHeadBlock != 200 || result.SafeToBlock != 188 {
		t.Fatalf("head/safe = %d/%d, want 200/188", result.ObservedHeadBlock, result.SafeToBlock)
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
	if len(client.queries) != 1 {
		t.Fatalf("queries = %d, want one stream-specific query", len(client.queries))
	}
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 188 {
		t.Fatalf("source cursor = %d, want 188", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
	for _, stream := range []Stream{ExecutorDestinationStream, DVNSourceStream, DVNDestinationStream} {
		if _, ok := store.cursors[cursorKey(40161, stream)]; ok {
			t.Fatalf("unconfigured stream %s cursor advanced", stream)
		}
	}
	assertLogContains(t, logs.String(),
		`msg="indexed executor source assignment"`,
		`guid=0x`,
		`src_eid=40161`,
		`dst_eid=40449`,
		`tx_hash=0x`,
		`block_number=123`,
		`log_index=2`,
		`status=ASSIGNED`,
	)
}

func TestIndexerProcessOnceMarksUnsupportedExecutorOptionsManualReview(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	sourceLogs := []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(42), unsupportedExecutorOptions(), 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(42), 1),
		testPacketSentLog(t, txHash, sendLib, 2),
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := newExecutorSourceIndexer(
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
	logger, logs := captureLogger(slog.LevelDebug)
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testExecutorSourceLogs(t, otherExecutor, sendLib, big.NewInt(42)),
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", configuredExecutor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(ExecutorSourceStream)

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
	if len(store.sourceSkips) != 1 {
		t.Fatalf("source skips = %d, want 1", len(store.sourceSkips))
	}
	assertLogContains(t, logs.String(),
		`level=DEBUG`,
		`msg="skipped executor source assignment"`,
		`reason=unexpected_worker`,
		`worker=0x2323232323232323232323232323232323232323`,
		`expected_worker=0x2222222222222222222222222222222222222222`,
	)
}

func TestIndexerProcessOnceRecordsExternalExecutorWithoutAssignmentLog(t *testing.T) {
	configuredExecutor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	externalExecutor := common.HexToAddress("0x2323232323232323232323232323232323232323")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	packet, err := PacketRecordFromSentLog(testPacketSentLog(t, txHash, sendLib, 2))
	if err != nil {
		t.Fatalf("PacketRecordFromSentLog() error = %v", err)
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head: 200,
		sourceLogs: []gethtypes.Log{
			testExecutorFeePaidLog(t, txHash, sendLib, externalExecutor, big.NewInt(42), 1),
			testPacketSentLog(t, txHash, sendLib, 2),
		},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.SrcEID, "ethereum-sepolia", configuredExecutor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	if _, err := indexer.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	skip, ok := store.sourceSkips[sourceSkipLookupKey(sourceRoleExecutor, packet.SrcEID, packet.DstEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64())]
	if !ok {
		t.Fatal("external executor source skip was not recorded")
	}
	if skip.Worker != externalExecutor || skip.Reason != "unexpected_worker" {
		t.Fatalf("source skip worker/reason = %s/%q, want %s/unexpected_worker", skip.Worker, skip.Reason, externalExecutor)
	}
	if queriesHaveAddress(client.queries, externalExecutor) {
		t.Fatalf("source query unexpectedly included external executor %s", externalExecutor)
	}
}

func TestIndexerProcessOnceIgnoresUnrelatedInvalidPacketRoute(t *testing.T) {
	pathway := testIndexerPathway()
	tests := []struct {
		name   string
		stream Stream
		feeLog func(*testing.T, common.Hash) gethtypes.Log
	}{
		{
			name:   "executor source",
			stream: ExecutorSourceStream,
			feeLog: func(t *testing.T, txHash common.Hash) gethtypes.Log {
				return testExecutorFeePaidLog(t, txHash, pathway.SendLib, pathway.SourceWorkers.OpenExecutor, big.NewInt(1), 0)
			},
		},
		{
			name:   "dvn source",
			stream: DVNSourceStream,
			feeLog: func(t *testing.T, txHash common.Hash) gethtypes.Log {
				return testDVNFeePaidLog(t, txHash, pathway.SendLib, pathway.SourceWorkers.OpenDVN, big.NewInt(1), 0)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			txHash := common.HexToHash("0xabababababababababababababababababababababababababababababababab")
			encoded := testEncodedPacketWithNonceAndGUID(9, common.HexToHash("0xcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd"))
			copy(encoded[13:45], addressToBytes32(common.HexToAddress("0x1212121212121212121212121212121212121212")))
			clear(encoded[49:81])
			store := newFakeIndexerStore()
			client := &fakeLogClient{
				head: 200,
				sourceLogs: []gethtypes.Log{
					test.feeLog(t, txHash),
					testPacketSentLogWithPacket(t, txHash, pathway.SendLib, encoded, 1),
				},
			}
			indexer := newExecutorSourceIndexer(
				testIndexerChain(pathway.SrcEID, "ethereum-sepolia", pathway.SourceWorkers.OpenExecutor),
				[]chain.Pathway{pathway},
				store,
				client,
				discardLogger(),
			).withTestStream(test.stream)

			if _, err := indexer.ProcessOnce(context.Background()); err != nil {
				t.Fatalf("ProcessOnce() error = %v", err)
			}
			if len(store.packets) != 0 || len(store.jobs) != 0 || len(store.dvnJobs) != 0 || len(store.sourceSkips) != 0 {
				t.Fatalf("stored packets/executor jobs/dvn jobs/skips = %d/%d/%d/%d, want 0/0/0/0", len(store.packets), len(store.jobs), len(store.dvnJobs), len(store.sourceSkips))
			}
			if got := store.cursors[cursorKey(pathway.SrcEID, test.stream)]; got != 200 {
				t.Fatalf("%s cursor = %d, want 200", test.stream, got)
			}
		})
	}
}

func TestIndexerProcessOnceRecordsSendLibraryMismatchForBothRoles(t *testing.T) {
	pathway := testIndexerPathway()
	oldSendLib := common.HexToAddress("0x9898989898989898989898989898989898989898")
	txHash := common.HexToHash("0xacacacacacacacacacacacacacacacacacacacacacacacacacacacacacacac")
	packet, err := PacketRecordFromSentLog(testPacketSentLog(t, txHash, oldSendLib, 0))
	if err != nil {
		t.Fatalf("PacketRecordFromSentLog() error = %v", err)
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200, sourceLogs: []gethtypes.Log{testPacketSentLog(t, txHash, oldSendLib, 0)}}
	for _, stream := range []Stream{ExecutorSourceStream, DVNSourceStream} {
		indexer := newExecutorSourceIndexer(
			testIndexerChain(packet.SrcEID, "ethereum-sepolia", pathway.SourceWorkers.OpenExecutor),
			[]chain.Pathway{pathway},
			store,
			client,
			discardLogger(),
		).withTestStream(stream)
		if _, err := indexer.ProcessOnce(context.Background()); err != nil {
			t.Fatalf("ProcessOnce(%s) error = %v", stream, err)
		}
	}
	for _, role := range []string{sourceRoleExecutor, sourceRoleDVN} {
		skip, ok := store.sourceSkips[sourceSkipLookupKey(role, packet.SrcEID, packet.DstEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64())]
		if !ok || skip.Reason != "unexpected_send_library" {
			t.Fatalf("%s source skip = %+v, present %t, want unexpected_send_library", role, skip, ok)
		}
	}
}

func TestIndexerProcessOnceRecordsExternalDVNWithoutAssignmentLog(t *testing.T) {
	pathway := testIndexerPathway()
	externalDVN := common.HexToAddress("0x3434343434343434343434343434343434343434")
	txHash := common.HexToHash("0xadadadadadadadadadadadadadadadadadadadadadadadadadadadadadadad")
	packet, err := PacketRecordFromSentLog(testPacketSentLog(t, txHash, pathway.SendLib, 1))
	if err != nil {
		t.Fatalf("PacketRecordFromSentLog() error = %v", err)
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head: 200,
		sourceLogs: []gethtypes.Log{
			testDVNFeePaidLog(t, txHash, pathway.SendLib, externalDVN, big.NewInt(42), 0),
			testPacketSentLog(t, txHash, pathway.SendLib, 1),
		},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.SrcEID, "ethereum-sepolia", pathway.SourceWorkers.OpenExecutor),
		[]chain.Pathway{pathway},
		store,
		client,
		discardLogger(),
	).withTestStream(DVNSourceStream)

	if _, err := indexer.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	skip, ok := store.sourceSkips[sourceSkipLookupKey(sourceRoleDVN, packet.SrcEID, packet.DstEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64())]
	if !ok || skip.Worker != externalDVN || skip.Reason != "unexpected_worker" {
		t.Fatalf("dvn source skip = %+v, present %t, want external unexpected worker", skip, ok)
	}
}

func TestIndexerProcessOnceRejectsMissingConfiguredDVNAssignmentLog(t *testing.T) {
	pathway := testIndexerPathway()
	txHash := common.HexToHash("0xaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeaeae")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head: 200,
		sourceLogs: []gethtypes.Log{
			testDVNFeePaidLog(t, txHash, pathway.SendLib, pathway.SourceWorkers.OpenDVN, big.NewInt(42), 0),
			testPacketSentLog(t, txHash, pathway.SendLib, 1),
		},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(pathway.SrcEID, "ethereum-sepolia", pathway.SourceWorkers.OpenExecutor),
		[]chain.Pathway{pathway},
		store,
		client,
		discardLogger(),
	).withTestStream(DVNSourceStream)

	_, err := indexer.ProcessOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "assignment log is missing") {
		t.Fatalf("ProcessOnce() error = %v, want missing configured dvn assignment", err)
	}
	if _, ok := store.cursors[cursorKey(pathway.SrcEID, DVNSourceStream)]; ok {
		t.Fatal("dvn source cursor advanced after missing configured assignment")
	}
}

func TestIndexerProcessOnceRejectsMissingConfiguredExecutorAssignmentLog(t *testing.T) {
	configuredExecutor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head: 200,
		sourceLogs: []gethtypes.Log{
			testExecutorFeePaidLog(t, txHash, sendLib, configuredExecutor, big.NewInt(42), 1),
			testPacketSentLog(t, txHash, sendLib, 2),
		},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", configuredExecutor),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	_, err := indexer.ProcessOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "assignment log is missing") {
		t.Fatalf("ProcessOnce() error = %v, want missing configured executor assignment", err)
	}
	if _, ok := store.cursors[cursorKey(40161, ExecutorSourceStream)]; ok {
		t.Fatal("executor source cursor advanced after missing configured assignment")
	}
}

func TestIndexerProcessOnceBackfillsDestinationEvents(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Status = string(packets.ExecutorAssigned)
	logger, logs := captureLogger(slog.LevelInfo)
	store := newFakeIndexerStore()
	store.packets[packet.GUID] = packet
	store.jobs[packet.GUID] = db.ExecutorJobRecord{
		GUID:   packet.GUID,
		Status: string(packets.ExecutorAssigned),
	}
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(ExecutorDestinationStream)

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
	assertLogContains(t, logs.String(),
		`msg="indexed executor destination event"`,
		`event=PacketVerified`,
		`guid=0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
		`src_eid=40161`,
		`dst_eid=40449`,
		`to_status=COMMITTED`,
	)
}

func TestIndexerProcessOnceDefersDestinationCursorUntilSourcePacketAppears(t *testing.T) {
	packet := testDestinationPacketRecord()
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorDestinationStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 0 {
		t.Fatalf("DestinationLogs = %d, want 0", result.DestinationLogs)
	}
	if !result.Pending || result.HasMore {
		t.Fatalf("pending/hasMore = %t/%t, want true/false", result.Pending, result.HasMore)
	}
	if _, ok := store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)]; ok {
		t.Fatalf("destination cursor advanced to %d, want missing", store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)])
	}

	store.packets[packet.GUID] = packet
	store.jobs[packet.GUID] = db.ExecutorJobRecord{GUID: packet.GUID, Status: string(packets.ExecutorAssigned)}
	result, err = indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("second ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 1 {
		t.Fatalf("second DestinationLogs = %d, want 1", result.DestinationLogs)
	}
	if store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)] != 200 {
		t.Fatalf("destination cursor = %d, want 200", store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)])
	}
}

func TestIndexerProcessOnceAdvancesDestinationCursorForSkippedSourcePacket(t *testing.T) {
	packet := testDestinationPacketRecord()
	store := newFakeIndexerStore()
	store.sourceSkips[sourceSkipLookupKey(sourceRoleExecutor, packet.SrcEID, packet.DstEID, packet.Sender, packet.Receiver, packet.Nonce.Uint64())] = db.SourcePacketSkip{
		Role:     sourceRoleExecutor,
		SrcEID:   packet.SrcEID,
		DstEID:   packet.DstEID,
		Nonce:    packet.Nonce.Uint64(),
		Sender:   packet.Sender,
		Receiver: packet.Receiver,
		GUID:     packet.GUID,
		Reason:   "unexpected_worker",
	}
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorDestinationStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 0 {
		t.Fatalf("DestinationLogs = %d, want 0", result.DestinationLogs)
	}
	if store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)] != 200 {
		t.Fatalf("destination cursor = %d, want 200", store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)])
	}
}

func TestIndexerProcessOnceAdvancesDestinationCursorForExternalMissingPacket(t *testing.T) {
	packet := testDestinationPacketRecord()
	packet.Sender = common.HexToAddress("0x1212121212121212121212121212121212121212")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorDestinationStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.DestinationLogs != 0 {
		t.Fatalf("DestinationLogs = %d, want 0", result.DestinationLogs)
	}
	if store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)] != 200 {
		t.Fatalf("destination cursor = %d, want 200", store.cursors[cursorKey(packet.DstEID, ExecutorDestinationStream)])
	}
}

func TestIndexerProcessOnceBackfillsDVNVerification(t *testing.T) {
	packet := testDestinationPacketRecord()
	logger, logs := captureLogger(slog.LevelInfo)
	store := newFakeIndexerStore()
	store.packets[packet.GUID] = packet
	store.dvnJobs[packet.GUID] = db.DVNJobRecord{
		GUID:                  packet.GUID,
		ConfirmationsRequired: 12,
		Status:                string(packets.DVNVerifyTxEnqueued),
	}
	client := &fakeLogClient{
		head:            200,
		destinationLogs: []gethtypes.Log{testPayloadVerifiedLog(t, packet, common.HexToAddress("0x6666666666666666666666666666666666666666"))},
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(DVNDestinationStream)

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
	assertLogContains(t, logs.String(),
		`msg="indexed dvn destination event"`,
		`event=PayloadVerified`,
		`guid=0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
		`to_status=VERIFIED`,
	)
}

func TestIndexerProcessOnceBackfillsDVNAssignment(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testDVNSourceLogs(t, dvn, sendLib, big.NewInt(42)),
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(DVNSourceStream)

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
	indexer := newExecutorSourceIndexer(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

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

func TestIndexerProcessOnceUsesSafeGenesisBlock(t *testing.T) {
	client := &fakeLogClient{head: 11, safe: 0, safeSet: true}
	store := newFakeIndexerStore()
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.ObservedHeadBlock != 11 || result.SafeToBlock != 0 {
		t.Fatalf("head/safe = %d/%d, want 11/0", result.ObservedHeadBlock, result.SafeToBlock)
	}
	if result.FromBlock != 0 || result.ToBlock != 0 {
		t.Fatalf("source window = %d..%d, want genesis only", result.FromBlock, result.ToBlock)
	}
	if got := store.cursors[cursorKey(40161, ExecutorSourceStream)]; got != 0 {
		t.Fatalf("source cursor = %d, want 0", got)
	}
}

func TestIndexerProcessOnceSafeFailureDoesNotQueryOrAdvance(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 11, safeErr: errors.New("safe unavailable")}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	result, err := indexer.ProcessOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "safe unavailable") {
		t.Fatalf("ProcessOnce() error = %v, want safe failure", err)
	}
	if result.ObservedHeadBlock != 0 || result.SafeToBlock != 0 {
		t.Fatalf("result head/safe = %d/%d, want 0/0 after safe failure", result.ObservedHeadBlock, result.SafeToBlock)
	}
	if len(client.queries) != 0 || len(store.cursors) != 0 {
		t.Fatalf("queries/cursors = %d/%d, want 0/0", len(client.queries), len(store.cursors))
	}
}

func TestIndexerProcessOnceRejectsSafeAboveHead(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 11, safe: 12, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)

	if _, err := indexer.ProcessOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "above quorum head") {
		t.Fatalf("ProcessOnce() error = %v, want safe-above-head error", err)
	}
	if len(client.queries) != 0 || len(store.cursors) != 0 {
		t.Fatalf("queries/cursors = %d/%d, want 0/0", len(client.queries), len(store.cursors))
	}
}

func TestIndexerProcessOnceRefreshesHeadAfterSafeAdvances(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 11, safe: 12, safeSet: true}
	client.onBlock = func() { client.head = 12 }
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.ObservedHeadBlock != 12 || result.SafeToBlock != 12 {
		t.Fatalf("head/safe = %d/%d, want 12/12", result.ObservedHeadBlock, result.SafeToBlock)
	}
	if got := store.cursors[cursorKey(40161, ExecutorSourceStream)]; got != 12 {
		t.Fatalf("source cursor = %d, want 12", got)
	}
}

func TestIndexerProcessOnceDoesNotRewindCursorAheadOfSafe(t *testing.T) {
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 30, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	if _, err := indexer.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if got := store.cursors[cursorKey(40161, ExecutorSourceStream)]; got != 40 {
		t.Fatalf("source cursor = %d, want preserved 40", got)
	}
	if len(client.queries) != 0 {
		t.Fatalf("queries = %d, want none while cursor is ahead of safe", len(client.queries))
	}
}

func TestIndexerProcessOnceUsesPersistedCursor(t *testing.T) {
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.IndexerBackfillBlockRange = 10
	indexer := newExecutorSourceIndexer(
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
	if result.FromBlock != 41 || result.ToBlock != 50 {
		t.Fatalf("source window = %d..%d, want 41..50", result.FromBlock, result.ToBlock)
	}
	if !result.HasMore {
		t.Fatal("ProcessOnce() HasMore = false, want configured backfill window to continue immediately")
	}
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 50 {
		t.Fatalf("source cursor = %d, want 50", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
}

func TestIndexerProcessOnceSplitsSourceQueriesByConfiguredRange(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 37, safe: 25, safeSet: true}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.IndexerQueryBlockRange = 10
	indexer := newExecutorSourceIndexer(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.FromBlock != 0 || result.ToBlock != 25 {
		t.Fatalf("source window = %d..%d, want 0..25", result.FromBlock, result.ToBlock)
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
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 25 {
		t.Fatalf("source cursor = %d, want 25", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
}

func TestIndexerProcessOnceSplitsDestinationQueriesByConfiguredRange(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 37, safe: 25, safeSet: true}
	configuredChain := testIndexerChain(40449, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555"))
	configuredChain.IndexerQueryBlockRange = 10
	indexer := newExecutorSourceIndexer(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorDestinationStream)

	result, err := indexer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatalf("ProcessOnce() error = %v", err)
	}
	if result.FromBlock != 0 || result.ToBlock != 25 {
		t.Fatalf("destination window = %d..%d, want 0..25", result.FromBlock, result.ToBlock)
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
	if store.cursors[cursorKey(40449, ExecutorDestinationStream)] != 25 {
		t.Fatalf("destination cursor = %d, want 25", store.cursors[cursorKey(40449, ExecutorDestinationStream)])
	}
}

func TestIndexerProcessOnceProcessesSourceLogsAsContiguousTransactions(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	firstTx := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secondTx := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	sourceLogs := []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, firstTx, executor, sendLib, big.NewInt(42), validExecutorOptions(), 0),
		testExecutorFeePaidLog(t, firstTx, sendLib, executor, big.NewInt(42), 1),
		testPacketSentLog(t, firstTx, sendLib, 2),
		testExecutorJobAssignedLogWithOptions(t, secondTx, executor, sendLib, big.NewInt(42), validExecutorOptions(), 3),
		testExecutorFeePaidLog(t, secondTx, sendLib, executor, big.NewInt(42), 4),
		testPacketSentLog(t, secondTx, sendLib, 5),
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := newExecutorSourceIndexer(
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

func TestIndexerProcessOnceProcessesMultipleSourceSendsInOneTransaction(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	txHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secondGUID := common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	sourceLogs := []gethtypes.Log{
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(41), validExecutorOptions(), 0),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(41), 1),
		testPacketSentLog(t, txHash, sendLib, 2),
		testExecutorJobAssignedLogWithOptions(t, txHash, executor, sendLib, big.NewInt(42), validExecutorOptions(), 3),
		testExecutorFeePaidLog(t, txHash, sendLib, executor, big.NewInt(42), 4),
		testPacketSentLogWithPacket(t, txHash, sendLib, testEncodedPacketWithNonceAndGUID(8, secondGUID), 5),
	}
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: sourceLogs,
	}
	indexer := newExecutorSourceIndexer(
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
	if len(store.packets) != 2 || len(store.jobs) != 2 {
		t.Fatalf("stored packets/jobs = %d/%d, want 2/2", len(store.packets), len(store.jobs))
	}
	if _, ok := store.packets[secondGUID]; !ok {
		t.Fatalf("second packet %s was not stored", secondGUID)
	}
}

func TestIndexerProcessOnceUsesConfiguredStartBlockWhenCursorMissing(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.StartBlockNumber = 150
	indexer := newExecutorSourceIndexer(
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
	if result.FromBlock != 150 || result.ToBlock != 200 {
		t.Fatalf("source window = %d..%d, want 150..200", result.FromBlock, result.ToBlock)
	}
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 200 {
		t.Fatalf("source cursor = %d, want 200", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
}

func TestIndexerProcessOnceExecutorOnlyStreamsDoNotWriteDVNJobs(t *testing.T) {
	dvn := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testDVNSourceLogs(t, dvn, sendLib, big.NewInt(42)),
	}
	for _, stream := range StreamsForRoles(true, false) {
		indexer := newExecutorSourceIndexer(
			testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
			[]chain.Pathway{testIndexerPathway()},
			store,
			client,
			discardLogger(),
		).withTestStream(stream)
		result, err := indexer.ProcessOnce(context.Background())
		if err != nil {
			t.Fatalf("ProcessOnce(%s) error = %v", stream, err)
		}
		if result.DVNTransactions != 0 {
			t.Fatalf("DVNTransactions(%s) = %d, want 0", stream, result.DVNTransactions)
		}
	}
	if len(store.dvnJobs) != 0 {
		t.Fatalf("dvn jobs = %d, want 0", len(store.dvnJobs))
	}
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 200 {
		t.Fatalf("executor source cursor = %d, want 200", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
	if store.cursors[cursorKey(40161, ExecutorDestinationStream)] != 200 {
		t.Fatalf("executor destination cursor = %d, want 200", store.cursors[cursorKey(40161, ExecutorDestinationStream)])
	}
	if _, ok := store.cursors[cursorKey(40161, DVNSourceStream)]; ok {
		t.Fatal("dvn source cursor advanced in executor-only mode")
	}
	if _, ok := store.cursors[cursorKey(40161, DVNDestinationStream)]; ok {
		t.Fatal("dvn destination cursor advanced in executor-only mode")
	}
}

func TestIndexerProcessOnceDVNOnlyStreamsDoNotWriteExecutorJobs(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	client := &fakeLogClient{
		head:       200,
		sourceLogs: testExecutorSourceLogs(t, executor, sendLib, big.NewInt(42)),
	}
	for _, stream := range StreamsForRoles(false, true) {
		indexer := newExecutorSourceIndexer(
			testIndexerChain(40161, "ethereum-sepolia", executor),
			[]chain.Pathway{testIndexerPathway()},
			store,
			client,
			discardLogger(),
		).withTestStream(stream)
		result, err := indexer.ProcessOnce(context.Background())
		if err != nil {
			t.Fatalf("ProcessOnce(%s) error = %v", stream, err)
		}
		if result.SourceTransactions != 0 {
			t.Fatalf("SourceTransactions(%s) = %d, want 0", stream, result.SourceTransactions)
		}
	}
	if len(store.jobs) != 0 {
		t.Fatalf("executor jobs = %d, want 0", len(store.jobs))
	}
	if store.cursors[cursorKey(40161, DVNSourceStream)] != 200 {
		t.Fatalf("dvn source cursor = %d, want 200", store.cursors[cursorKey(40161, DVNSourceStream)])
	}
	if store.cursors[cursorKey(40161, DVNDestinationStream)] != 200 {
		t.Fatalf("dvn destination cursor = %d, want 200", store.cursors[cursorKey(40161, DVNDestinationStream)])
	}
	if _, ok := store.cursors[cursorKey(40161, ExecutorSourceStream)]; ok {
		t.Fatal("executor source cursor advanced in dvn-only mode")
	}
	if _, ok := store.cursors[cursorKey(40161, ExecutorDestinationStream)]; ok {
		t.Fatal("executor destination cursor advanced in dvn-only mode")
	}
}

func TestIndexerProcessOnceSkipsUntilStartBlockIsSafe(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.StartBlockNumber = 250
	indexer := newExecutorSourceIndexer(
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
	if result.FromBlock != 0 || result.ToBlock != 0 || result.Advanced {
		t.Fatalf("result windows = %+v, want no indexed windows", result)
	}
	if len(client.queries) != 0 {
		t.Fatalf("queries = %d, want none before configured start block is safe", len(client.queries))
	}
	if len(store.cursors) != 0 {
		t.Fatalf("cursors = %d, want none before configured start block is safe", len(store.cursors))
	}
}

func TestIndexerProcessOnceFailureDoesNotAdvanceCursor(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200, filterErr: errors.New("filter unavailable")}
	indexer := newExecutorSourceIndexer(
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

func TestIndexerPollOnceLogsSyncProgress(t *testing.T) {
	logger, logs := captureLogger(slog.LevelInfo)
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(ExecutorSourceStream)
	indexer.backfillRange = 10
	indexer.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	output := logs.String()
	assertLogContains(t, output,
		`msg="indexer progress"`,
		`chain=ethereum-sepolia`,
		`eid=40161`,
		`stream=executor_source`,
		`advanced=true`,
		`from_block=41`,
		`to_block=50`,
		`safe_to_block=53`,
		`lag_blocks=3`,
		`duration=`,
	)
	if strings.Contains(output, `msg="indexer stream advanced"`) {
		t.Fatalf("stream progress logged at info level:\n%s", output)
	}
	if strings.Contains(output, `msg="indexer poll completed"`) {
		t.Fatalf("poll summary logged at info level:\n%s", output)
	}
}

func TestIndexerChunkFailureResumesFromLastCheckpoint(t *testing.T) {
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sendLib := common.HexToAddress("0x9999999999999999999999999999999999999999")
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true, sourceLogs: testExecutorSourceLogs(t, executor, sendLib, big.NewInt(42))}
	calls := 0
	client.onFilter = func(ethereum.FilterQuery) error {
		calls++
		if calls == 2 {
			return errors.New("second chunk conflicted")
		}
		return nil
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream)
	indexer.backfillRange = 10
	indexer.queryBlockRange = 5

	// The 41..50 window splits into chunks 41-45 and 46-50; the second chunk
	// fails after the first already wrote durable rows.
	if _, err := indexer.ProcessOnce(context.Background()); err == nil {
		t.Fatal("ProcessOnce() error = nil, want second-chunk failure")
	}
	if cursor := store.cursors[cursorKey(40161, ExecutorSourceStream)]; cursor != 45 {
		t.Fatalf("cursor after partial window = %d, want completed chunk checkpoint 45", cursor)
	}
	if len(store.packets) != 1 || len(store.jobs) != 1 {
		t.Fatalf("partial window wrote %d packets / %d jobs, want the first chunk's 1/1", len(store.packets), len(store.jobs))
	}

	// The retry resumes at 46 instead of replaying the completed 41..45 chunk.
	client.onFilter = nil
	if _, err := indexer.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce(replay) error = %v", err)
	}
	if cursor := store.cursors[cursorKey(40161, ExecutorSourceStream)]; cursor != 53 {
		t.Fatalf("cursor after retry = %d, want safe block 53", cursor)
	}
	wantQueries := [][2]uint64{{41, 45}, {46, 50}, {51, 53}}
	var gotQueries [][2]uint64
	for _, query := range client.queries {
		gotQueries = append(gotQueries, [2]uint64{query.FromBlock.Uint64(), query.ToBlock.Uint64()})
	}
	if !slices.Equal(gotQueries, wantQueries) {
		t.Fatalf("successful query checkpoints = %v, want %v", gotQueries, wantQueries)
	}
	if len(store.packets) != 1 || len(store.jobs) != 1 {
		t.Fatalf("replay duplicated rows: %d packets / %d jobs, want 1/1", len(store.packets), len(store.jobs))
	}
}

func TestIndexerPollOnceReportsProviderStatuses(t *testing.T) {
	providers := []rpcquorum.Provider{
		{ID: "provider-0", Status: rpcquorum.ProviderHealthy},
		{ID: "provider-1", Status: rpcquorum.ProviderConflict, LogConflict: true},
	}
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &quorumStatusLogClient{
		fakeLogClient: &fakeLogClient{head: 65, safe: 53, safeSet: true},
		providers:     providers,
	}
	recorder := &providerStatusRecorder{}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorSourceStream).WithMetrics(recorder)
	indexer.backfillRange = 10

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	if len(recorder.reports) != 1 {
		t.Fatalf("provider reports = %d, want 1", len(recorder.reports))
	}
	report := recorder.reports[0]
	if report.chainEID != 40161 || report.chainName != "ethereum-sepolia" {
		t.Fatalf("report identity = %d/%s, want 40161/ethereum-sepolia", report.chainEID, report.chainName)
	}
	if !reflect.DeepEqual(report.providers, providers) {
		t.Fatalf("report providers = %+v, want %+v", report.providers, providers)
	}

	client.filterErr = errors.New("log quorum conflict")
	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v, want swallowed poll failure", err)
	}
	if len(recorder.reports) != 2 {
		t.Fatalf("provider reports after failed poll = %d, want 2 (statuses must surface even when the poll fails)", len(recorder.reports))
	}
}

func TestIndexerPollOnceReportsPerStreamProgressInfo(t *testing.T) {
	logger, logs := captureLogger(slog.LevelInfo)
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	)
	indexer.backfillRange = 10
	indexer.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	output := logs.String()
	if count := strings.Count(output, `msg="indexer progress"`); count != 1 {
		t.Fatalf("indexer progress logs = %d, want 1:\n%s", count, output)
	}
	assertLogContains(t, output,
		`stream=executor_source`,
		`advanced=true`,
		`from_block=41`,
		`to_block=50`,
		`lag_blocks=3`,
	)
	if strings.Contains(output, `msg="indexer stream advanced"`) ||
		strings.Contains(output, `msg="indexer poll completed"`) {
		t.Fatalf("per-stream or poll summary logged at info level:\n%s", output)
	}
}

func TestIndexerPollOnceThrottlesSyncProgressInfo(t *testing.T) {
	logger, logs := captureLogger(slog.LevelDebug)
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(ExecutorSourceStream).WithProgressLogInterval(time.Minute)
	indexer.backfillRange = 10
	now := time.Unix(1_700_000_000, 0)
	indexer.now = func() time.Time { return now }

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("first pollOnce() error = %v", err)
	}
	logs.Reset()
	client.head = 75
	client.safe = 63
	now = now.Add(10 * time.Second)

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("second pollOnce() error = %v", err)
	}
	output := logs.String()
	assertLogContains(t, output,
		`level=DEBUG msg="indexer stream advanced"`,
		`from_block=51`,
		`to_block=60`,
		`level=DEBUG msg="indexer poll completed"`,
		`advanced=true`,
	)
	if strings.Contains(output, `level=INFO msg="indexer progress"`) {
		t.Fatalf("throttled progress logged at info level:\n%s", output)
	}
}

func TestIndexerPollOnceDisablesPeriodicProgressInfo(t *testing.T) {
	logger, logs := captureLogger(slog.LevelDebug)
	store := newFakeIndexerStore()
	store.cursors[cursorKey(40161, ExecutorSourceStream)] = 40
	client := &fakeLogClient{head: 65, safe: 53, safeSet: true}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		logger,
	).withTestStream(ExecutorSourceStream).WithProgressLogInterval(0)
	indexer.backfillRange = 10

	if _, err := indexer.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce() error = %v", err)
	}
	output := logs.String()
	assertLogContains(t, output,
		`level=DEBUG msg="indexer stream advanced"`,
		`level=DEBUG msg="indexer poll completed"`,
	)
	if strings.Contains(output, `level=INFO msg="indexer progress"`) ||
		strings.Contains(output, `level=INFO msg="indexer stream advanced"`) ||
		strings.Contains(output, `level=INFO msg="indexer poll completed"`) {
		t.Fatalf("progress info logs emitted with interval 0:\n%s", output)
	}
}

func TestIndexerPollUntilPausedCatchesUpWithoutWaitingBetweenBackfillRanges(t *testing.T) {
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 35, safe: 25, safeSet: true}
	safeCalls := 0
	client.onSafe = func() {
		safeCalls++
		if safeCalls >= 2 {
			client.safe = 35
		}
	}
	indexer := newExecutorSourceIndexer(
		testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	)
	indexer.backfillRange = 10
	indexer.queryBlockRange = 5
	indexer.pollInterval = time.Hour

	if err := indexer.pollUntilPaused(context.Background()); err != nil {
		t.Fatalf("pollUntilPaused() error = %v", err)
	}
	if cursor := store.cursors[cursorKey(40161, ExecutorSourceStream)]; cursor != 35 {
		t.Fatalf("source cursor = %d, want moving safe block 35", cursor)
	}
	if safeCalls != 4 {
		t.Fatalf("safe snapshot calls = %d, want four immediate backfill passes", safeCalls)
	}
	wantQueries := [][2]uint64{{0, 4}, {5, 9}, {10, 14}, {15, 19}, {20, 24}, {25, 29}, {30, 34}, {35, 35}}
	var gotQueries [][2]uint64
	for _, query := range client.queries {
		gotQueries = append(gotQueries, [2]uint64{query.FromBlock.Uint64(), query.ToBlock.Uint64()})
	}
	if !slices.Equal(gotQueries, wantQueries) {
		t.Fatalf("catch-up queries = %v, want %v", gotQueries, wantQueries)
	}
}

func TestIndexerPollUntilPausedStopsAfterPendingDestinationChunk(t *testing.T) {
	packet := testDestinationPacketRecord()
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 20, destinationLogs: []gethtypes.Log{testPacketVerifiedLog(t, packet)}}
	safeCalls := 0
	client.onSafe = func() { safeCalls++ }
	indexer := newExecutorSourceIndexer(
		testIndexerChain(packet.DstEID, "hoodi", common.HexToAddress("0x5555555555555555555555555555555555555555")),
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).withTestStream(ExecutorDestinationStream)
	indexer.backfillRange = 10
	indexer.queryBlockRange = 5

	if err := indexer.pollUntilPaused(context.Background()); err != nil {
		t.Fatalf("pollUntilPaused() error = %v", err)
	}
	if safeCalls != 1 {
		t.Fatalf("safe snapshot calls = %d, want one before pending stopped catch-up", safeCalls)
	}
	if len(client.queries) != 1 {
		t.Fatalf("destination queries = %d, want one pending chunk", len(client.queries))
	}
}

func TestIndexerRunPollsImmediatelyAndOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var blockCalls atomic.Int32
	store := newFakeIndexerStore()
	metricRecorder := &fakeIndexerMetrics{}
	client := &fakeLogClient{
		head: 200,
		onBlock: func() {
			if metricRecorder.registerCalls == 0 {
				metricRecorder.pollStartedBeforeRegister = true
			}
			if blockCalls.Add(1) == 2 {
				cancel()
			}
		},
	}
	configuredChain := testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222"))
	configuredChain.IndexerPollInterval = time.Millisecond
	indexer := newExecutorSourceIndexer(
		configuredChain,
		[]chain.Pathway{testIndexerPathway()},
		store,
		client,
		discardLogger(),
	).WithMetrics(metricRecorder)
	if indexer.pollInterval != time.Millisecond {
		t.Fatalf("poll interval = %s, want 1ms", indexer.pollInterval)
	}

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
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 200 {
		t.Fatalf("source cursor = %d, want 200", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
	if metricRecorder.registerCalls != 1 || metricRecorder.pollStartedBeforeRegister || metricRecorder.pollsBeforeRegister {
		t.Fatalf("metric registration state = %#v", metricRecorder)
	}
	if metricRecorder.chainEID != 40161 || metricRecorder.chainName != "ethereum-sepolia" || metricRecorder.stream != ExecutorSourceStream.String() || metricRecorder.pollInterval != time.Millisecond {
		t.Fatalf("registered indexer metrics = %#v", metricRecorder)
	}
	if metricRecorder.pollCalls < 2 {
		t.Fatalf("recorded polls = %d, want at least 2", metricRecorder.pollCalls)
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
	indexer := newExecutorSourceIndexer(
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
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 200 {
		t.Fatalf("source cursor = %d, want retry to advance to 200", store.cursors[cursorKey(40161, ExecutorSourceStream)])
	}
}

func TestIndexerRunContinuesPollingAfterSafeUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var safeCalls atomic.Int32
	store := newFakeIndexerStore()
	client := &fakeLogClient{head: 200, safe: 200, safeSet: true, safeErr: errors.New("safe unavailable")}
	client.onSafe = func() {
		switch safeCalls.Add(1) {
		case 2:
			client.safeErr = nil
		case 3:
			cancel()
		}
	}
	indexer := newExecutorSourceIndexer(
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
		t.Fatal("Run() did not continue polling after safe became available")
	}
	if got := safeCalls.Load(); got < 3 {
		t.Fatalf("SafeBlockNumber calls = %d, want at least 3", got)
	}
	if len(client.queries) == 0 {
		t.Fatal("queries = 0, want polling to resume after safe became available")
	}
	if store.cursors[cursorKey(40161, ExecutorSourceStream)] != 200 {
		t.Fatalf("source cursor = %d, want 200", store.cursors[cursorKey(40161, ExecutorSourceStream)])
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
				indexer := newExecutorSourceIndexer(
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
			name: "backfill range",
			indexer: func() *Indexer {
				indexer := newExecutorSourceIndexer(
					testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
					[]chain.Pathway{testIndexerPathway()},
					newFakeIndexerStore(),
					&fakeLogClient{head: 200},
					discardLogger(),
				)
				indexer.backfillRange = 0
				return indexer
			}(),
			wantError: "backfill range",
		},
		{
			name: "query range",
			indexer: func() *Indexer {
				indexer := newExecutorSourceIndexer(
					testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
					[]chain.Pathway{testIndexerPathway()},
					newFakeIndexerStore(),
					&fakeLogClient{head: 200},
					discardLogger(),
				)
				indexer.queryBlockRange = 0
				return indexer
			}(),
			wantError: "query block range",
		},
		{
			name: "stream",
			indexer: newExecutorSourceIndexer(
				testIndexerChain(40161, "ethereum-sepolia", common.HexToAddress("0x2222222222222222222222222222222222222222")),
				[]chain.Pathway{testIndexerPathway()},
				newFakeIndexerStore(),
				&fakeLogClient{head: 200},
				discardLogger(),
			).withTestStream(Stream("unknown")),
			wantError: "unsupported indexer stream",
		},
		{
			name: "store",
			indexer: newExecutorSourceIndexer(
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
			indexer: newExecutorSourceIndexer(
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
	safe            uint64
	safeSet         bool
	blockErr        error
	safeErr         error
	filterErr       error
	sourceLogs      []gethtypes.Log
	destinationLogs []gethtypes.Log
	queries         []ethereum.FilterQuery
	onBlock         func()
	onSafe          func()
	onFilter        func(query ethereum.FilterQuery) error
}

type fakeIndexerMetrics struct {
	chainEID                  uint32
	chainName                 string
	stream                    string
	pollInterval              time.Duration
	registerCalls             int
	pollCalls                 int
	pollStartedBeforeRegister bool
	pollsBeforeRegister       bool
}

func (m *fakeIndexerMetrics) RegisterIndexer(chainEID uint32, chainName, stream string, pollInterval time.Duration) {
	m.chainEID = chainEID
	m.chainName = chainName
	m.stream = stream
	m.pollInterval = pollInterval
	m.registerCalls++
}

func (m *fakeIndexerMetrics) RecordIndexerPoll(_ uint32, _, _ string, _ time.Duration, _ uint64, _ uint64, _ int, _ int, _ int, _ time.Duration, _ error) {
	if m.registerCalls == 0 {
		m.pollsBeforeRegister = true
	}
	m.pollCalls++
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

func (c *fakeLogClient) SafeLogSnapshot(context.Context) (rpcquorum.SafeLogSnapshot, error) {
	if c.onSafe != nil {
		c.onSafe()
	}
	if c.safeErr != nil {
		return nil, c.safeErr
	}
	number := c.head
	if c.safeSet {
		number = c.safe
	}
	return &fakeSafeLogSnapshot{client: c, number: number}, nil
}

type fakeSafeLogSnapshot struct {
	client *fakeLogClient
	number uint64
}

func (s *fakeSafeLogSnapshot) Number() uint64 { return s.number }

func (s *fakeSafeLogSnapshot) Hash() common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(s.number))
}

func (s *fakeSafeLogSnapshot) FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error) {
	return s.client.FilterLogs(ctx, query)
}

func (c *fakeLogClient) FilterLogs(_ context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error) {
	if c.filterErr != nil {
		return nil, c.filterErr
	}
	if c.onFilter != nil {
		if err := c.onFilter(query); err != nil {
			return nil, err
		}
	}
	c.queries = append(c.queries, query)
	if queryHasTopic(query, lzabi.PacketSentTopic()) {
		return append([]gethtypes.Log(nil), c.sourceLogs...), nil
	}
	if queryHasTopic(query, lzabi.PacketVerifiedTopic()) {
		return append([]gethtypes.Log(nil), c.destinationLogs...), nil
	}
	if queryHasTopic(query, lzabi.PayloadVerifiedTopic()) {
		return append([]gethtypes.Log(nil), c.destinationLogs...), nil
	}
	return nil, nil
}

type quorumStatusLogClient struct {
	*fakeLogClient
	providers []rpcquorum.Provider
}

func (c *quorumStatusLogClient) Providers() []rpcquorum.Provider {
	return c.providers
}

type providerReport struct {
	chainEID  uint32
	chainName string
	providers []rpcquorum.Provider
}

type providerStatusRecorder struct {
	reports []providerReport
}

func (r *providerStatusRecorder) RegisterIndexer(uint32, string, string, time.Duration) {}

func (r *providerStatusRecorder) RecordIndexerPoll(uint32, string, string, time.Duration, uint64, uint64, int, int, int, time.Duration, error) {
}

func (r *providerStatusRecorder) RecordRPCProviders(chainEID uint32, chainName string, providers []rpcquorum.Provider) {
	r.reports = append(r.reports, providerReport{chainEID: chainEID, chainName: chainName, providers: providers})
}

type fakeIndexerStore struct {
	packets         map[common.Hash]db.PacketRecord
	jobs            map[common.Hash]db.ExecutorJobRecord
	dvnJobs         map[common.Hash]db.DVNJobRecord
	cursors         map[string]uint64
	sourceSkips     map[string]db.SourcePacketSkip
	committedGUID   common.Hash
	dvnVerifiedGUID common.Hash
}

func newFakeIndexerStore() *fakeIndexerStore {
	return &fakeIndexerStore{
		packets:     make(map[common.Hash]db.PacketRecord),
		jobs:        make(map[common.Hash]db.ExecutorJobRecord),
		dvnJobs:     make(map[common.Hash]db.DVNJobRecord),
		cursors:     make(map[string]uint64),
		sourceSkips: make(map[string]db.SourcePacketSkip),
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

func (s *fakeIndexerStore) RecordSourcePacketSkip(_ context.Context, skip db.SourcePacketSkip) error {
	s.sourceSkips[sourceSkipLookupKey(skip.Role, skip.SrcEID, skip.DstEID, skip.Sender, skip.Receiver, skip.Nonce)] = skip
	return nil
}

func (s *fakeIndexerStore) UpsertExecutorAssignment(_ context.Context, packet db.PacketRecord, job db.ExecutorJobRecord) error {
	s.packets[packet.GUID] = packet
	s.jobs[job.GUID] = job
	return nil
}

func (s *fakeIndexerStore) UpsertDVNAssignment(_ context.Context, packet db.PacketRecord, job db.DVNJobRecord) error {
	if existing, ok := s.packets[packet.GUID]; ok {
		packet.Status = existing.Status
	}
	s.packets[packet.GUID] = packet
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

func (s *fakeIndexerStore) GetSourcePacketSkip(_ context.Context, role string, srcEID, dstEID uint32, sender, receiver common.Address, nonce uint64) (db.SourcePacketSkip, error) {
	skip, ok := s.sourceSkips[sourceSkipLookupKey(role, srcEID, dstEID, sender, receiver, nonce)]
	if !ok {
		return db.SourcePacketSkip{}, pgx.ErrNoRows
	}
	return skip, nil
}

func (s *fakeIndexerStore) MarkExecutorCommittedObserved(_ context.Context, guid, _ common.Hash, _ string) error {
	s.committedGUID = guid
	if job, ok := s.jobs[guid]; ok {
		job.Status = string(packets.ExecutorCommitted)
		s.jobs[guid] = job
	}
	return nil
}

func (s *fakeIndexerStore) MarkExecutorDeliveredObserved(_ context.Context, guid, _ common.Hash, _ string) error {
	if job, ok := s.jobs[guid]; ok {
		job.Status = string(packets.ExecutorDelivered)
		s.jobs[guid] = job
	}
	return nil
}

func (s *fakeIndexerStore) MarkExecutorReceiveFailedObserved(_ context.Context, guid, _ common.Hash, _, _ string) error {
	if job, ok := s.jobs[guid]; ok {
		job.Status = string(packets.ExecutorLzReceiveFailed)
		s.jobs[guid] = job
	}
	return nil
}

func (s *fakeIndexerStore) MarkDVNVerifiedObserved(_ context.Context, guid, _ common.Hash, _ string) error {
	s.dvnVerifiedGUID = guid
	if job, ok := s.dvnJobs[guid]; ok {
		job.Status = string(packets.DVNVerified)
		s.dvnJobs[guid] = job
	}
	return nil
}

func testIndexerChain(eid uint32, name string, executor common.Address) chain.Chain {
	return chain.Chain{
		EID:                       eid,
		Name:                      name,
		EndpointAddress:           common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Confirmations:             12,
		IndexerQueryBlockRange:    500,
		IndexerBackfillBlockRange: 10_000,
		IndexerPollInterval:       5 * time.Second,
		TxRoles: chain.TxRoles{
			Executor: chain.ExecutorTxRole{SignerID: executor.Hex()},
		},
	}
}

func testIndexerPathway() chain.Pathway {
	return chain.Pathway{
		SrcEID:                  40161,
		DstEID:                  40449,
		SrcOApp:                 common.HexToAddress("0x7777777777777777777777777777777777777777"),
		DstOApp:                 common.HexToAddress("0x8888888888888888888888888888888888888888"),
		SendLib:                 common.HexToAddress("0x9999999999999999999999999999999999999999"),
		ReceiveLib:              common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		SendULNConfirmations:    12,
		ReceiveULNConfirmations: 12,
		SourceWorkers: chain.WorkerContracts{
			OpenExecutor: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			OpenDVN:      common.HexToAddress("0x3333333333333333333333333333333333333333"),
		},
		DestinationWorkers: chain.DestinationWorkerContracts{
			OpenDVN: common.HexToAddress("0x6666666666666666666666666666666666666666"),
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

func captureLogger(level slog.Leveler) (*slog.Logger, *bytes.Buffer) {
	var logs bytes.Buffer
	return slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: level})), &logs
}

func assertLogContains(t *testing.T, output string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("logs missing %q in:\n%s", want, output)
		}
	}
}

func cursorKey[S ~string](chainEID uint32, stream S) string {
	return string(stream) + ":" + new(big.Int).SetUint64(uint64(chainEID)).String()
}
