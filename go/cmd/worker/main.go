package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/app"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/logging"
)

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	logLevelName := flag.String("log-level", logging.DefaultLevelName, "minimum log level: debug, info, warn, or error")
	indexerProgressLogInterval := flag.Duration("indexer-progress-log-interval", app.DefaultIndexerProgressLogInterval, "minimum interval between indexer progress info logs; 0 disables periodic progress info logs")
	flag.Parse()

	logLevel, err := logging.ParseLevel(*logLevelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse log level: %v\n", err)
		os.Exit(1)
	}
	if *indexerProgressLogInterval < 0 {
		fmt.Fprintf(os.Stderr, "parse indexer progress log interval: must be non-negative\n")
		os.Exit(1)
	}
	logger := logging.NewTextLogger(os.Stdout, logLevel)
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	worker, err := app.NewWithOptions(cfg, logger, app.Options{
		IndexerProgressLogInterval:    *indexerProgressLogInterval,
		IndexerProgressLogIntervalSet: true,
	})
	if err != nil {
		logger.Error("create worker", "error", err)
		os.Exit(1)
	}
	if err := worker.Run(ctx); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
