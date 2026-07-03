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
	flag.Parse()

	logLevel, err := logging.ParseLevel(*logLevelName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse log level: %v\n", err)
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

	worker, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("create worker", "error", err)
		os.Exit(1)
	}
	if err := worker.Run(ctx); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
