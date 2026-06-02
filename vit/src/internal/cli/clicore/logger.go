
package clicore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

const logDisabled = slog.LevelError + 1

type customLogHandler struct {
	w         io.Writer
	level     slog.Level
	showLevel bool // Whether to show the level prefix (info / error / debug)
}

func (h *customLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *customLogHandler) Handle(_ context.Context, r slog.Record) error {
	if h.showLevel {
		// Verbose mode: show padded level
		level := strings.ToUpper(r.Level.String())
		level = fmt.Sprintf("%-5s", level) // "INFO ", "ERROR", "DEBUG"
		fmt.Fprintf(h.w, "%s %s", level, r.Message)
	} else {
		// Normal mode: just the message
		fmt.Fprint(h.w, r.Message)
	}

	// Add attributes if any
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(h.w, " %s=%v", a.Key, a.Value)
		return true
	})

	fmt.Fprintln(h.w)
	return nil
}

func (h *customLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *customLogHandler) WithGroup(name string) slog.Handler {
	return h
}

func newCliLogger(opt OutputOpt) *slog.Logger {
	level := logDisabled
	if opt.Debug {
		level = slog.LevelDebug
	} else if opt.Verbose {
		level = slog.LevelInfo
	}

	var handler slog.Handler

	if opt.JSON {
		if level > slog.LevelInfo {
			level = slog.LevelInfo
		}
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else if opt.Debug {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		})
	} else {
		handler = &customLogHandler{
			w:         os.Stderr,
			level:     level,
			showLevel: opt.Verbose,
		}
	}
	return slog.New(handler)
}
