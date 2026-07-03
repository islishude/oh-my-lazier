package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// DefaultLevelName is the minimum log level used by worker entrypoints.
const DefaultLevelName = "info"

// ParseLevel parses a CLI log level value.
func ParseLevel(value string) (slog.Level, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q (want debug, info, warn, or error)", value)
	}
}

// NewTextLogger builds a slog text logger that filters below level.
func NewTextLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// NewJSONLogger builds a slog JSON logger that filters below level.
func NewJSONLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
