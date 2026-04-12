package index

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeFile mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func setupTagFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Inline tag list format
	writeFile(t, filepath.Join(dir, "note_golang_web.md"), `---
title: Golang Web
tags: [golang, web]
---
Content here.
`)

	// Block list format
	writeFile(t, filepath.Join(dir, "note_golang_testing.md"), `---
title: Golang Testing
tags:
  - golang
  - testing
---
Content here.
`)

	// Missing tags field
	writeFile(t, filepath.Join(dir, "note_no_tags.md"), `---
title: No Tags
---
No tags here.
`)

	// Empty tags
	writeFile(t, filepath.Join(dir, "note_empty_tags.md"), `---
title: Empty Tags
tags: []
---
Empty tags.
`)

	// Non-md file — should be skipped
	writeFile(t, filepath.Join(dir, "readme.txt"), "not a markdown file")

	return dir
}

func TestTagIndexBuild(t *testing.T) {
	dir := setupTagFixtures(t)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	tags := idx.Tags()
	want := []string{"golang", "testing", "web"}
	if len(tags) != len(want) {
		t.Fatalf("Tags() = %v, want %v", tags, want)
	}
	for i, tag := range tags {
		if tag != want[i] {
			t.Errorf("Tags()[%d] = %q, want %q", i, tag, want[i])
		}
	}
}

func TestTagIndexNotesByTag(t *testing.T) {
	dir := setupTagFixtures(t)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	golangNotes := idx.NotesByTag("golang")
	if len(golangNotes) != 2 {
		t.Fatalf("NotesByTag(golang) = %v, want 2 entries", golangNotes)
	}

	webNotes := idx.NotesByTag("web")
	if len(webNotes) != 1 {
		t.Fatalf("NotesByTag(web) = %v, want 1 entry", webNotes)
	}

	testingNotes := idx.NotesByTag("testing")
	if len(testingNotes) != 1 {
		t.Fatalf("NotesByTag(testing) = %v, want 1 entry", testingNotes)
	}

	// Nonexistent tag should return empty (not nil)
	none := idx.NotesByTag("nonexistent")
	if none == nil {
		t.Error("NotesByTag(nonexistent) = nil, want empty slice")
	}
	if len(none) != 0 {
		t.Errorf("NotesByTag(nonexistent) = %v, want empty", none)
	}
}

func TestTagIndexDuplicateTags(t *testing.T) {
	dir := t.TempDir()
	// File with "go" tag listed twice (within-file duplicate)
	writeFile(t, filepath.Join(dir, "note_dups.md"), `---
title: Dups
tags: [go, go]
---
`)
	// Two files sharing the "go" tag (cross-file scenario)
	writeFile(t, filepath.Join(dir, "note_go_1.md"), `---
title: Go File 1
tags: [go]
---
`)
	writeFile(t, filepath.Join(dir, "note_go_2.md"), `---
title: Go File 2
tags: [go]
---
`)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Tags() should return ["go"]
	tags := idx.Tags()
	if len(tags) != 1 || tags[0] != "go" {
		t.Errorf("Tags() = %v, want [\"go\"]", tags)
	}

	// NotesByTag("go") should return exactly 2 entries (one per file, not duplicated)
	notes := idx.NotesByTag("go")
	if len(notes) != 3 {
		t.Errorf("NotesByTag(go) = %v, want 3 entries (1 file with duplicate + 2 files with go)", notes)
	}
}

func TestTagIndexBlockListFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "note_block.md"), `---
title: Block List
tags:
  - alpha
  - beta
---
`)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	tags := idx.Tags()
	want := []string{"alpha", "beta"}
	if len(tags) != len(want) {
		t.Fatalf("Tags() = %v, want %v", tags, want)
	}
	for i, tag := range tags {
		if tag != want[i] {
			t.Errorf("Tags()[%d] = %q, want %q", i, tag, want[i])
		}
	}
}
