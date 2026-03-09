package observability

import (
	"io"
	"log/slog"
	"os"
)

// InitLogger configures the global slog logger with JSON output.
// Returns the configured logger for explicit use.
func InitLogger(level slog.Level) *slog.Logger {
	return InitLoggerWithWriter(os.Stderr, level)
}

// InitLoggerWithWriter configures the global slog logger writing to the given writer.
func InitLoggerWithWriter(w io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
