package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestInitLogger(t *testing.T) {
	logger := InitLogger(slog.LevelInfo)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestInitLoggerWithWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := InitLoggerWithWriter(&buf, slog.LevelInfo)

	logger.Info("test message", "key", "value")

	output := buf.String()
	if output == "" {
		t.Fatal("expected log output")
	}

	// Verify JSON output.
	var logEntry map[string]any
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("expected valid JSON log output, got: %s", output)
	}

	if logEntry["msg"] != "test message" {
		t.Errorf("expected msg 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("expected key=value, got %v", logEntry["key"])
	}
}

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := InitLoggerWithWriter(&buf, slog.LevelWarn)

	logger.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("info message should be filtered at warn level")
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("warn message should not be filtered")
	}
}
