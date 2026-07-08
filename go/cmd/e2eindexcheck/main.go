package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/islishude/oh-my-lazier/go/cmd/e2eindexcheck/e2eindexcheck"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func main() {
	configPath := flag.String("config", "tmp/e2e/worker.host.yaml", "worker config path")
	evidencePath := flag.String("evidence", "tmp/e2e/multi-oft-send-indexer.json", "multi-send indexer evidence path")
	timeout := flag.Duration("timeout", 60*time.Second, "maximum wait for indexer rows")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	evidence, err := e2eindexcheck.LoadEvidence(*evidencePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load evidence: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := e2eindexcheck.Check(ctx, cfg.DatabaseURL, evidence, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "check indexer evidence: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(
		"multi-send indexer rows verified: tx=%s packets=%d\n",
		evidence.SourceTxHash,
		len(evidence.ExpectedPackets),
	)
}
