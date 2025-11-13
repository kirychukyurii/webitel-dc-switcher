package logger

import (
	"log/slog"
	"os"
)

// New creates a new structured logger using slog
func New() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

// NewWithLevel creates a new logger with specified log level
func NewWithLevel(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}
