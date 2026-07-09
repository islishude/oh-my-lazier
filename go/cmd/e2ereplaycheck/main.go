package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/islishude/oh-my-lazier/go/cmd/e2ereplaycheck/e2ereplaycheck"
	"github.com/islishude/oh-my-lazier/go/internal/config"
)

func main() {
	configPath := flag.String("config", "tmp/e2e/worker.host.yaml", "worker config path")
	evidencePath := flag.String("evidence", "tmp/e2e/destination-replay.json", "destination replay evidence path")
	timeout := flag.Duration("timeout", 60*time.Second, "maximum wait for replay convergence")
	flag.Parse()

	cfg, err := config.LoadStatic(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	evidence, err := e2ereplaycheck.LoadEvidence(*evidencePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load evidence: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := e2ereplaycheck.Check(ctx, cfg, evidence, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "check destination replay: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(
		"destination replay verified: tx=%s packets=%d\n",
		evidence.SourceTxHash,
		len(evidence.ExpectedPackets),
	)
}
