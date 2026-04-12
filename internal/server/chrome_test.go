package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildDirBreadcrumbs(t *testing.T) {
	tests := []struct {
		name       string
		sidebarDir string
		wantMode   string
		wantCrumbs []Crumb
	}{
		{
			name:       "empty sidebar dir",
			sidebarDir: "",
			wantMode:   "dir",
			wantCrumbs: nil,
		},
		{
			name:       "single segment",
			sidebarDir: "2026",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "two segments",
			sidebarDir: "2026/03",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Href: "/dir/2026"},
				{Label: "03", Current: true},
			},
		},
		{
			name:       "deeply nested path",
			sidebarDir: "a/b/c/d",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "a", Href: "/dir/a"},
				{Label: "b", Href: "/dir/a/b"},
				{Label: "c", Href: "/dir/a/b/c"},
				{Label: "d", Current: true},
			},
		},
		{
			name:       "trailing slash stripped",
			sidebarDir: "2026/",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "leading slash stripped",
			sidebarDir: "/2026",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "leading and trailing slashes stripped",
			sidebarDir: "/2026/03/",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Href: "/dir/2026"},
				{Label: "03", Current: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDirBreadcrumbs(tt.sidebarDir)
			if got.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", got.Mode, tt.wantMode)
			}
			if len(got.Crumbs) != len(tt.wantCrumbs) {
				t.Fatalf("len(Crumbs) = %d, want %d; got %+v", len(got.Crumbs), len(tt.wantCrumbs), got.Crumbs)
			}
			for i, want := range tt.wantCrumbs {
				g := got.Crumbs[i]
				if g.Label != want.Label || g.Href != want.Href || g.Current != want.Current {
					t.Errorf("Crumbs[%d] = %+v, want %+v", i, g, want)
				}
			}
		})
	}
}

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
		entries, err := readDirEntries(tmp, "notes")
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

	t.Run("directory entries link to /dir/", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes")
		if err != nil {
			t.Fatal(err)
		}
		// "alpha" dir entry should link to /dir/notes/alpha
		alphaEntry := entries[0]
		want := "/dir/notes/alpha"
		if alphaEntry.Href != want {
			t.Errorf("dir href = %q, want %q", alphaEntry.Href, want)
		}
	})

	t.Run("file entries link to /view/", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes")
		if err != nil {
			t.Fatal(err)
		}
		// "apple.md" is the first file entry (index 2)
		appleEntry := entries[2]
		want := "/view/notes/apple.md"
		if appleEntry.Href != want {
			t.Errorf("file href = %q, want %q", appleEntry.Href, want)
		}
	})

	t.Run("empty relPath", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "")
		if err != nil {
			t.Fatal(err)
		}
		// With empty relPath, dir entry name is used directly
		alphaEntry := entries[0]
		wantDirHref := "/dir/alpha"
		if alphaEntry.Href != wantDirHref {
			t.Errorf("dir href = %q, want %q", alphaEntry.Href, wantDirHref)
		}
		// File entry with empty relPath
		appleEntry := entries[2]
		wantFileHref := "/view/apple.md"
		if appleEntry.Href != wantFileHref {
			t.Errorf("file href = %q, want %q", appleEntry.Href, wantFileHref)
		}
	})

	t.Run("nonexistent directory returns error", func(t *testing.T) {
		_, err := readDirEntries(filepath.Join(tmp, "nonexistent"), "")
		if err == nil {
			t.Error("expected error for nonexistent directory")
		}
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		empty := t.TempDir()
		entries, err := readDirEntries(empty, "")
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

		entries, err := readDirEntries(filtered, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("got %d entries, want 0 (all should be filtered)", len(entries))
		}
	})
}
