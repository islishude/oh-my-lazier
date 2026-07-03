package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  slog.Level
	}{
		{name: "debug", value: "debug", want: slog.LevelDebug},
		{name: "info", value: "info", want: slog.LevelInfo},
		{name: "warn", value: "warn", want: slog.LevelWarn},
		{name: "warning alias", value: "warning", want: slog.LevelWarn},
		{name: "error", value: "error", want: slog.LevelError},
		{name: "trim uppercase", value: " DEBUG ", want: slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.value)
			if err != nil {
				t.Fatalf("ParseLevel() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	if _, err := ParseLevel("trace"); err == nil {
		t.Fatal("ParseLevel() error = nil, want error")
	}
}

func TestNewTextLoggerFiltersByLevel(t *testing.T) {
	var logs bytes.Buffer
	logger := NewTextLogger(&logs, slog.LevelInfo)

	logger.Debug("hidden")
	logger.Info("visible")

	output := logs.String()
	if strings.Contains(output, "hidden") {
		t.Fatalf("debug log was emitted at info level: %s", output)
	}
	if !strings.Contains(output, "visible") {
		t.Fatalf("info log missing: %s", output)
	}
}

func TestNewJSONLoggerFiltersByLevel(t *testing.T) {
	var logs bytes.Buffer
	logger := NewJSONLogger(&logs, slog.LevelDebug)

	logger.Debug("visible")

	output := logs.String()
	if !strings.Contains(output, `"level":"DEBUG"`) || !strings.Contains(output, `"msg":"visible"`) {
		t.Fatalf("debug json log missing fields: %s", output)
	}
}
