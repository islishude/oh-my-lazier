package indexer

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/lzabi"
	"github.com/islishude/oh-my-lazier/go/internal/workerloop"
	"github.com/jackc/pgx/v5"
)

const (
	defaultPollInterval    = 5 * time.Second
	defaultBackfillRange   = uint64(500)
	defaultQueryBlockRange = uint64(500)
)

const (
	// ExecutorSourceStream tracks source-chain OpenExecutor assignments.
	ExecutorSourceStream = "executor_source"
	// ExecutorDestinationStream tracks destination-chain executor outcomes.
	ExecutorDestinationStream = "executor_destination"
	// DVNSourceStream tracks source-chain OpenDVN assignments.
	DVNSourceStream = "dvn_source"
	// DVNDestinationStream tracks destination-chain OpenDVN verification outcomes.
	DVNDestinationStream = "dvn_destination"

	executorSourceStream = ExecutorSourceStream
	executorDestStream   = ExecutorDestinationStream
	dvnSourceStream      = DVNSourceStream
	dvnDestStream        = DVNDestinationStream
)

const (
	sourceRoleExecutor = "executor"
	sourceRoleDVN      = "dvn"
)

// StreamSet selects the durable indexer streams this process advances.
type StreamSet struct {
	ExecutorSource      bool
	ExecutorDestination bool
	DVNSource           bool
	DVNDestination      bool
}

// StreamsForRoles returns the indexer streams required by enabled worker roles.
func StreamsForRoles(executorEnabled, dvnEnabled bool) StreamSet {
	return StreamSet{
		ExecutorSource:      executorEnabled,
		ExecutorDestination: executorEnabled,
		DVNSource:           dvnEnabled,
		DVNDestination:      dvnEnabled,
	}
}

// Empty reports whether no indexer stream is enabled.
func (s StreamSet) Empty() bool {
	return !s.ExecutorSource && !s.ExecutorDestination && !s.DVNSource && !s.DVNDestination
}

func allStreams() StreamSet {
	return StreamSet{
		ExecutorSource:      true,
		ExecutorDestination: true,
		DVNSource:           true,
		DVNDestination:      true,
	}
}

// Store persists indexed executor source records and destination outcomes.
type Store interface {
	DestinationStore
	GetIndexerCursor(ctx context.Context, chainEID uint32, stream string) (uint64, error)
	UpdateIndexerCursor(ctx context.Context, chainEID uint32, stream string, lastBlock uint64) error
	UpsertPacket(ctx context.Context, packet db.PacketRecord) error
	UpsertExecutorJob(ctx context.Context, job db.ExecutorJobRecord) error
	UpsertDVNJob(ctx context.Context, job db.DVNJobRecord) error
}

// LogClient reads chain heads and historical EVM logs.
type LogClient interface {
	BlockNumber(ctx context.Context) (uint64, error)
	FilterLogs(ctx context.Context, query ethereum.FilterQuery) ([]gethtypes.Log, error)
}

// MetricsRecorder records process-local indexer polling outcomes.
type MetricsRecorder interface {
	RecordIndexerPoll(chainEID uint32, chainName string, observedHeadBlock uint64, confirmedToBlock uint64, sourceTransactions int, dvnTransactions int, destinationLogs int, duration time.Duration, err error)
}

// Indexer watches one chain for LayerZero and worker contract events.
type Indexer struct {
	chain               chain.Chain
	sourcePathways      []chain.Pathway
	destinationPathways []chain.Pathway
	destinationEID      uint32
	store               Store
	client              LogClient
	pollInterval        time.Duration
	backfillRange       uint64
	queryBlockRange     uint64
	logger              *slog.Logger
	metrics             MetricsRecorder
	streams             StreamSet
}

// New creates an indexer for one configured chain.
func New(configuredChain chain.Chain, pathways []chain.Pathway, store Store, logger *slog.Logger) *Indexer {
	return NewWithClient(configuredChain, pathways, store, configuredChain.RPC, logger)
}

