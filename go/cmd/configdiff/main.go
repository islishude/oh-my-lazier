package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/islishude/oh-my-lazier/go/internal/config"
	"github.com/islishude/oh-my-lazier/go/internal/configdiff"
)

func main() {
	fromPath := flag.String("from", "", "current validated config path")
	toPath := flag.String("to", "", "desired validated config path")
	format := flag.String("format", "text", "output format: text or json")
	failOnDiff := flag.Bool("fail-on-diff", false, "exit with status 2 when differences exist")
	flag.Parse()

	if *fromPath == "" || *toPath == "" {
		fmt.Fprintln(os.Stderr, "-from and -to are required")
		os.Exit(1)
	}

	before, err := config.LoadStatic(*fromPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load from config: %v\n", err)
		os.Exit(1)
	}
	after, err := config.LoadStatic(*toPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load to config: %v\n", err)
		os.Exit(1)
	}

	changes := configdiff.Diff(before, after)
	switch *format {
	case "text":
		fmt.Print(configdiff.RenderText(changes))
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(changes); err != nil {
			fmt.Fprintf(os.Stderr, "encode diff: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported format %q\n", *format)
		os.Exit(1)
	}
	if *failOnDiff && len(changes) != 0 {
		os.Exit(2)
	}
}
