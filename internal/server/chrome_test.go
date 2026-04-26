package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDirEntries(t *testing.T) {
	// Set up a temp directory tree:
	//   notes/
	//     .hidden/
	//     alpha/
	//     beta/
	//     image.png
	//     .secret.md
	//     banana.md
	//     apple.md
	tmp := t.TempDir()

	dirs := []string{".hidden", "alpha", "beta"}
	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(tmp, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"image.png", ".secret.md", "banana.md", "apple.md"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmp, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("filters and sorts correctly", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantNames := []string{"alpha", "beta", "apple.md", "banana.md"}
		if len(entries) != len(wantNames) {
			t.Fatalf("got %d entries, want %d: %+v", len(entries), len(wantNames), entries)
		}
		for i, want := range wantNames {
			if entries[i].Name != want {
				t.Errorf("entries[%d].Name = %q, want %q", i, entries[i].Name, want)
			}
		}

		// Directories first
		if !entries[0].IsDir || !entries[1].IsDir {
			t.Error("first two entries should be directories")
		}
		// Files after
		if entries[2].IsDir || entries[3].IsDir {
			t.Error("last two entries should be files")
		}
	})

	t.Run("directory entries have no href", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir && e.Href != "" {
				t.Errorf("dir %q should have no href, got %q", e.Name, e.Href)
			}
		}
	})

	t.Run("file entries without index have no href", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !e.IsDir && e.Href != "" {
				t.Errorf("file %q without index should have no href, got %q", e.Name, e.Href)
			}
		}
	})

	t.Run("nonexistent directory returns error", func(t *testing.T) {
		_, err := readDirEntries(filepath.Join(tmp, "nonexistent"), "", nil)
		if err == nil {
			t.Error("expected error for nonexistent directory")
		}
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		empty := t.TempDir()
		entries, err := readDirEntries(empty, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("got %d entries, want 0", len(entries))
		}
	})

	t.Run("directory with only dotfiles and non-md files", func(t *testing.T) {
		filtered := t.TempDir()
		os.WriteFile(filepath.Join(filtered, ".hidden"), nil, 0o644)
		os.WriteFile(filepath.Join(filtered, "readme.txt"), nil, 0o644)
		os.Mkdir(filepath.Join(filtered, ".dotdir"), 0o755)

		entries, err := readDirEntries(filtered, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("got %d entries, want 0 (all should be filtered)", len(entries))
		}
	})
}
