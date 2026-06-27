package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

type retryOutput struct {
	Action string      `json:"action"`
	Before db.OutboxTx `json:"before"`
	After  db.OutboxTx `json:"after"`
}

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	action := flag.String("action", "", "retry action: retry-failed or replace")
	id := flag.Int64("id", 0, "tx_outbox id")
	maxFeePerGasValue := flag.String("max-fee-per-gas", "", "replacement max fee per gas in wei")
	maxPriorityFeePerGasValue := flag.String("max-priority-fee-per-gas", "", "replacement max priority fee per gas in wei")
	flag.Parse()

	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "-id is required")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping database: %v\n", err)
		os.Exit(1)
	}

	before, err := store.GetOutboxTx(ctx, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load outbox tx: %v\n", err)
		os.Exit(1)
	}

	switch *action {
	case "retry-failed":
		maxFeePerGas, err := parseOptionalPositiveBigInt(*maxFeePerGasValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse max fee: %v\n", err)
			os.Exit(1)
		}
		maxPriorityFeePerGas, err := parseOptionalPositiveBigInt(*maxPriorityFeePerGasValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse priority fee: %v\n", err)
			os.Exit(1)
		}
		if err := store.RetryFailedTx(ctx, *id, maxFeePerGas, maxPriorityFeePerGas); err != nil {
			fmt.Fprintf(os.Stderr, "retry failed tx: %v\n", err)
			os.Exit(1)
		}
	case "replace":
		maxFeePerGas, maxPriorityFeePerGas, err := parseRequiredFees(*maxFeePerGasValue, *maxPriorityFeePerGasValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse replacement fees: %v\n", err)
			os.Exit(1)
		}
		if err := store.PrepareReplacementTx(ctx, *id, maxFeePerGas, maxPriorityFeePerGas); err != nil {
			fmt.Fprintf(os.Stderr, "prepare replacement tx: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "-action must be retry-failed or replace")
		os.Exit(1)
	}

	after, err := store.GetOutboxTx(ctx, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reload outbox tx: %v\n", err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(retryOutput{Action: *action, Before: before, After: after}); err != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", err)
		os.Exit(1)
	}
}

func parseRequiredFees(maxFeeValue, priorityFeeValue string) (*big.Int, *big.Int, error) {
	maxFee, err := parseOptionalPositiveBigInt(maxFeeValue)
	if err != nil {
		return nil, nil, err
	}
	priorityFee, err := parseOptionalPositiveBigInt(priorityFeeValue)
	if err != nil {
		return nil, nil, err
	}
	if maxFee == nil || priorityFee == nil {
		return nil, nil, errors.New("max fee and priority fee are both required")
	}
	return maxFee, priorityFee, nil
}

func parseOptionalPositiveBigInt(value string) (*big.Int, error) {
	if value == "" {
		return nil, nil
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("%q is not a decimal integer", value)
	}
	if parsed.Sign() <= 0 {
		return nil, errors.New("fee must be positive")
	}
	return parsed, nil
}