// NewWithClient creates an indexer with an explicit log client for tests.
func NewWithClient(configuredChain chain.Chain, pathways []chain.Pathway, store Store, client LogClient, logger *slog.Logger) *Indexer {
	sourcePathways := make([]chain.Pathway, 0)
	destinationPathways := make([]chain.Pathway, 0)
	for _, pathway := range pathways {
		if pathway.SrcEID == configuredChain.EID {
			sourcePathways = append(sourcePathways, pathway)
		}
		if pathway.DstEID == configuredChain.EID {
			destinationPathways = append(destinationPathways, pathway)
		}
	}
	queryBlockRange := configuredChain.IndexerQueryBlockRange
	if queryBlockRange == 0 {
		queryBlockRange = defaultQueryBlockRange
	}
	return &Indexer{
		chain:               configuredChain,
		sourcePathways:      sourcePathways,
		destinationPathways: destinationPathways,
		destinationEID:      configuredChain.EID,
		store:               store,
		client:              client,
		pollInterval:        defaultPollInterval,
		backfillRange:       defaultBackfillRange,
		queryBlockRange:     queryBlockRange,
		logger:              logger,
		streams:             allStreams(),
	}
}

// WithStreams selects the durable streams this indexer advances.
func (i *Indexer) WithStreams(streams StreamSet) *Indexer {
	i.streams = streams
	return i
}

// WithMetrics records process-local indexer polling metrics.
func (i *Indexer) WithMetrics(metrics MetricsRecorder) *Indexer {
	i.metrics = metrics
	return i
}

// Run starts the chain indexer loop until the context is canceled.
func (i *Indexer) Run(ctx context.Context) error {
	i.logger.Info("indexer loop started", "chain", i.chain.Name, "eid", i.chain.EID)
	return i.runPollingLoop(ctx)
}

// ProcessOnce backfills one confirmed log window for source and destination executor events.
func (i *Indexer) ProcessOnce(ctx context.Context) (ProcessResult, error) {
	if i.store == nil {
		return ProcessResult{}, errors.New("indexer store is required")
	}
	if i.client == nil {
		return ProcessResult{}, errors.New("indexer log client is required")
	}
	head, err := i.client.BlockNumber(ctx)
	if err != nil {
		return ProcessResult{}, err
	}
	if head < i.chain.Confirmations {
		return ProcessResult{ObservedHeadBlock: head}, nil
	}
	confirmedTo := head - i.chain.Confirmations
	result := ProcessResult{ConfirmedToBlock: confirmedTo, ObservedHeadBlock: head}
	sourceWindowSet := false
	destinationWindowSet := false
	if i.streams.ExecutorSource {
		from, to, ok, err := i.cursorWindow(ctx, executorSourceStream, confirmedTo)
		if err != nil {
			return ProcessResult{}, err
		}
		if ok {
			source, dvn, err := i.processSourceWindow(ctx, from, to, sourceRoleExecutor)
			if err != nil {
				return ProcessResult{}, err
			}
			if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, executorSourceStream, to); err != nil {
				return ProcessResult{}, err
			}
			result.SourceFromBlock = from
			result.SourceToBlock = to
			sourceWindowSet = true
			result.SourceTransactions += source
			result.DVNTransactions += dvn
		}
	}
	if i.streams.DVNSource {
		from, to, ok, err := i.cursorWindow(ctx, dvnSourceStream, confirmedTo)
		if err != nil {
			return ProcessResult{}, err
		}
		if ok {
			source, dvn, err := i.processSourceWindow(ctx, from, to, sourceRoleDVN)
			if err != nil {
				return ProcessResult{}, err
			}
			if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, dvnSourceStream, to); err != nil {
				return ProcessResult{}, err
			}
			if !sourceWindowSet {
				result.SourceFromBlock = from
				result.SourceToBlock = to
			}
			result.SourceTransactions += source
			result.DVNTransactions += dvn
		}
	}
	if i.streams.ExecutorDestination {
		from, to, ok, err := i.cursorWindow(ctx, executorDestStream, confirmedTo)
		if err != nil {
			return ProcessResult{}, err
		}
		if ok {
			destination, err := i.processDestinationWindow(ctx, from, to, sourceRoleExecutor)
			if err != nil {
				return ProcessResult{}, err
			}
			if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, executorDestStream, to); err != nil {
				return ProcessResult{}, err
			}
			result.DestinationFromBlock = from
			result.DestinationToBlock = to
			destinationWindowSet = true
			result.DestinationLogs += destination
		}
	}
	if i.streams.DVNDestination {
		from, to, ok, err := i.cursorWindow(ctx, dvnDestStream, confirmedTo)
		if err != nil {
			return ProcessResult{}, err
		}
		if ok {
			destination, err := i.processDestinationWindow(ctx, from, to, sourceRoleDVN)
			if err != nil {
				return ProcessResult{}, err
			}
			if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, dvnDestStream, to); err != nil {
				return ProcessResult{}, err
			}
			if !destinationWindowSet {
				result.DestinationFromBlock = from
				result.DestinationToBlock = to
			}
			result.DestinationLogs += destination
		}
	}
	return result, nil
}

