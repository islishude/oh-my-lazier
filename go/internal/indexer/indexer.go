package indexer

import (
	"context"
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
	"github.com/jackc/pgx/v5"
)

const (
	defaultPollInterval  = 5 * time.Second
	defaultBackfillRange = uint64(500)
	executorSourceStream = "executor_source"
	executorDestStream   = "executor_destination"
)

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

// SubscriptionLogClient reads historical logs and subscribes to live log notifications.
type SubscriptionLogClient interface {
	LogClient
	SubscribeFilterLogs(ctx context.Context, query ethereum.FilterQuery, ch chan<- gethtypes.Log) (ethereum.Subscription, error)
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
	expectedExecutor    common.Address
	expectedDVN         common.Address
	logger              *slog.Logger
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
	return &Indexer{
		chain:               configuredChain,
		sourcePathways:      sourcePathways,
		destinationPathways: destinationPathways,
		destinationEID:      configuredChain.EID,
		store:               store,
		client:              client,
		pollInterval:        defaultPollInterval,
		backfillRange:       defaultBackfillRange,
		expectedExecutor:    configuredChain.Workers.OpenExecutor,
		expectedDVN:         configuredChain.Workers.OpenDVN,
		logger:              logger,
	}
}

// Run starts the chain indexer loop until the context is canceled.
func (i *Indexer) Run(ctx context.Context) error {
	i.logger.Info("indexer loop started", "chain", i.chain.Name, "eid", i.chain.EID)
	if _, err := i.ProcessOnce(ctx); err != nil {
		return err
	}
	subscriber, ok := i.client.(SubscriptionLogClient)
	if !ok {
		return i.runPollingLoop(ctx)
	}
	logs := make(chan gethtypes.Log, 256)
	subscription, err := subscriber.SubscribeFilterLogs(ctx, i.liveQuery(), logs)
	if err != nil {
		i.logger.Warn("indexer live subscription unavailable; using polling", "chain", i.chain.Name, "error", err)
		return i.runPollingLoop(ctx)
	}
	defer subscription.Unsubscribe()
	i.logger.Info("indexer live subscription started", "chain", i.chain.Name, "eid", i.chain.EID)
	return i.runLiveLoop(ctx, logs, subscription.Err())
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
		return ProcessResult{}, nil
	}
	confirmedTo := head - i.chain.Confirmations
	result := ProcessResult{ConfirmedToBlock: confirmedTo}
	if from, to, ok, err := i.cursorWindow(ctx, executorSourceStream, confirmedTo); err != nil {
		return ProcessResult{}, err
	} else if ok {
		source, dvn, err := i.processSourceWindow(ctx, from, to)
		if err != nil {
			return ProcessResult{}, err
		}
		if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, executorSourceStream, to); err != nil {
			return ProcessResult{}, err
		}
		result.SourceFromBlock = from
		result.SourceToBlock = to
		result.SourceTransactions = source
		result.DVNTransactions = dvn
	}
	if from, to, ok, err := i.cursorWindow(ctx, executorDestStream, confirmedTo); err != nil {
		return ProcessResult{}, err
	} else if ok {
		destination, err := i.processDestinationWindow(ctx, from, to)
		if err != nil {
			return ProcessResult{}, err
		}
		if err := i.store.UpdateIndexerCursor(ctx, i.chain.EID, executorDestStream, to); err != nil {
			return ProcessResult{}, err
		}
		result.DestinationFromBlock = from
		result.DestinationToBlock = to
		result.DestinationLogs = destination
	}
	return result, nil
}

// ProcessResult summarizes one indexer polling pass.
type ProcessResult struct {
	ConfirmedToBlock     uint64
	SourceFromBlock      uint64
	SourceToBlock        uint64
	DestinationFromBlock uint64
	DestinationToBlock   uint64
	SourceTransactions   int
	DVNTransactions      int
	DestinationLogs      int
}

