package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/dvn"
	"github.com/islishude/oh-my-lazier/go/internal/executor"
	"github.com/islishude/oh-my-lazier/go/internal/indexer"
	"github.com/islishude/oh-my-lazier/go/internal/metrics"
	"github.com/islishude/oh-my-lazier/go/internal/pricing"
	"github.com/islishude/oh-my-lazier/go/internal/txmgr"
)

// App owns the configured worker process and its durable service loops.
type App struct {
	cfg    config.Config
	logger *slog.Logger
}

// New builds an App from already-validated configuration.
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	return &App{cfg: cfg, logger: logger}, nil
}

// Run connects dependencies and runs all worker loops until cancellation or a loop failure.
func (a *App) Run(ctx context.Context) error {
	store, err := db.Connect(ctx, a.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		return err
	}

	registry, err := chain.NewRegistry(a.cfg.Chains)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 8+len(a.cfg.Chains))
	start := func(name string, run func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Any durable loop failure cancels the whole worker; partial worker operation can
			// otherwise advance packet state with missing indexers or senders.
			if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
				cancel()
			}
			a.logger.Info("loop stopped", "name", name)
		}()
	}

	start("metrics", metrics.New(a.cfg.Metrics.ListenAddress, a.logger).Run)
	for _, c := range registry.All() {
		start("indexer."+c.Name, indexer.New(c, a.logger).Run)
	}
	start("txmgr", txmgr.New(store, a.logger).Run)

	executorWorker := executor.New(a.logger)
	start("executor.committer", executorWorker.RunCommitter)
	start("executor.deliverer", executorWorker.RunDeliverer)
	start("dvn", dvn.New(a.cfg.DVN.Mode, a.logger).Run)
	start("pricing", pricing.New(a.logger).Run)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		cancel()
		wg.Wait()
		return err
	}
	wg.Wait()
	return nil
}
