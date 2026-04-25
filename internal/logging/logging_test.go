package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDefaults(t *testing.T) {
	ctx := context.Background()
	logger, closer, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if closer != nil {
		t.Errorf("expected nil closer when no file configured")
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if !logger.Enabled(ctx, slog.LevelInfo) {
		t.Errorf("info level should be enabled by default")
	}
	if logger.Enabled(ctx, slog.LevelDebug) {
		t.Errorf("debug level should not be enabled by default")
	}
}

func TestNewLevels(t *testing.T) {
	ctx := context.Background()
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"DEBUG":   slog.LevelDebug,
	}
	for in, want := range cases {
		logger, _, err := New(Config{Level: in})
		if err != nil {
			t.Errorf("New(%q): %v", in, err)
			continue
		}
		if !logger.Enabled(ctx, want) {
			t.Errorf("level %q: want %v enabled", in, want)
		}
	}
}

func TestNewInvalidLevel(t *testing.T) {
	_, _, err := New(Config{Level: "loud"})
	if err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestNewInvalidFormat(t *testing.T) {
	_, _, err := New(Config{Format: "yaml"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestNewWithFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "nview.log")

	logger, closer, err := New(Config{File: path, Format: "json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if closer == nil {
		t.Fatal("expected non-nil closer when file configured")
	}
	defer func() { _ = closer.Close() }()

	logger.Info("hello", "who", "world")

	// The file should have been created alongside any missing parent dirs
	// and should contain the JSON-encoded record.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"msg":"hello"`) || !strings.Contains(s, `"who":"world"`) {
		t.Errorf("log file content = %q, want JSON record with msg/who fields", s)
	}
}

func TestDiscardLogger(t *testing.T) {
	logger := Discard()
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	// Error is the lowest level we allow through, but output goes to io.Discard,
	// so this should not panic or produce visible output.
	logger.Error("dropped")
}
