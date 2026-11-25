package clicore

import (
	"context"
	"log/slog"
	"testing"
)

func TestCliLoggerShowsWarningsByDefault(t *testing.T) {
	logger := newCliLogger(OutputOpt{})
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("warnings hidden by default")
	}
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("info shown by default")
	}
}
