package index

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func setupTagFixtures(t *testing.T) string {
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

func TestTagsSorted(t *testing.T) {
	dir := setupTagFixtures(t)
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

func TestNotesByTag(t *testing.T) {
	dir := setupTagFixtures(t)
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

func TestNotesByTagDeduplicatesWithinFile(t *testing.T) {
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

func TestNotesByTagSortedRelPaths(t *testing.T) {
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

// entryByRel is a test helper that returns the entry at rel, or fails the
// test. Reads unexported state — tests run in package index.
func entryByRel(t *testing.T, idx *NoteIndex, rel string) NoteEntry {
	t.Helper()
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, e := range idx.entries {
		if e.RelPath == rel {
			return e
		}
	}
	t.Fatalf("no entry with RelPath %q; have %d entries", rel, len(idx.entries))
	return NoteEntry{}
}

func TestNoteEntryTitle(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.md"), "---\ntitle: Hello World\n---\n")
	mustWriteFile(t, filepath.Join(dir, "b.md"), "# No frontmatter\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := entryByRel(t, idx, "a.md").Title; got != "Hello World" {
		t.Errorf("Title = %q, want Hello World", got)
	}
	if got := entryByRel(t, idx, "b.md").Title; got != "" {
		t.Errorf("Title = %q, want empty", got)
	}
}

func TestNoteEntryAliases(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "inline.md"),
		"---\naliases: [k8s, kube]\n---\n")
	mustWriteFile(t, filepath.Join(dir, "block.md"),
		"---\naliases:\n  - one\n  - two\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := entryByRel(t, idx, "inline.md").Aliases
	if len(got) != 2 || got[0] != "k8s" || got[1] != "kube" {
		t.Errorf("inline Aliases = %v", got)
	}
	got = entryByRel(t, idx, "block.md").Aliases
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("block Aliases = %v", got)
	}
}

func TestNoteEntrySlugFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.md"),
		"---\nslug: My Awesome_Note\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := entryByRel(t, idx, "a.md").Slug; got != "my-awesome-note" {
		t.Errorf("Slug = %q, want my-awesome-note", got)
	}
}

func TestNoteEntrySlugDerivedFromStem(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "20260331_9201_weekly_digest.md"), "# Body\n")
	mustWriteFile(t, filepath.Join(dir, "20260331_9202.md"), "# Body\n")
	mustWriteFile(t, filepath.Join(dir, "README.md"), "# Readme\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := entryByRel(t, idx, "20260331_9201_weekly_digest.md").Slug; got != "weekly-digest" {
		t.Errorf("Slug = %q, want weekly-digest", got)
	}
	// Bare UID filename → empty slug.
	if got := entryByRel(t, idx, "20260331_9202.md").Slug; got != "" {
		t.Errorf("Slug = %q, want empty", got)
	}
	// Non-UID filename → normalized stem.
	if got := entryByRel(t, idx, "README.md").Slug; got != "readme" {
		t.Errorf("Slug = %q, want readme", got)
	}
}

func TestNoteEntryDateFromUID(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "20260331_9201.md"), "---\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e := entryByRel(t, idx, "20260331_9201.md")
	if e.DateSource != "uid" {
		t.Errorf("DateSource = %q, want uid", e.DateSource)
	}
	if e.Date.Year() != 2026 || e.Date.Month() != 3 || e.Date.Day() != 31 {
		t.Errorf("Date = %v, want 2026-03-31", e.Date)
	}
}

func TestNoteEntryDateFromFrontmatterWhenNoUID(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "README.md"), "---\ndate: 2020-01-02\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e := entryByRel(t, idx, "README.md")
	if e.DateSource != "frontmatter" {
		t.Errorf("DateSource = %q, want frontmatter", e.DateSource)
	}
	if e.Date.Year() != 2020 || e.Date.Month() != 1 || e.Date.Day() != 2 {
		t.Errorf("Date = %v, want 2020-01-02", e.Date)
	}
}

func TestNoteEntryDateFromMtimeWhenNoUIDAndNoFrontmatterDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.md")
	mustWriteFile(t, path, "body only, no frontmatter\n")

	// Stamp mtime to a known instant.
	want := time.Date(2019, 7, 4, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(path, want, want); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e := entryByRel(t, idx, "plain.md")
	if e.DateSource != "mtime" {
		t.Errorf("DateSource = %q, want mtime", e.DateSource)
	}
	if !e.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", e.Date, want)
	}
}

func TestNoteEntryDateUIDInvalidFallsThrough(t *testing.T) {
	// UID digits match the pattern but yield an invalid date: month=99.
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "20269931_0001.md"),
		"---\ndate: 2021-06-06\n---\n")

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e := entryByRel(t, idx, "20269931_0001.md")
	if e.DateSource != "frontmatter" {
		t.Errorf("DateSource = %q, want frontmatter (UID date invalid)", e.DateSource)
	}
}

func TestMalformedFrontmatterDoesNotFailBuild(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "bad.md"),
		"---\ntags: [unterminated\n---\n")
	mustWriteFile(t, filepath.Join(dir, "20260331_9201.md"),
		"---\ntags: [golang]\n---\n")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	idx := New(dir, logger)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build returned error, want nil: %v", err)
	}
	if _, ok := idx.NoteByUID("20260331_9201"); !ok {
		t.Error("sibling UID entry should still be indexed")
	}
	// The malformed file is still recorded as an entry (no UID, no tags).
	e := entryByRel(t, idx, "bad.md")
	if len(e.Tags) != 0 {
		t.Errorf("bad.md tags = %v, want none", e.Tags)
	}
	if !strings.Contains(buf.String(), "frontmatter parse failed") {
		t.Errorf("expected parse-failed warning in log, got: %s", buf.String())
	}
}

func TestUnreadableFileStillIndexedByUID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-mode-based test not reliable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "20260331_9201.md")
	mustWriteFile(t, path, "---\ntags: [go]\n---\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	idx := New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	// UID still comes from the filename; content was unreadable.
	if _, ok := idx.NoteByUID("20260331_9201"); !ok {
		t.Error("UID should still be indexed even when file contents unreadable")
	}
	// Tags should be absent (we couldn't read the frontmatter).
	if got := idx.NotesByTag("go"); len(got) != 0 {
		t.Errorf("NotesByTag(go) = %v, want empty (file unreadable)", got)
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
