package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/chain"
	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/configcheck"
)

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	format := flag.String("format", "text", "output format: text or json")
	failOnMismatch := flag.Bool("fail-on-mismatch", true, "exit with status 2 when on-chain state does not match config")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	registry, err := chain.NewRegistry(cfg.Chains, cfg.Pathways)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build registry: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var opts []configcheck.Option
	if cfg.Pricing.Enabled {
		opts = append(opts, configcheck.WithPricingSigner(cfg.Pricing.Signer.Common()))
	}
	report, err := configcheck.Check(ctx, registry, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check on-chain config: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "text":
		fmt.Print(configcheck.RenderText(report))
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode report: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(1)
	}
	if *failOnMismatch && !report.OK {
		os.Exit(2)
	}
}
