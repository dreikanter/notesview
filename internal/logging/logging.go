// Package logging builds structured loggers (log/slog) for notesview from a
// small Config value. The goal is one place where log sinks, level, and
// format are decided, so the rest of the codebase just takes a *slog.Logger.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config captures the user-facing logging knobs. Zero value is valid and
// produces an info-level text logger writing to stdout.
type Config struct {
	// Level is one of "debug", "info", "warn", "error". Empty means "info".
	Level string
	// Format is "text" or "json". Empty means "text".
	Format string
	// File, if non-empty, is an additional log sink opened for append.
	// Logs are always written to stdout as well.
	File string
}

// New builds a *slog.Logger from cfg. When cfg.File is set, it is opened for
// append (and created if missing, along with any parent dirs) and logs are
// fanned out to both stdout and the file via io.MultiWriter. The returned
// io.Closer closes the file sink (nil when no file was opened); callers
// should close it at shutdown.
func New(cfg Config) (*slog.Logger, io.Closer, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, nil, err
	}

	writers := []io.Writer{os.Stdout}
	var closer io.Closer
	if cfg.File != "" {
		if dir := filepath.Dir(cfg.File); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("create log dir: %w", err)
			}
		}
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("open log file: %w", err)
		}
		writers = append(writers, f)
		closer = f
	}

	handler, err := newHandler(io.MultiWriter(writers...), cfg.Format, level)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, nil, err
	}
	return slog.New(handler), closer, nil
}

// Discard returns a logger that drops every record. Useful as a default for
// tests and embedded consumers that don't want log output.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newHandler(w io.Writer, format string, level slog.Level) (slog.Handler, error) {
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(format) {
	case "", "text":
		return slog.NewTextHandler(w, opts), nil
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("unknown log format %q (want text or json)", format)
	}
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (want debug, info, warn, or error)", s)
	}
}
