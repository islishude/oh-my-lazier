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
	maxFeePerGasValue := flag.String("max-fee-per-gas", "", "dynamic-fee replacement max fee per gas in wei")
	maxPriorityFeePerGasValue := flag.String("max-priority-fee-per-gas", "", "dynamic-fee replacement max priority fee per gas in wei")
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
	txType, err := chainTxType(cfg, before.ChainEID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve tx type: %v\n", err)
		os.Exit(1)
	}

	switch *action {
	case "retry-failed":
		maxFeePerGas, maxPriorityFeePerGas, legacyReplacement, err := retryInputs(*action, txType, *maxFeePerGasValue, *maxPriorityFeePerGasValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse retry inputs: %v\n", err)
			os.Exit(1)
		}
		if legacyReplacement {
			fmt.Fprintln(os.Stderr, "retry-failed does not support legacy replacement")
			os.Exit(1)
		}
		if err := store.RetryFailedTx(ctx, *id, maxFeePerGas, maxPriorityFeePerGas); err != nil {
			fmt.Fprintf(os.Stderr, "retry failed tx: %v\n", err)
			os.Exit(1)
		}
	case "replace":
		maxFeePerGas, maxPriorityFeePerGas, legacyReplacement, err := retryInputs(*action, txType, *maxFeePerGasValue, *maxPriorityFeePerGasValue)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse replacement inputs: %v\n", err)
			os.Exit(1)
		}
		if legacyReplacement {
			if err := store.PrepareLegacyReplacementTx(ctx, *id); err != nil {
				fmt.Fprintf(os.Stderr, "prepare legacy replacement tx: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := store.PrepareReplacementTx(ctx, *id, maxFeePerGas, maxPriorityFeePerGas); err != nil {
				fmt.Fprintf(os.Stderr, "prepare replacement tx: %v\n", err)
				os.Exit(1)
			}
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

func retryInputs(action, txType, maxFeeValue, priorityFeeValue string) (*big.Int, *big.Int, bool, error) {
	switch txType {
	case config.TxTypeDynamicFee, "":
		switch action {
		case "retry-failed":
			maxFee, err := parseOptionalPositiveBigInt(maxFeeValue)
			if err != nil {
				return nil, nil, false, fmt.Errorf("max fee: %w", err)
			}
			priorityFee, err := parseOptionalPositiveBigInt(priorityFeeValue)
			if err != nil {
				return nil, nil, false, fmt.Errorf("priority fee: %w", err)
			}
			return maxFee, priorityFee, false, nil
		case "replace":
			maxFee, priorityFee, err := parseRequiredFees(maxFeeValue, priorityFeeValue)
			return maxFee, priorityFee, false, err
		default:
			return nil, nil, false, fmt.Errorf("unsupported action %q", action)
		}
	case config.TxTypeLegacy:
		if maxFeeValue != "" || priorityFeeValue != "" {
			return nil, nil, false, errors.New("legacy tx replacement uses RPC-suggested gas price; EIP-1559 fee flags are not supported")
		}
		switch action {
		case "retry-failed":
			return nil, nil, false, nil
		case "replace":
			return nil, nil, true, nil
		default:
			return nil, nil, false, fmt.Errorf("unsupported action %q", action)
		}
	default:
		return nil, nil, false, fmt.Errorf("unsupported tx type %q", txType)
	}
}

func chainTxType(cfg config.Config, chainEID uint32) (string, error) {
	for _, chain := range cfg.Chains {
		if chain.EID == chainEID {
			return config.NormalizeTxType(chain.TxType), nil
		}
	}
	return "", fmt.Errorf("chain eid %d is not configured", chainEID)
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
