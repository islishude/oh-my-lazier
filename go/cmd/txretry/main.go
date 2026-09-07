package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/cmd/txretry/txretry"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

func main() {
	configPath := flag.String("config", "config.yaml", "worker config path")
	action := flag.String("action", "", "inspect, retry-failed, replace, cancel-nonce, resolve-external-nonce, or rebroadcast")
	id := flag.Int64("id", 0, "tx_outbox id")
	resolution := flag.String("resolution", "", "external nonce resolution: retry or abandon")
	rpcURL := flag.String("rpc-url", "", "RPC endpoint for rebroadcast (required only for that action)")
	flag.Parse()
	if err := run(*configPath, txretry.Options{ID: *id, Action: *action, Resolution: *resolution, RPCURL: *rpcURL}); err != nil {
		fmt.Fprintf(os.Stderr, "txretry: %v\n", err)
		os.Exit(1)
	}
}

func run(path string, options txretry.Options) error {
	if err := options.Validate(); err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := txretry.Run(ctx, store, options, txretry.Dependencies{Chains: cfg.Chains})
	if len(result) > 0 {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			return fmt.Errorf("command result could not be written (action may have completed): %w", encodeErr)
		}
	}
	return err
}
