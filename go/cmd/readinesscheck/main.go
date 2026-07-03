package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
	"github.com/islishude/oh-my-lazier/go/internal/readiness"
)

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	format := flag.String("format", "text", "output format: text or json")
	failOnNotReady := flag.Bool("fail-on-not-ready", true, "exit with status 2 when readiness checks fail")
	flag.Parse()

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

	stats, err := store.Stats(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load stats: %v\n", err)
		os.Exit(1)
	}
	report := readiness.EvaluateWithServices(stats, readiness.Services{
		ExecutorEnabled: cfg.ExecutorEnabled(),
		DVNEnabled:      cfg.DVNEnabled(),
	})
	switch *format {
	case "text":
		fmt.Print(renderText(report))
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode readiness report: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(1)
	}
	if *failOnNotReady && !report.Ready {
		os.Exit(2)
	}
}

func renderText(report readiness.Report) string {
	out := fmt.Sprintf("ready: %t\n", report.Ready)
	if len(report.Issues) == 0 {
		return out + "issues: none\n"
	}
	out += "issues:\n"
	for _, issue := range report.Issues {
		out += fmt.Sprintf("- %s: %s\n", issue.Code, issue.Message)
	}
	return out
}
