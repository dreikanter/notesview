package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePath_Directory(t *testing.T) {
	dir := t.TempDir()
	root, initialFile, err := resolvePath(dir)
	require.NoError(t, err)
	want, _ := filepath.Abs(dir)
	assert.Equal(t, want, root)
	assert.Empty(t, initialFile)
}

func TestResolvePath_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0o644))

	root, initialFile, err := resolvePath(file)
	require.NoError(t, err)
	wantRoot, _ := filepath.Abs(dir)
	assert.Equal(t, wantRoot, root)
	assert.Equal(t, "note.md", initialFile)
}

func TestResolvePath_Nonexistent(t *testing.T) {
	_, _, err := resolvePath("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err)
}

func TestResolvePath_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(target, 0o755))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))

	root, initialFile, err := resolvePath(link)
	require.NoError(t, err)
	// resolvePath uses filepath.Abs, which preserves symlink paths
	wantRoot, _ := filepath.Abs(link)
	assert.Equal(t, wantRoot, root)
	assert.Empty(t, initialFile)
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
			assert.Equal(t, tt.want, got)
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
				assert.Nil(t, cmd)
				return
			}
			require.NotNil(t, cmd)
			assert.Truef(t, filepath.Base(cmd.Path) == tt.wantBin || cmd.Args[0] == tt.wantBin,
				"expected binary %q, got path %q", tt.wantBin, cmd.Path)
		})
	}
}
