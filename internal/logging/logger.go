package logging

import (
	"log/slog"
	"os"
)

func NewLogger(level slog.Leveler) *slog.Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler)
}