// ProcessResult summarizes one indexer polling pass.
type ProcessResult struct {
	ConfirmedToBlock     uint64
	ObservedHeadBlock    uint64
	SourceFromBlock      uint64
	SourceToBlock        uint64
	DestinationFromBlock uint64
	DestinationToBlock   uint64
	SourceTransactions   int
	DVNTransactions      int
	DestinationLogs      int
}

func (i *Indexer) processSourceWindow(ctx context.Context, from, to uint64, role string) (int, int, error) {
	if len(i.sourcePathways) == 0 {
		return 0, 0, nil
	}
	executorProcessed := 0
	dvnProcessed := 0
	for chunkFrom := from; chunkFrom <= to; {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		chunkTo := i.chunkToBlock(chunkFrom, to)
		logs, err := i.client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: blockNumber(chunkFrom),
			ToBlock:   blockNumber(chunkTo),
			Addresses: i.sourceAddresses(role),
			Topics:    [][]common.Hash{i.sourceTopics(role)},
		})
		if err != nil {
			return executorProcessed, dvnProcessed, err
		}
		executor, dvn, err := i.processSourceLogs(ctx, logs, role)
		if err != nil {
			return executorProcessed, dvnProcessed, err
		}
		executorProcessed += executor
		dvnProcessed += dvn
		if chunkTo == to {
			break
		}
		chunkFrom = chunkTo + 1
	}
	return executorProcessed, dvnProcessed, nil
}

func (i *Indexer) processSourceLogs(ctx context.Context, logs []gethtypes.Log, role string) (int, int, error) {
	var current sourceTxLogs
	executorProcessed := 0
	dvnProcessed := 0
	for _, log := range logs {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		if len(current.logs) > 0 && current.txHash != log.TxHash {
			executor, dvn, err := i.processSourceTxLogs(ctx, current, role)
			if err != nil {
				return executorProcessed, dvnProcessed, err
			}
			executorProcessed += executor
			dvnProcessed += dvn
			current.reset()
		}
		current.append(log)
	}
	if len(current.logs) == 0 {
		return executorProcessed, dvnProcessed, nil
	}
	executor, dvn, err := i.processSourceTxLogs(ctx, current, role)
	if err != nil {
		return executorProcessed, dvnProcessed, err
	}
	return executorProcessed + executor, dvnProcessed + dvn, nil
}

func (i *Indexer) processSourceTxLogs(ctx context.Context, tx sourceTxLogs, role string) (int, int, error) {
	executorProcessed := 0
	dvnProcessed := 0
	// PacketSent and fee logs are decoder context; assignment logs anchor job processing.
	if role == sourceRoleExecutor && tx.hasExecutorAssignment {
		processed, err := i.processExecutorSourceTx(ctx, tx.logs)
		if err != nil {
			return executorProcessed, dvnProcessed, err
		}
		executorProcessed += processed
	}
	if role == sourceRoleDVN && tx.hasDVNAssignment {
		processed, err := i.processDVNSourceTx(ctx, tx.logs)
		if err != nil {
			return executorProcessed, dvnProcessed, err
		}
		dvnProcessed += processed
	}
	return executorProcessed, dvnProcessed, nil
}

type sourceTxLogs struct {
	txHash                common.Hash
	logs                  []gethtypes.Log
	hasExecutorAssignment bool
	hasDVNAssignment      bool
}

