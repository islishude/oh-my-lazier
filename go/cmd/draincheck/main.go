package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/db"
)

func main() {
	configPath := flag.String("config", "config/example.yaml", "worker config path")
	srcEID := flag.Uint("src-eid", 0, "source endpoint ID")
	dstEID := flag.Uint("dst-eid", 0, "destination endpoint ID")
	format := flag.String("format", "text", "output format: text or json")
	failOnPending := flag.Bool("fail-on-pending", true, "exit with status 2 when pathway is not drained")
	flag.Parse()

	if *srcEID == 0 || *dstEID == 0 {
		fmt.Fprintln(os.Stderr, "-src-eid and -dst-eid are required")
		os.Exit(1)
	}
	if *srcEID > uint(^uint32(0)) || *dstEID > uint(^uint32(0)) {
		fmt.Fprintln(os.Stderr, "endpoint IDs must fit uint32")
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

	status, err := store.CheckDrainStatus(ctx, uint32(*srcEID), uint32(*dstEID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check drain status: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "text":
		fmt.Print(renderText(status))
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(status); err != nil {
			fmt.Fprintf(os.Stderr, "encode drain status: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(1)
	}
	if *failOnPending && !status.Ready {
		os.Exit(2)
	}
}

func renderText(status db.DrainStatus) string {
	out := fmt.Sprintf("pathway %d -> %d\n", status.SrcEID, status.DstEID)
	out += fmt.Sprintf("ready: %t\n", status.Ready)
	out += fmt.Sprintf("packets_total: %d\n", status.PacketsTotal)
	out += renderCounts("executor_pending", status.ExecutorPending)
	out += renderCounts("dvn_pending", status.DVNPending)
	out += renderCounts("outbox_pending", status.OutboxPending)
	out += fmt.Sprintf("verified_but_undelivered_count: %d\n", status.VerifiedButUndeliveredCount)
	return out
}

func renderCounts(label string, counts []db.StatusCount) string {
	if len(counts) == 0 {
		return fmt.Sprintf("%s: none\n", label)
	}
	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s:\n", label))
	for _, count := range counts {
		out.WriteString(fmt.Sprintf("- %s: %d\n", count.Status, count.Count))
	}
	return out.String()
}
