package index

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.MkdirAll(filepath.Join(dir, "2026", "01"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260330_9198.md"), []byte("# Note"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "01", "20260102_8814_report.md"), []byte("# Report"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Readme"), 0o644)
	return dir
}

func TestLegacyIndexBuild(t *testing.T) {
	dir := setupTestDir(t)
	idx := NewLegacy(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	tests := []struct {
		uid  string
		want string
		ok   bool
	}{
		{"20260331_9201", "2026/03/20260331_9201_todo.md", true},
		{"20260330_9198", "2026/03/20260330_9198.md", true},
		{"20260102_8814", "2026/01/20260102_8814_report.md", true},
		{"99999999_0000", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.uid, func(t *testing.T) {
			got, ok := idx.Lookup(tt.uid)
			if ok != tt.ok {
				t.Errorf("Lookup(%q) ok = %v, want %v", tt.uid, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("Lookup(%q) = %q, want %q", tt.uid, got, tt.want)
			}
		})
	}
}

func TestLegacyBuildSkipsUnreadableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}

	dir := setupTestDir(t)

	// Create a subdirectory that is not readable.
	unreadable := filepath.Join(dir, "2026", "secret")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unreadable, "20260401_0001.md"), []byte("# Secret"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	idx := NewLegacy(dir, logger)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}

	// Files in readable dirs should still be indexed.
	if _, ok := idx.Lookup("20260331_9201"); !ok {
		t.Error("expected 20260331_9201 to be indexed")
	}

	// File in the unreadable dir should be skipped.
	if _, ok := idx.Lookup("20260401_0001"); ok {
		t.Error("expected 20260401_0001 to be skipped")
	}

	// A warning should have been logged.
	if !strings.Contains(buf.String(), "permission denied") {
		t.Errorf("expected permission denied warning in log, got: %s", buf.String())
	}
}

func TestLegacyBuildReturnsNonPermissionError(t *testing.T) {
	idx := NewLegacy("/nonexistent-root-path-that-does-not-exist", nil)
	if err := idx.Build(); err == nil {
		t.Fatal("expected Build to return an error for nonexistent root")
	}
}

func TestIsUID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		// Classic 4-digit year
		{"20260331_9201", true},
		{"20261231_0001", true},
		// Variable-width year (5+ digits before "_")
		{"12026_0001", true}, // 1-digit year ("1"), month 20, day 26 — still valid as UID pattern
		{"12345_0001", true}, // 1-digit year, month 23, day 45 — UID pattern only, date validity checked elsewhere
		// Too few digits before "_"
		{"2026_0001", false}, // only 4 digits
		{"1234_0001", false},
		// Missing or malformed suffix
		{"20260331_", false},
		{"20260331_abc", false},
		// Not a UID shape at all
		{"hello_world", false},
		{"202603319201", false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := IsUID(tt.s); got != tt.want {
				t.Errorf("IsUID(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