func (i *Indexer) processSourceWindow(ctx context.Context, from, to uint64) (int, int, error) {
	if len(i.sourcePathways) == 0 {
		return 0, 0, nil
	}
	logs, err := i.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: blockNumber(from),
		ToBlock:   blockNumber(to),
		Addresses: i.sourceAddresses(),
		Topics: [][]common.Hash{{
			lzabi.PacketSentTopic(),
			lzabi.ExecutorFeePaidTopic(),
			lzabi.ExecutorJobAssignedTopic(),
			lzabi.DVNFeePaidTopic(),
			lzabi.DVNJobAssignedTopic(),
		}},
	})
	if err != nil {
		return 0, 0, err
	}
	groups := logsByTxHash(logs)
	executorProcessed := 0
	dvnProcessed := 0
	for _, txLogs := range groups {
		if containsTopic(txLogs, lzabi.ExecutorJobAssignedTopic()) {
			didProcess, err := i.processExecutorSourceTx(ctx, txLogs)
			if err != nil {
				return executorProcessed, dvnProcessed, err
			}
			if didProcess {
				executorProcessed++
			}
		}
		if containsTopic(txLogs, lzabi.DVNJobAssignedTopic()) {
			didProcess, err := i.processDVNSourceTx(ctx, txLogs)
			if err != nil {
				return executorProcessed, dvnProcessed, err
			}
			if didProcess {
				dvnProcessed++
			}
		}
	}
	return executorProcessed, dvnProcessed, nil
}

func (i *Indexer) processExecutorSourceTx(ctx context.Context, txLogs []gethtypes.Log) (bool, error) {
	records, err := ExecutorSourceTxRecordsFromLogs(txLogs, i.expectedExecutor)
	if err != nil {
		return false, err
	}
	pathway, ok := i.sourcePathway(records.Packet)
	if !ok || !pathway.Enabled {
		return false, nil
	}
	if records.Packet.SendLib != pathway.SendLib {
		return false, fmt.Errorf("packet %s send lib %s does not match configured send lib %s", records.Packet.GUID, records.Packet.SendLib, pathway.SendLib)
	}
	records.Packet.Status = records.ExecutorJob.Status
	if err := i.store.UpsertPacket(ctx, records.Packet); err != nil {
		return false, err
	}
	if err := i.store.UpsertExecutorJob(ctx, records.ExecutorJob); err != nil {
		return false, err
	}
	return true, nil
}

func (i *Indexer) processDVNSourceTx(ctx context.Context, txLogs []gethtypes.Log) (bool, error) {
	records, err := DVNSourceTxRecordsFromLogs(txLogs, i.expectedDVN)
	if err != nil {
		return false, err
	}
	pathway, ok := i.sourcePathway(records.Packet)
	if !ok || !pathway.Enabled {
		return false, nil
	}
	if records.Packet.SendLib != pathway.SendLib {
		return false, fmt.Errorf("packet %s send lib %s does not match configured send lib %s", records.Packet.GUID, records.Packet.SendLib, pathway.SendLib)
	}
	if err := i.store.UpsertPacket(ctx, records.Packet); err != nil {
		return false, err
	}
	if err := i.store.UpsertDVNJob(ctx, records.DVNJob); err != nil {
		return false, err
	}
	return true, nil
}

func (i *Indexer) processDestinationWindow(ctx context.Context, from, to uint64) (int, error) {
	logs, err := i.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: blockNumber(from),
		ToBlock:   blockNumber(to),
		Addresses: i.destinationAddresses(),
		Topics: [][]common.Hash{{
			lzabi.PacketVerifiedTopic(),
			lzabi.PacketDeliveredTopic(),
			lzabi.LzReceiveAlertTopic(),
			lzabi.PayloadVerifiedTopic(),
		}},
	})
	if err != nil {
		return 0, err
	}
	executorApplied, err := ApplyExecutorDestinationLogs(ctx, i.store, i.destinationEID, logs)
	if err != nil {
		return executorApplied, err
	}
	dvnApplied, err := ApplyDVNDestinationLogs(ctx, i.store, i.destinationEID, i.expectedDVN, logs)
	if err != nil {
		return executorApplied, err
	}
	return executorApplied + dvnApplied, nil
}

