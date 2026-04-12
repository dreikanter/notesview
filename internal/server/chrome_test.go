package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildBreadcrumbs(t *testing.T) {
	tests := []struct {
		name       string
		sidebarDir string
		notePath   string
		wantHome   string
		wantCrumbs []Crumb
	}{
		{
			name:       "empty sidebar dir",
			sidebarDir: "",
			notePath:   "2026/03/hello.md",
			wantHome:   "/view/2026/03/hello.md",
			wantCrumbs: nil,
		},
		{
			name:       "single segment",
			sidebarDir: "2026",
			notePath:   "2026/03/hello.md",
			wantHome:   "/view/2026/03/hello.md",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "two segments",
			sidebarDir: "2026/03",
			notePath:   "2026/03/hello.md",
			wantHome:   "/view/2026/03/hello.md",
			wantCrumbs: []Crumb{
				{Label: "2026", Href: "/view/2026/03/hello.md?dir=2026"},
				{Label: "03", Current: true},
			},
		},
		{
			name:       "deeply nested path",
			sidebarDir: "a/b/c/d",
			notePath:   "note.md",
			wantHome:   "/view/note.md",
			wantCrumbs: []Crumb{
				{Label: "a", Href: "/view/note.md?dir=a"},
				{Label: "b", Href: "/view/note.md?dir=a%2Fb"},
				{Label: "c", Href: "/view/note.md?dir=a%2Fb%2Fc"},
				{Label: "d", Current: true},
			},
		},
		{
			name:       "trailing slash stripped",
			sidebarDir: "2026/",
			notePath:   "note.md",
			wantHome:   "/view/note.md",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "leading slash stripped",
			sidebarDir: "/2026",
			notePath:   "note.md",
			wantHome:   "/view/note.md",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
			},
		},
		{
			name:       "leading and trailing slashes stripped",
			sidebarDir: "/2026/03/",
			notePath:   "note.md",
			wantHome:   "/view/note.md",
			wantCrumbs: []Crumb{
				{Label: "2026", Href: "/view/note.md?dir=2026"},
				{Label: "03", Current: true},
			},
		},
		{
			name:       "no note path",
			sidebarDir: "2026/03",
			notePath:   "",
			wantHome:   "/",
			wantCrumbs: []Crumb{
				{Label: "2026", Href: "/?dir=2026"},
				{Label: "03", Current: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBreadcrumbs(tt.sidebarDir, tt.notePath)
			if got.HomeHref != tt.wantHome {
				t.Errorf("HomeHref = %q, want %q", got.HomeHref, tt.wantHome)
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
		entries, err := readDirEntries(tmp, "notes", "current.md")
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

	t.Run("directory entries use dirLinkHref", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes", "current.md")
		if err != nil {
			t.Fatal(err)
		}
		// "alpha" dir entry should link via dirLinkHref
		alphaEntry := entries[0]
		want := "/view/current.md?dir=notes%2Falpha"
		if alphaEntry.Href != want {
			t.Errorf("dir href = %q, want %q", alphaEntry.Href, want)
		}
	})

	t.Run("file entries use fileLinkHref", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "notes", "current.md")
		if err != nil {
			t.Fatal(err)
		}
		// "apple.md" is the first file entry (index 2)
		appleEntry := entries[2]
		want := "/view/notes/apple.md?dir=notes"
		if appleEntry.Href != want {
			t.Errorf("file href = %q, want %q", appleEntry.Href, want)
		}
	})

	t.Run("empty relPath", func(t *testing.T) {
		entries, err := readDirEntries(tmp, "", "current.md")
		if err != nil {
			t.Fatal(err)
		}
		// With empty relPath, dir entry name is used directly
		alphaEntry := entries[0]
		wantDirHref := "/view/current.md?dir=alpha"
		if alphaEntry.Href != wantDirHref {
			t.Errorf("dir href = %q, want %q", alphaEntry.Href, wantDirHref)
		}
		// File entry with empty relPath: no dir query
		appleEntry := entries[2]
		wantFileHref := "/view/apple.md"
		if appleEntry.Href != wantFileHref {
			t.Errorf("file href = %q, want %q", appleEntry.Href, wantFileHref)
		}
	})

	t.Run("nonexistent directory returns error", func(t *testing.T) {
		_, err := readDirEntries(filepath.Join(tmp, "nonexistent"), "", "")
		if err == nil {
			t.Error("expected error for nonexistent directory")
		}
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		empty := t.TempDir()
		entries, err := readDirEntries(empty, "", "")
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

		entries, err := readDirEntries(filtered, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("got %d entries, want 0 (all should be filtered)", len(entries))
		}
	})
}
