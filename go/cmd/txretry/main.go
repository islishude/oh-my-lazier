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
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	action := flag.String("action", "", "inspect, retry-failed, replace, cancel-nonce, or resolve-external-nonce")
	id := flag.Int64("id", 0, "tx_outbox id")
	resolution := flag.String("resolution", "", "external nonce resolution: retry or abandon")
	flag.Parse()
	if err := run(*configPath, txretry.Options{ID: *id, Action: *action, Resolution: *resolution}); err != nil {
		fmt.Fprintf(os.Stderr, "txretry: %v\n", err)
		os.Exit(1)
	}
}

func run(path string, options txretry.Options) error {
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
	result, err := txretry.Run(ctx, store, options)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