func (i *Indexer) runPollingLoop(ctx context.Context) error {
	for {
		timer := time.NewTimer(i.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if _, err := i.ProcessOnce(ctx); err != nil {
			return err
		}
	}
}

func (i *Indexer) runLiveLoop(ctx context.Context, logs <-chan gethtypes.Log, subErr <-chan error) error {
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-subErr:
			if err != nil {
				i.logger.Warn("indexer live subscription stopped; continuing with polling", "chain", i.chain.Name, "error", err)
			}
			return i.runPollingLoop(ctx)
		case <-logs:
			drainLogs(logs)
			if _, err := i.ProcessOnce(ctx); err != nil {
				return err
			}
		case <-ticker.C:
			if _, err := i.ProcessOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func drainLogs(logs <-chan gethtypes.Log) {
	for {
		select {
		case <-logs:
		default:
			return
		}
	}
}

func (i *Indexer) liveQuery() ethereum.FilterQuery {
	seen := make(map[common.Address]struct{})
	for _, address := range i.sourceAddresses() {
		seen[address] = struct{}{}
	}
	for _, address := range i.destinationAddresses() {
		seen[address] = struct{}{}
	}
	addresses := make([]common.Address, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(a, b int) bool {
		return addresses[a].Hex() < addresses[b].Hex()
	})
	return ethereum.FilterQuery{
		Addresses: addresses,
		Topics: [][]common.Hash{{
			lzabi.PacketSentTopic(),
			lzabi.ExecutorFeePaidTopic(),
			lzabi.ExecutorJobAssignedTopic(),
			lzabi.DVNFeePaidTopic(),
			lzabi.DVNJobAssignedTopic(),
			lzabi.PacketVerifiedTopic(),
			lzabi.PacketDeliveredTopic(),
			lzabi.LzReceiveAlertTopic(),
			lzabi.PayloadVerifiedTopic(),
		}},
	}
}

func (i *Indexer) destinationAddresses() []common.Address {
	seen := map[common.Address]struct{}{
		i.chain.EndpointAddress: {},
	}
	for _, pathway := range i.destinationPathways {
		seen[pathway.ReceiveLib] = struct{}{}
	}
	addresses := make([]common.Address, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(a, b int) bool {
		return addresses[a].Hex() < addresses[b].Hex()
	})
	return addresses
}

func (i *Indexer) sourceAddresses() []common.Address {
	seen := map[common.Address]struct{}{
		i.chain.EndpointAddress:      {},
		i.chain.Workers.OpenExecutor: {},
		i.chain.Workers.OpenDVN:      {},
	}
	for _, pathway := range i.sourcePathways {
		seen[pathway.SendLib] = struct{}{}
	}
	addresses := make([]common.Address, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(a, b int) bool {
		return addresses[a].Hex() < addresses[b].Hex()
	})
	return addresses
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

func logsByTxHash(logs []gethtypes.Log) [][]gethtypes.Log {
	ordered := make([]gethtypes.Log, len(logs))
	copy(ordered, logs)
	sort.SliceStable(ordered, func(a, b int) bool {
		if ordered[a].BlockNumber != ordered[b].BlockNumber {
			return ordered[a].BlockNumber < ordered[b].BlockNumber
		}
		if ordered[a].TxIndex != ordered[b].TxIndex {
			return ordered[a].TxIndex < ordered[b].TxIndex
		}
		return ordered[a].Index < ordered[b].Index
	})
	groups := make([][]gethtypes.Log, 0)
	byHash := make(map[common.Hash]int)
	for _, log := range ordered {
		idx, ok := byHash[log.TxHash]
		if !ok {
			byHash[log.TxHash] = len(groups)
			groups = append(groups, []gethtypes.Log{log})
			continue
		}
		groups[idx] = append(groups[idx], log)
	}
	return groups
}

func containsTopic(logs []gethtypes.Log, topic common.Hash) bool {
	for _, log := range logs {
		if len(log.Topics) > 0 && log.Topics[0] == topic {
			return true
		}
	}
	return false
}
