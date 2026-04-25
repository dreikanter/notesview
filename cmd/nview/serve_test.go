package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_Directory(t *testing.T) {
	dir := t.TempDir()
	root, initialFile, err := resolvePath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := filepath.Abs(dir)
	if root != want {
		t.Errorf("root = %q, want %q", root, want)
	}
	if initialFile != "" {
		t.Errorf("initialFile = %q, want empty", initialFile)
	}
}

func TestResolvePath_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, initialFile, err := resolvePath(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantRoot, _ := filepath.Abs(dir)
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
	if initialFile != "note.md" {
		t.Errorf("initialFile = %q, want %q", initialFile, "note.md")
	}
}

func TestResolvePath_Nonexistent(t *testing.T) {
	_, _, err := resolvePath("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestResolvePath_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	root, initialFile, err := resolvePath(link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// resolvePath uses filepath.Abs, which preserves symlink paths
	wantRoot, _ := filepath.Abs(link)
	if root != wantRoot {
		t.Errorf("root = %q, want %q", root, wantRoot)
	}
	if initialFile != "" {
		t.Errorf("initialFile = %q, want empty", initialFile)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde slash prefix", "~/foo", home + "/foo"},
		{"tilde only", "~", home},
		{"no tilde", "/usr/local", "/usr/local"},
		{"empty string", "", ""},
		{"tilde in middle", "/path/~/foo", "/path/~/foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTilde(tt.path)
			if got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestBrowserCommand(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		url     string
		wantNil bool
		wantBin string
	}{
		{"darwin", "darwin", "http://localhost:8080", false, "open"},
		{"linux", "linux", "http://localhost:8080", false, "xdg-open"},
		{"windows", "windows", "http://localhost:8080", false, "rundll32"},
		{"unknown", "freebsd", "http://localhost:8080", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := browserCommand(tt.goos, tt.url)
			if tt.wantNil {
				if cmd != nil {
					t.Errorf("expected nil command for GOOS=%q", tt.goos)
				}
				return
			}
			if cmd == nil {
				t.Fatalf("expected non-nil command for GOOS=%q", tt.goos)
			}
			if filepath.Base(cmd.Path) != tt.wantBin && cmd.Args[0] != tt.wantBin {
				t.Errorf("expected binary %q, got path %q", tt.wantBin, cmd.Path)
			}
		})
	}
}
