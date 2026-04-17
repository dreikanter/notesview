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

func setupNoteIndexDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "2026", "03"))
	mustMkdirAll(t, filepath.Join(dir, "2026", "01"))
	mustWriteFile(t, filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), "# Todo\n")
	mustWriteFile(t, filepath.Join(dir, "2026", "03", "20260330_9198.md"), "# Note\n")
	mustWriteFile(t, filepath.Join(dir, "2026", "01", "20260102_8814_report.md"), "# Report\n")
	// File with a variable-width-year UID
	mustWriteFile(t, filepath.Join(dir, "12026_0001.md"), "# Future\n")
	// Non-UID file — indexed as an entry with UID="" but not reachable via NoteByUID
	mustWriteFile(t, filepath.Join(dir, "README.md"), "# Readme\n")
	return dir
}

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func TestNoteByUID(t *testing.T) {
	dir := setupNoteIndexDir(t)
	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	cases := []struct {
		uid     string
		wantRel string
		wantOK  bool
	}{
		{"20260331_9201", "2026/03/20260331_9201_todo.md", true},
		{"20260330_9198", "2026/03/20260330_9198.md", true},
		{"20260102_8814", "2026/01/20260102_8814_report.md", true},
		{"12026_0001", "12026_0001.md", true},
		{"99999999_0000", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.uid, func(t *testing.T) {
			got, ok := idx.NoteByUID(tt.uid)
			if ok != tt.wantOK {
				t.Errorf("NoteByUID(%q) ok = %v, want %v", tt.uid, ok, tt.wantOK)
			}
			if ok && got != tt.wantRel {
				t.Errorf("NoteByUID(%q) = %q, want %q", tt.uid, got, tt.wantRel)
			}
		})
	}
}

func TestNoteByUIDUsesForwardSlashes(t *testing.T) {
	dir := setupNoteIndexDir(t)
	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.NoteByUID("20260331_9201")
	if !ok {
		t.Fatal("expected UID to be found")
	}
	if strings.Contains(got, "\\") {
		t.Errorf("rel-path %q contains backslash; expected forward slashes only", got)
	}
}

func TestBuildReturnsNonPermissionError(t *testing.T) {
	idx := New("/nonexistent-root-path-that-does-not-exist", nil)
	if err := idx.Build(); err == nil {
		t.Fatal("expected Build to return an error for nonexistent root")
	}
}

func TestBuildSkipsUnreadableDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}

	dir := setupNoteIndexDir(t)
	unreadable := filepath.Join(dir, "2026", "secret")
	mustMkdirAll(t, unreadable)
	mustWriteFile(t, filepath.Join(unreadable, "20260401_0001.md"), "# Secret\n")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	idx := New(dir, logger)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if _, ok := idx.NoteByUID("20260331_9201"); !ok {
		t.Error("expected 20260331_9201 to be indexed")
	}
	if _, ok := idx.NoteByUID("20260401_0001"); ok {
		t.Error("expected 20260401_0001 to be skipped")
	}
	if !strings.Contains(buf.String(), "permission denied") {
		t.Errorf("expected permission denied warning in log, got: %s", buf.String())
	}
}