func (l *sourceTxLogs) append(log gethtypes.Log) {
	if len(l.logs) == 0 {
		l.txHash = log.TxHash
	}
	l.logs = append(l.logs, log)
	if len(log.Topics) == 0 {
		return
	}
	switch log.Topics[0] {
	case lzabi.ExecutorJobAssignedTopic():
		l.hasExecutorAssignment = true
	case lzabi.DVNJobAssignedTopic():
		l.hasDVNAssignment = true
	}
}

func (l *sourceTxLogs) reset() {
	l.logs = l.logs[:0]
	l.hasExecutorAssignment = false
	l.hasDVNAssignment = false
}

func (i *Indexer) chunkToBlock(from, limit uint64) uint64 {
	if i.queryBlockRange == 0 {
		return limit
	}
	to := from + i.queryBlockRange - 1
	if to < from || to > limit {
		return limit
	}
	return to
}

func (i *Indexer) processExecutorSourceTx(ctx context.Context, txLogs []gethtypes.Log) (int, error) {
	records, err := ExecutorSourceTxRecordsFromLogs(txLogs)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		pathway, ok := i.sourcePathway(record.Packet)
		if !ok || !pathway.Enabled {
			continue
		}
		if record.Executor != pathway.SourceWorkers.OpenExecutor {
			continue
		}
		if record.Packet.SendLib != pathway.SendLib {
			return processed, fmt.Errorf("packet %s send lib %s does not match configured send lib %s", record.Packet.GUID, record.Packet.SendLib, pathway.SendLib)
		}
		record.Packet.Status = record.ExecutorJob.Status
		if err := i.store.UpsertPacket(ctx, record.Packet); err != nil {
			return processed, err
		}
		if err := i.store.UpsertExecutorJob(ctx, record.ExecutorJob); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (i *Indexer) processDVNSourceTx(ctx context.Context, txLogs []gethtypes.Log) (int, error) {
	records, err := DVNSourceTxRecordsFromLogs(txLogs)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		pathway, ok := i.sourcePathway(record.Packet)
		if !ok || !pathway.Enabled {
			continue
		}
		if record.DVN != pathway.SourceWorkers.OpenDVN {
			continue
		}
		if record.Packet.SendLib != pathway.SendLib {
			return processed, fmt.Errorf("packet %s send lib %s does not match configured send lib %s", record.Packet.GUID, record.Packet.SendLib, pathway.SendLib)
		}
		if err := i.store.UpsertPacket(ctx, record.Packet); err != nil {
			return processed, err
		}
		if err := i.store.UpsertDVNJob(ctx, record.DVNJob); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (i *Indexer) processDestinationWindow(ctx context.Context, from, to uint64, role string) (int, error) {
	applied := 0
	for chunkFrom := from; chunkFrom <= to; {
		chunkTo := i.chunkToBlock(chunkFrom, to)
		logs, err := i.client.FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: blockNumber(chunkFrom),
			ToBlock:   blockNumber(chunkTo),
			Addresses: i.destinationAddresses(role),
			Topics:    [][]common.Hash{i.destinationTopics(role)},
		})
		if err != nil {
			return applied, err
		}
		switch role {
		case sourceRoleExecutor:
			executorApplied, err := ApplyExecutorDestinationLogs(ctx, i.store, i.destinationEID, logs)
			applied += executorApplied
			if err != nil {
				return applied, err
			}
		case sourceRoleDVN:
			dvnApplied, err := ApplyDVNDestinationLogs(ctx, i.store, i.destinationEID, i.destinationPathways, logs)
			applied += dvnApplied
			if err != nil {
				return applied, err
			}
		}
		if chunkTo == to {
			break
		}
		chunkFrom = chunkTo + 1
	}
	return applied, nil
}

