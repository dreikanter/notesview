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

func TestParseFrontmatterInlineTags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "---\ntitle: Hello\ntags: [golang, web]\naliases: [\"h\", 'hi']\n---\n# Body\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Title != "Hello" {
		t.Errorf("Title = %q, want %q", fm.Title, "Hello")
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "golang" || fm.Tags[1] != "web" {
		t.Errorf("Tags = %v, want [golang web]", fm.Tags)
	}
	if len(fm.Aliases) != 2 || fm.Aliases[0] != "h" || fm.Aliases[1] != "hi" {
		t.Errorf("Aliases = %v, want [h hi]", fm.Aliases)
	}
}

func TestParseFrontmatterBlockList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "---\ntags:\n  - alpha\n  - beta\naliases:\n  - a1\n---\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "alpha" || fm.Tags[1] != "beta" {
		t.Errorf("Tags = %v", fm.Tags)
	}
	if len(fm.Aliases) != 1 || fm.Aliases[0] != "a1" {
		t.Errorf("Aliases = %v", fm.Aliases)
	}
}

func TestParseFrontmatterMissingFences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "# No frontmatter here\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Title != "" || len(fm.Tags) != 0 || len(fm.Aliases) != 0 {
		t.Errorf("expected zero-value frontmatter, got %+v", fm)
	}
}

func TestParseFrontmatterMalformedYAMLIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "---\ntags: [unterminated\n---\n")

	_, err := parseFrontmatter(path)
	if err == nil {
		t.Fatal("expected YAML parse error, got nil")
	}
}

func TestParseFrontmatterReadError(t *testing.T) {
	_, err := parseFrontmatter("/nonexistent/file/path.md")
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestParseFrontmatterDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "---\ndate: 2026-04-17\n---\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Date.Year() != 2026 || fm.Date.Month() != 4 || fm.Date.Day() != 17 {
		t.Errorf("Date = %v, want 2026-04-17", fm.Date)
	}
}

func TestParseFrontmatterEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Title != "" || len(fm.Tags) != 0 {
		t.Errorf("expected zero-value frontmatter, got %+v", fm)
	}
}

func TestParseFrontmatterEmptyBetweenFences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	mustWriteFile(t, path, "---\n---\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm.Title != "" || len(fm.Tags) != 0 {
		t.Errorf("expected zero-value frontmatter, got %+v", fm)
	}
}

func TestParseFrontmatterIndentedTripleDashIsNotFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	// Opening fence is valid. The body contains an indented "  ---"
	// line inside a multi-line YAML value; it must NOT be treated as
	// the closing fence.
	mustWriteFile(t, path, "---\ndescription: |\n  ---\n  second line\ntags: [x]\n---\n")

	fm, err := parseFrontmatter(path)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if len(fm.Tags) != 1 || fm.Tags[0] != "x" {
		t.Errorf("Tags = %v, want [x]", fm.Tags)
	}
}

func setupNoteIndexTagFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "note_golang_web.md"),
		"---\ntitle: Golang Web\ntags: [golang, web]\n---\nContent.\n")
	mustWriteFile(t, filepath.Join(dir, "note_golang_testing.md"),
		"---\ntitle: Golang Testing\ntags:\n  - golang\n  - testing\n---\nContent.\n")
	mustWriteFile(t, filepath.Join(dir, "note_no_tags.md"),
		"---\ntitle: No Tags\n---\n")
	mustWriteFile(t, filepath.Join(dir, "note_empty_tags.md"),
		"---\ntitle: Empty Tags\ntags: []\n---\n")
	mustWriteFile(t, filepath.Join(dir, "readme.txt"),
		"not markdown")
	return dir
}

func TestNoteIndexTagsSorted(t *testing.T) {
	dir := setupNoteIndexTagFixtures(t)
	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.Tags()
	want := []string{"golang", "testing", "web"}
	if len(got) != len(want) {
		t.Fatalf("Tags() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Tags()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNoteIndexNotesByTag(t *testing.T) {
	dir := setupNoteIndexTagFixtures(t)
	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := idx.NotesByTag("golang"); len(got) != 2 {
		t.Errorf("NotesByTag(golang) = %v, want 2 entries", got)
	}
	if got := idx.NotesByTag("web"); len(got) != 1 {
		t.Errorf("NotesByTag(web) = %v, want 1 entry", got)
	}
	none := idx.NotesByTag("nonexistent")
	if none == nil {
		t.Error("NotesByTag(nonexistent) = nil, want non-nil empty slice")
	}
	if len(none) != 0 {
		t.Errorf("NotesByTag(nonexistent) = %v, want empty", none)
	}
}

func TestNoteIndexNotesByTagDeduplicatesWithinFile(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "dups.md"),
		"---\ntags: [go, go]\n---\n")
	mustWriteFile(t, filepath.Join(dir, "one.md"),
		"---\ntags: [go]\n---\n")
	mustWriteFile(t, filepath.Join(dir, "two.md"),
		"---\ntags: [go]\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	tags := idx.Tags()
	if len(tags) != 1 || tags[0] != "go" {
		t.Errorf("Tags() = %v, want [go]", tags)
	}
	// dups.md contributes exactly one "go" slot (within-file dedup);
	// one.md and two.md each contribute one → total 3.
	notes := idx.NotesByTag("go")
	if len(notes) != 3 {
		t.Errorf("NotesByTag(go) = %v, want 3 entries", notes)
	}
}

func TestNoteIndexNotesByTagSortedRelPaths(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "b"))
	mustMkdirAll(t, filepath.Join(dir, "a"))
	mustWriteFile(t, filepath.Join(dir, "b", "note.md"), "---\ntags: [t]\n---\n")
	mustWriteFile(t, filepath.Join(dir, "a", "note.md"), "---\ntags: [t]\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	notes := idx.NotesByTag("t")
	if len(notes) != 2 || notes[0] != "a/note.md" || notes[1] != "b/note.md" {
		t.Errorf("NotesByTag(t) = %v, want [a/note.md b/note.md]", notes)
	}
}
