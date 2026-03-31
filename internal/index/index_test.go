package index

import (
	"os"
	"path/filepath"
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

func TestIndexBuild(t *testing.T) {
	dir := setupTestDir(t)
	idx := New(dir)
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

func TestIsUID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"20260331_9201", true},
		{"20261231_0001", true},
		{"2026031_9201", false},
		{"20260331_", false},
		{"20260331_abc", false},
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
