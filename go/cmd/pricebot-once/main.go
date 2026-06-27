package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/app"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	worker, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("create app", "error", err)
		os.Exit(1)
	}
	if err := worker.RunPriceOnce(ctx); err != nil {
		logger.Error("enqueue price updates", "error", err)
		os.Exit(1)
	}
	logger.Info("price update batch enqueued")
}
