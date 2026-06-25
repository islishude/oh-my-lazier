package indexer

import (
	"context"
	"log/slog"

	"github.com/islishude/oh-my-lazier/go/internal/chain"
)

// Indexer watches one chain for LayerZero and worker contract events.
type Indexer struct {
	chain  chain.Chain
	logger *slog.Logger
}

// New creates an indexer for one configured chain.
func New(chain chain.Chain, logger *slog.Logger) *Indexer {
	return &Indexer{chain: chain, logger: logger}
}

// Run starts the chain indexer loop until the context is canceled.
func (i *Indexer) Run(ctx context.Context) error {
	i.logger.Info("indexer loop started", "chain", i.chain.Name, "eid", i.chain.EID)
	<-ctx.Done()
	return ctx.Err()
}
