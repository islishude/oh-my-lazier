package executor

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/packets"
)

const loopInterval = 5 * time.Second

// Store is the durable executor state required by the worker.
type Store interface {
	ListExecutorWork(ctx context.Context, status string, limit int) ([]db.ExecutorWorkItem, error)
	EnqueueExecutorTx(ctx context.Context, guid common.Hash, expectedStatus, nextStatus string, request db.TxRequest) (int64, error)
}

// Worker runs executor commit and delivery workflows.
type Worker struct {
	store    Store
	registry *chain.Registry
	callers  map[uint32]ContractCaller
	signerID string
	logger   *slog.Logger
}

// New creates an executor worker.
func New(store Store, registry *chain.Registry, signerID string, logger *slog.Logger) *Worker {
	callers := make(map[uint32]ContractCaller)
	if registry != nil {
		for _, configuredChain := range registry.All() {
			if configuredChain.RPC != nil {
				callers[configuredChain.EID] = configuredChain.RPC
			}
		}
	}
	return NewWithCallers(store, registry, callers, signerID, logger)
}

// NewWithCallers creates an executor worker with explicit chain call clients.
func NewWithCallers(store Store, registry *chain.Registry, callers map[uint32]ContractCaller, signerID string, logger *slog.Logger) *Worker {
	copiedCallers := make(map[uint32]ContractCaller, len(callers))
	maps.Copy(copiedCallers, callers)
	return &Worker{store: store, registry: registry, callers: copiedCallers, signerID: signerID, logger: logger}
}

// RunCommitter starts the commitVerification enqueue loop.
func (w *Worker) RunCommitter(ctx context.Context) error {
	w.logger.Info("executor committer loop started")
	return w.runLoop(ctx, w.ProcessCommitterOnce)
}

// RunDeliverer starts the lzReceive delivery loop.
func (w *Worker) RunDeliverer(ctx context.Context) error {
	w.logger.Info("executor deliverer loop started")
	return w.runLoop(ctx, w.ProcessDelivererOnce)
}

// ProcessCommitterOnce enqueues one commitVerification transaction for a verifiable packet.
func (w *Worker) ProcessCommitterOnce(ctx context.Context) (bool, error) {
	work, err := w.store.ListExecutorWork(ctx, string(packets.ExecutorVerifiable), 1)
	if err != nil {
		return false, err
	}
	if len(work) == 0 {
		return false, nil
	}
	item := work[0]
	pathway, err := w.registry.Pathway(item.Packet.SrcEID, item.Packet.DstEID, item.Packet.Sender, item.Packet.Receiver)
	if err != nil {
		return false, err
	}
	if !pathway.Enabled {
		return false, nil
	}
	dstChain, err := w.registry.Get(item.Packet.DstEID)
	if err != nil {
		return false, err
	}
	ready, err := IsCommitVerifiable(ctx, w.caller(item.Packet.DstEID), dstChain.EndpointAddress, pathway.ReceiveLib, item.Packet)
	if err != nil {
		return false, err
	}
	if !ready {
		return false, nil
	}
	request, err := BuildCommitVerificationTx(item.Packet, pathway.ReceiveLib, w.signerID, TxFees{})
	if err != nil {
		return false, err
	}
	id, err := w.store.EnqueueExecutorTx(ctx, item.Packet.GUID, string(packets.ExecutorVerifiable), string(packets.ExecutorCommitTxEnqueued), request)
	if err != nil {
		return false, err
	}
	w.logger.Info("enqueued executor commit tx", "guid", item.Packet.GUID, "tx_outbox_id", id)
	return true, nil
}

// ProcessDelivererOnce enqueues one lzReceive transaction for an executable packet.
func (w *Worker) ProcessDelivererOnce(ctx context.Context) (bool, error) {
	if processed, err := w.processDelivererStatus(ctx, string(packets.ExecutorExecutable)); err != nil || processed {
		return processed, err
	}
	return w.processDelivererStatus(ctx, string(packets.ExecutorLzReceiveFailed))
}

func (w *Worker) processDelivererStatus(ctx context.Context, status string) (bool, error) {
	work, err := w.store.ListExecutorWork(ctx, status, 1)
	if err != nil || len(work) == 0 {
		return false, err
	}
	item := work[0]
	dstChain, err := w.registry.Get(item.Packet.DstEID)
	if err != nil {
		return false, err
	}
	ready, err := IsLzReceiveExecutable(ctx, w.caller(item.Packet.DstEID), dstChain.EndpointAddress, item.Packet)
	if err != nil {
		return false, err
	}
	if !ready {
		return false, nil
	}
	request, err := BuildLzReceiveTx(item.Packet, dstChain.EndpointAddress, w.signerID, TxFees{})
	if err != nil {
		return false, err
	}
	id, err := w.store.EnqueueExecutorTx(ctx, item.Packet.GUID, status, string(packets.ExecutorLzReceiveTxEnqueued), request)
	if err != nil {
		return false, err
	}
	w.logger.Info("enqueued executor lzReceive tx", "guid", item.Packet.GUID, "tx_outbox_id", id)
	return true, nil
}

func (w *Worker) runLoop(ctx context.Context, process func(context.Context) (bool, error)) error {
	for {
		processed, err := process(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		timer := time.NewTimer(loopInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (w *Worker) caller(eid uint32) ContractCaller {
	if w == nil {
		return nil
	}
	return w.callers[eid]
}