func (i *Indexer) runPollingLoop(ctx context.Context) error {
	if i.pollInterval <= 0 {
		return workerloop.Fatal(errors.New("indexer poll interval must be positive"))
	}
	if i.store == nil {
		return workerloop.Fatal(errors.New("indexer store is required"))
	}
	if i.client == nil {
		return workerloop.Fatal(errors.New("indexer log client is required"))
	}
	if err := i.pollOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := i.pollOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (i *Indexer) pollOnce(ctx context.Context) error {
	start := time.Now()
	result, err := i.ProcessOnce(ctx)
	duration := time.Since(start)
	if i.metrics != nil {
		i.metrics.RecordIndexerPoll(
			i.chain.EID,
			i.chain.Name,
			result.ObservedHeadBlock,
			result.ConfirmedToBlock,
			result.SourceTransactions,
			result.DVNTransactions,
			result.DestinationLogs,
			duration,
			err,
		)
	}
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	i.logger.Warn("indexer poll failed; retrying on next interval", "chain", i.chain.Name, "eid", i.chain.EID, "error", err)
	return nil
}

func (i *Indexer) destinationAddresses(role string) []common.Address {
	seen := make(map[common.Address]struct{})
	switch role {
	case sourceRoleExecutor:
		seen[i.chain.EndpointAddress] = struct{}{}
	case sourceRoleDVN:
		for _, pathway := range i.destinationPathways {
			seen[pathway.ReceiveLib] = struct{}{}
		}
	}
	addresses := make([]common.Address, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(a, b int) bool {
		return hex.EncodeToString(addresses[a].Bytes()) < hex.EncodeToString(addresses[b].Bytes())
	})
	return addresses
}

func (i *Indexer) sourceAddresses(role string) []common.Address {
	seen := map[common.Address]struct{}{
		i.chain.EndpointAddress: {},
	}
	for _, pathway := range i.sourcePathways {
		seen[pathway.SendLib] = struct{}{}
		switch role {
		case sourceRoleExecutor:
			seen[pathway.SourceWorkers.OpenExecutor] = struct{}{}
		case sourceRoleDVN:
			seen[pathway.SourceWorkers.OpenDVN] = struct{}{}
		}
	}
	addresses := make([]common.Address, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(a, b int) bool {
		return hex.EncodeToString(addresses[a].Bytes()) < hex.EncodeToString(addresses[b].Bytes())
	})
	return addresses
}

func (i *Indexer) sourceTopics(role string) []common.Hash {
	switch role {
	case sourceRoleExecutor:
		return []common.Hash{
			lzabi.PacketSentTopic(),
			lzabi.ExecutorFeePaidTopic(),
			lzabi.ExecutorJobAssignedTopic(),
		}
	case sourceRoleDVN:
		return []common.Hash{
			lzabi.PacketSentTopic(),
			lzabi.DVNFeePaidTopic(),
			lzabi.DVNJobAssignedTopic(),
		}
	default:
		return nil
	}
}

func (i *Indexer) destinationTopics(role string) []common.Hash {
	switch role {
	case sourceRoleExecutor:
		return []common.Hash{
			lzabi.PacketVerifiedTopic(),
			lzabi.PacketDeliveredTopic(),
			lzabi.LzReceiveAlertTopic(),
		}
	case sourceRoleDVN:
		return []common.Hash{
			lzabi.PayloadVerifiedTopic(),
		}
	default:
		return nil
	}
}

func (i *Indexer) sourcePathway(packet db.PacketRecord) (chain.Pathway, bool) {
	for _, pathway := range i.sourcePathways {
		if pathway.DstEID == packet.DstEID && pathway.SrcOApp == packet.Sender && pathway.DstOApp == packet.Receiver {
			return pathway, true
		}
	}
	return chain.Pathway{}, false
}

func (i *Indexer) cursorWindow(ctx context.Context, stream string, confirmedTo uint64) (uint64, uint64, bool, error) {
	cursor, err := i.store.GetIndexerCursor(ctx, i.chain.EID, stream)
	cursorExists := true
	if errors.Is(err, pgx.ErrNoRows) {
		cursorExists = false
	} else if err != nil {
		return 0, 0, false, err
	}
	from := uint64(0)
	if cursorExists {
		if cursor >= confirmedTo {
			return 0, 0, false, nil
		}
		from = cursor + 1
	} else {
		from = i.chain.StartBlockNumber
		if from > confirmedTo {
			return 0, 0, false, nil
		}
	}
	to := confirmedTo
	if i.backfillRange > 0 && to-from+1 > i.backfillRange {
		to = from + i.backfillRange - 1
	}
	return from, to, true, nil
}

func blockNumber(number uint64) *big.Int {
	return new(big.Int).SetUint64(number)
}
