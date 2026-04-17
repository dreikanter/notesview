# NoteIndex Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge `Index` and `TagIndex` into a single `NoteIndex` that walks the notes tree once, parses frontmatter once per file, and exposes today's UID + tag lookups; populate a rich per-file `NoteEntry` to enable future derived maps without a second walk.

**Architecture:** New type `NoteIndex` in `internal/index/note_index.go` with a single `filepath.WalkDir`, a `yaml.v2`-backed frontmatter parser, one `sync.RWMutex` plus a `building` `TryLock`. All rel-paths normalized to forward slashes. Old `Index` and `TagIndex` kept intact through most of the plan so each commit is green; deleted at the end along with their tests. Callers updated one package at a time.

**Tech Stack:** Go 1.22+, `gopkg.in/yaml.v2` (promoted from indirect to direct), `log/slog`, existing `internal/logging` package.

**Spec:** `docs/superpowers/specs/2026-04-17-note-index-unification-design.md`

References #64.

---

### Task 1: Relax the UID regex

The spec generalizes the UID format from fixed 4-digit year to variable-width year (`^\d{5,}_\d+$`, year = all digits before the trailing `MMDD`). This widening is a safe preliminary step that touches only the existing package-level regex and the existing `TestIsUID` — no downstream caller behavior changes.

**Files:**
- Modify: `internal/index/index.go:16-18` (regex declarations)
- Modify: `internal/index/index_test.go:107-127` (`TestIsUID`)

- [ ] **Step 1: Update `TestIsUID` to cover variable-width-year cases**

Replace `TestIsUID` in `internal/index/index_test.go` with:

```go
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
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `cd internal/index && go test -run TestIsUID -v`
Expected: FAIL — `"12026_0001"` returns false under the current `^\d{8}_\d+$`.

- [ ] **Step 3: Relax the regexes in `internal/index/index.go`**

Replace lines 16–18:

```go
var uidPattern = regexp.MustCompile(`^(\d{5,}_\d+)`)

var fullUIDPattern = regexp.MustCompile(`^\d{5,}_\d+$`)
```

- [ ] **Step 4: Run tests to confirm they pass**

Run: `cd internal/index && go test -v`
Expected: all tests pass, including the new variable-year cases.

- [ ] **Step 5: Commit**

```bash
git add internal/index/index.go internal/index/index_test.go
git commit -m "Relax UID regex to accept variable-width years"
```

---

### Task 2: Scaffold `NoteIndex` with UID-only `Build`, `NoteByUID`, and `Rebuild`

Create the new type with the minimum surface needed to replace `Index`: UID lookup and async rebuild. Frontmatter / tags come in later tasks. Old `Index` / `TagIndex` remain untouched.

**Files:**
- Create: `internal/index/note_index.go`
- Create: `internal/index/note_index_test.go`
- Modify: `internal/index/index.go` (rename `New` → `NewLegacy`)
- Modify: `internal/index/index_test.go` (rename colliding tests, update `New` calls)
- Modify: `internal/server/server.go` (call `NewLegacy`)
- Modify: `internal/renderer/noteext_test.go` (call `NewLegacy`)

This task has an unusual ordering because the new file introduces a `New` constructor and two test names that collide with symbols already declared in the existing `index.go` / `index_test.go`. Those renames happen **first**, so every intermediate step compiles.

- [ ] **Step 1: Rename the old constructor and colliding tests to `*Legacy`**

In `internal/index/index.go`, line 32:

```go
func NewLegacy(root string, logger *slog.Logger) *Index {
```

In `internal/index/index_test.go`:

- Line 25: `func TestLegacyIndexBuild(t *testing.T) {`
- Line 56: `func TestLegacyBuildSkipsUnreadableDirs(t *testing.T) {`
- Line 100: `func TestLegacyBuildReturnsNonPermissionError(t *testing.T) {`
- Lines 27, 79, 101 — update the `New(...)` calls inside those functions to `NewLegacy(...)`.

`TestIsUID` (line 107) does **not** collide with anything the new file declares — leave it as-is.

In `internal/server/server.go`, line 35:

```go
	idx := index.NewLegacy(root, logger)
```

In `internal/renderer/noteext_test.go`, line 72:

```go
	idx := index.NewLegacy(dir, nil)
```

Run: `go build ./... && go test ./...`
Expected: all green. This step is a pure rename; no behavior change.

- [ ] **Step 2: Write failing tests for the new surface**

Create `internal/index/note_index_test.go`:

```go
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
```

- [ ] **Step 3: Run the new tests to confirm they fail to compile**

Run: `cd internal/index && go vet ./... 2>&1 | head`
Expected: compilation error — `undefined: New` when the test calls `New(dir, nil)` because we haven't created the new-type constructor yet. (This is the TDD red phase at compile-time.)

- [ ] **Step 4: Implement `NoteIndex` with `Build` (UID-only), `NoteByUID`, and `Rebuild`**

Create `internal/index/note_index.go`:

```go
package index

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/logging"
)

// NoteEntry is the per-file record built during a single walk. Fields not
// needed for today's lookups are populated for future derived maps
// (bySlug, byAlias, byDate) without requiring a second walk.
type NoteEntry struct {
	RelPath    string
	UID        string
	Stem       string
	Slug       string
	Title      string
	Tags       []string
	Aliases    []string
	Date       time.Time
	DateSource string
}

// NoteIndex is the unified in-memory index of the notes tree. It is safe
// for concurrent use. Build performs a single filepath.WalkDir, parses
// frontmatter once per file, and swaps all state in atomically.
type NoteIndex struct {
	root     string
	logger   *slog.Logger
	mu       sync.RWMutex
	entries  []NoteEntry
	byUID    map[string]string
	byTag    map[string][]string
	allTags  []string
	building sync.Mutex
}

// New creates a NoteIndex rooted at root. A nil logger is replaced with
// a discard logger.
func New(root string, logger *slog.Logger) *NoteIndex {
	if logger == nil {
		logger = logging.Discard()
	}
	return &NoteIndex{
		root:   root,
		logger: logger,
		byUID:  make(map[string]string),
		byTag:  make(map[string][]string),
	}
}

// Build walks the notes tree once, reads each .md file, and rebuilds all
// state. The swap at the end is atomic. Non-permission walk errors are
// propagated; permission-denied directories are warned and skipped.
func (i *NoteIndex) Build() error {
	var entries []NoteEntry
	byUID := make(map[string]string)

	err := filepath.WalkDir(i.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				i.logger.Warn("skipping path: permission denied", "path", path)
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		rel, err := filepath.Rel(i.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		stem := strings.TrimSuffix(d.Name(), ".md")
		uid := ""
		if m := uidPattern.FindStringSubmatch(d.Name()); m != nil {
			uid = m[1]
		}

		entry := NoteEntry{
			RelPath: rel,
			UID:     uid,
			Stem:    stem,
		}
		entries = append(entries, entry)

		if uid != "" {
			byUID[uid] = rel
		}
		return nil
	})
	if err != nil {
		return err
	}

	i.mu.Lock()
	i.entries = entries
	i.byUID = byUID
	i.byTag = make(map[string][]string)
	i.allTags = nil
	i.mu.Unlock()
	return nil
}

// Rebuild triggers a background index build, coalescing concurrent calls.
// If a build is already in progress, the call returns immediately.
func (i *NoteIndex) Rebuild() {
	if !i.building.TryLock() {
		return
	}
	go func() {
		defer i.building.Unlock()
		if err := i.Build(); err != nil {
			i.logger.Error("note index rebuild failed", "err", err)
		}
	}()
}

// NoteByUID returns the forward-slash rel-path for a UID and a boolean
// found flag. UIDs are unique.
func (i *NoteIndex) NoteByUID(uid string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	p, ok := i.byUID[uid]
	return p, ok
}
```

- [ ] **Step 5: Run tests to confirm they pass**

Run: `cd internal/index && go test -v`
Expected: all tests pass — legacy tests (`TestLegacyIndexBuild`, `TestLegacyBuildSkipsUnreadableDirs`, `TestLegacyBuildReturnsNonPermissionError`, `TestIsUID`) and the new `TestNoteByUID`, `TestNoteByUIDUsesForwardSlashes`, `TestBuildReturnsNonPermissionError`, `TestBuildSkipsUnreadableDirs`.

Run: `go test ./...`
Expected: all packages green.

- [ ] **Step 6: Commit**

```bash
git add internal/index/note_index.go internal/index/note_index_test.go internal/index/index.go internal/index/index_test.go internal/server/server.go internal/renderer/noteext_test.go
git commit -m "Add NoteIndex with UID-only Build, NoteByUID, Rebuild"
```

---

### Task 3: Frontmatter parser

Add the YAML-based frontmatter parser as a self-contained helper. Test it in isolation before wiring into `Build`.

**Files:**
- Create: `internal/index/frontmatter.go`
- Modify: `internal/index/note_index_test.go` (add frontmatter helper tests)
- Modify: `go.mod` (promote `gopkg.in/yaml.v2` from indirect to direct)

- [ ] **Step 1: Write failing tests for `parseFrontmatter`**

Append to `internal/index/note_index_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `cd internal/index && go test -run TestParseFrontmatter -v`
Expected: FAIL — `parseFrontmatter` undefined.

- [ ] **Step 3: Implement the frontmatter parser**

Create `internal/index/frontmatter.go`:

```go
package index

import (
	"bufio"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// frontmatter is the typed per-file frontmatter. Fields not carried by a
// given file default to Go zero values. The struct is intentionally
// private: extending it with new fields is a local change.
type frontmatter struct {
	Title   string    `yaml:"title"`
	Tags    []string  `yaml:"tags"`
	Aliases []string  `yaml:"aliases"`
	Date    time.Time `yaml:"date"`
}

// parseFrontmatter reads the file at path, extracts the YAML block between
// the first two `---` fences on their own lines, and unmarshals it. Missing
// fences yield a zero-valued frontmatter and no error. Read errors and
// malformed YAML are returned.
func parseFrontmatter(path string) (frontmatter, error) {
	var fm frontmatter

	f, err := os.Open(path)
	if err != nil {
		return fm, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow very long frontmatter lines (defensive against defaults).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// First non-empty line must be `---`.
	if !scanner.Scan() {
		return fm, scanner.Err()
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return fm, nil
	}

	var body strings.Builder
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			closed = true
			break
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return fm, err
	}
	if !closed {
		// No closing fence — treat as no frontmatter.
		return frontmatter{}, nil
	}

	if body.Len() == 0 {
		return fm, nil
	}

	if err := yaml.Unmarshal([]byte(body.String()), &fm); err != nil {
		return frontmatter{}, err
	}
	return fm, nil
}
```

- [ ] **Step 4: Promote `yaml.v2` from indirect to direct**

Run: `go mod tidy`
Expected: `go.mod` updates to drop the `// indirect` comment on `gopkg.in/yaml.v2 v2.4.0`.

- [ ] **Step 5: Run tests to confirm they pass**

Run: `cd internal/index && go test -run TestParseFrontmatter -v`
Expected: all six `TestParseFrontmatter*` tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/index/frontmatter.go internal/index/note_index_test.go go.mod
git commit -m "Add frontmatter parser backed by yaml.v2"
```

---

### Task 4: Wire frontmatter into `Build` — `Tags()` and `NotesByTag()`

Integrate the parser into the walk, populate `NoteEntry.Tags`, and build `byTag` + `allTags`. Ship the two public lookups.

**Files:**
- Modify: `internal/index/note_index.go` (extend `Build`, add methods)
- Modify: `internal/index/note_index_test.go` (add tag tests)

- [ ] **Step 1: Write failing tests for `Tags()` and `NotesByTag()`**

Append to `internal/index/note_index_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `cd internal/index && go test -run 'TestTags|TestNotesByTag' -v`
Expected: FAIL — methods don't exist yet.

- [ ] **Step 3: Extend `Build` and add methods**

In `internal/index/note_index.go`, replace the `Build` function with the extended version that parses frontmatter and populates tags. Also add `Tags()` and `NotesByTag()`.

Add at top of file, update imports:

```go
import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/logging"
)
```

Replace `Build`:

```go
func (i *NoteIndex) Build() error {
	var entries []NoteEntry
	byUID := make(map[string]string)
	byTag := make(map[string][]string)

	err := filepath.WalkDir(i.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				i.logger.Warn("skipping path: permission denied", "path", path)
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		rel, err := filepath.Rel(i.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		stem := strings.TrimSuffix(d.Name(), ".md")
		uid := ""
		if m := uidPattern.FindStringSubmatch(d.Name()); m != nil {
			uid = m[1]
		}

		fm, fmErr := parseFrontmatter(path)
		if fmErr != nil {
			i.logger.Warn("frontmatter parse failed", "path", rel, "err", fmErr)
			fm = frontmatter{}
		}

		tags := dedupStrings(fm.Tags)

		entry := NoteEntry{
			RelPath: rel,
			UID:     uid,
			Stem:    stem,
			Tags:    tags,
		}
		entries = append(entries, entry)

		if uid != "" {
			byUID[uid] = rel
		}
		for _, t := range tags {
			byTag[t] = append(byTag[t], rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	allTags := make([]string, 0, len(byTag))
	for t := range byTag {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags)
	for t := range byTag {
		sort.Strings(byTag[t])
	}

	i.mu.Lock()
	i.entries = entries
	i.byUID = byUID
	i.byTag = byTag
	i.allTags = allTags
	i.mu.Unlock()
	return nil
}

// Tags returns a copy of the sorted, deduplicated tag list.
func (i *NoteIndex) Tags() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, len(i.allTags))
	copy(out, i.allTags)
	return out
}

// NotesByTag returns a copy of the sorted rel-path slice for a tag.
// Unknown tags return a non-nil empty slice.
func (i *NoteIndex) NotesByTag(tag string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.byTag[tag]
	out := make([]string, len(paths))
	copy(out, paths)
	return out
}

// dedupStrings returns s with duplicates removed, preserving first-seen
// order. A nil or empty input returns nil.
func dedupStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
```

- [ ] **Step 4: Run tests to confirm they pass**

Run: `cd internal/index && go test -v`
Expected: all tests pass (new tag tests + existing UID tests).

- [ ] **Step 5: Commit**

```bash
git add internal/index/note_index.go internal/index/note_index_test.go
git commit -m "Populate tag lookups in NoteIndex.Build"
```

---

### Task 5: Populate `NoteEntry.Title`, `Aliases`, `Slug`, `Date`

These fields are populated but not exposed via public getters. Tests sit in `package index` so they can read private state directly.

**Files:**
- Modify: `internal/index/note_index.go` (slug derivation, date resolution)
- Modify: `internal/index/note_index_test.go` (entry tests)

- [ ] **Step 1: Write failing tests for entry fields**

Append to `internal/index/note_index_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

Run: `cd internal/index && go test -run 'TestNoteEntry' -v`
Expected: FAIL — `Slug`, `Date`, `DateSource` are never set; tests on `Slug == "weekly-digest"` and `DateSource == "uid"` etc. fail.

- [ ] **Step 3: Add slug + date helpers and wire into `Build`**

Append to `internal/index/note_index.go`:

```go
// deriveSlug returns the normalized slug for an entry. If the frontmatter
// supplies one, normalize it. Otherwise derive from the stem: strip the
// UID + trailing "_" prefix if present, then normalize. An empty residue
// yields an empty slug. Normalization: lowercase; runs of characters that
// are neither letters nor digits become a single "-"; trim leading and
// trailing "-".
func deriveSlug(stem, uid, frontmatterSlug string) string {
	raw := frontmatterSlug
	if raw == "" {
		residue := stem
		if uid != "" && strings.HasPrefix(residue, uid) {
			residue = strings.TrimPrefix(residue, uid)
			residue = strings.TrimPrefix(residue, "_")
		}
		raw = residue
	}
	if raw == "" {
		return ""
	}
	return normalizeSlug(raw)
}

func normalizeSlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}

// resolveDate returns (Date, DateSource) per the spec priority: UID date,
// then frontmatter date, then file mtime. stat may be nil in tests; if
// all branches fail, returns a zero Date and empty DateSource.
func resolveDate(uid string, fmDate time.Time, info os.FileInfo) (time.Time, string) {
	if d, ok := uidDate(uid); ok {
		return d, "uid"
	}
	if !fmDate.IsZero() {
		return fmDate, "frontmatter"
	}
	if info != nil {
		return info.ModTime(), "mtime"
	}
	return time.Time{}, ""
}

// uidDate parses the UID's leading digit run as [Y…][MM][DD]. Returns
// (time.Time{}, false) if the digit run is shorter than 5 or the
// resulting date is not real (e.g., month 13, Feb 30).
func uidDate(uid string) (time.Time, bool) {
	if uid == "" {
		return time.Time{}, false
	}
	underscore := strings.IndexByte(uid, '_')
	if underscore < 5 {
		return time.Time{}, false
	}
	head := uid[:underscore]
	yearLen := len(head) - 4
	if yearLen < 1 {
		return time.Time{}, false
	}
	y, err := parseIntASCII(head[:yearLen])
	if err != nil {
		return time.Time{}, false
	}
	m, err := parseIntASCII(head[yearLen : yearLen+2])
	if err != nil {
		return time.Time{}, false
	}
	d, err := parseIntASCII(head[yearLen+2:])
	if err != nil {
		return time.Time{}, false
	}
	// Reject out-of-range dates. time.Date normalizes silently, so we
	// build then verify the fields round-trip.
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return time.Time{}, false
	}
	return t, true
}

func parseIntASCII(s string) (int, error) {
	if s == "" {
		return 0, errEmptyInt
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNonDigit
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var (
	errEmptyInt = errors.New("empty int")
	errNonDigit = errors.New("non-digit in int")
)
```

- [ ] **Step 4: Add `Slug` to the frontmatter struct**

In `internal/index/frontmatter.go`, replace the `frontmatter` struct:

```go
type frontmatter struct {
	Title   string    `yaml:"title"`
	Slug    string    `yaml:"slug"`
	Tags    []string  `yaml:"tags"`
	Aliases []string  `yaml:"aliases"`
	Date    time.Time `yaml:"date"`
}
```

- [ ] **Step 5: Rewrite the per-file block in `Build` to populate all entry fields**

In `internal/index/note_index.go`, locate the per-file block in `Build` (everything from `fm, fmErr := parseFrontmatter(path)` through `entries = append(entries, entry)`) and replace it with:

```go
		fm, fmErr := parseFrontmatter(path)
		if fmErr != nil {
			i.logger.Warn("frontmatter parse failed", "path", rel, "err", fmErr)
			fm = frontmatter{}
		}

		tags := dedupStrings(fm.Tags)

		var info os.FileInfo
		if fi, ierr := d.Info(); ierr == nil {
			info = fi
		}
		date, source := resolveDate(uid, fm.Date, info)

		entry := NoteEntry{
			RelPath:    rel,
			UID:        uid,
			Stem:       stem,
			Slug:       deriveSlug(stem, uid, fm.Slug),
			Title:      fm.Title,
			Tags:       tags,
			Aliases:    append([]string(nil), fm.Aliases...),
			Date:       date,
			DateSource: source,
		}
		entries = append(entries, entry)
```

- [ ] **Step 6: Run tests to confirm they pass**

Run: `cd internal/index && go test -v`
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/index/note_index.go internal/index/frontmatter.go internal/index/note_index_test.go
git commit -m "Populate NoteEntry Title, Aliases, Slug, Date, DateSource"
```

---

### Task 6: Error tolerance — unreadable files and malformed YAML

Verify the build survives individual-file errors with warnings only, and that unreadable files still contribute their UID to `byUID` (existing-behavior guarantee).

**Files:**
- Modify: `internal/index/note_index_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/index/note_index_test.go`:

```go
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
```

- [ ] **Step 2: Run tests**

Run: `cd internal/index && go test -run 'TestMalformedFrontmatter|TestUnreadableFileStillIndexed' -v`
Expected: both pass without code changes — `Build` already tolerates parse errors and still records entries / UIDs from the filename. If a test fails, the parse-error branch in `Build` or the frontmatter parser needs adjustment.

> **If `TestUnreadableFileStillIndexedByUID` fails** because the file-open error short-circuits before the entry is appended, adjust the `parseFrontmatter` call site in `Build`: on error, log a warning and proceed with a zero-valued `fm` — do **not** skip the entry. This matches the existing behavior already coded in Task 4 (`if fmErr != nil { ... fm = frontmatter{} }`).

- [ ] **Step 3: Commit**

```bash
git add internal/index/note_index_test.go
git commit -m "Cover malformed YAML and unreadable files in NoteIndex tests"
```

---

### Task 7: Switch the renderer to `NoteIndex`

The renderer uses `*index.Index` and `idx.Lookup(uid)`. Switch to `*index.NoteIndex` and `idx.NoteByUID(uid)`. Renderer tests still use `setupTestIndex` which returned `*index.Index`; update to return `*index.NoteIndex` via `index.New(...)`.

**Files:**
- Modify: `internal/renderer/renderer.go:32-37` (struct field type + constructor)
- Modify: `internal/renderer/noteext.go:63, 108, 173` (state field type + lookup calls)
- Modify: `internal/renderer/noteext_test.go` (test setup)

- [ ] **Step 1: Update `Renderer` and `NewRenderer`**

In `internal/renderer/renderer.go`, replace lines 32–54:

```go
type Renderer struct {
	md    goldmark.Markdown
	index *index.NoteIndex
}

func NewRenderer(idx *index.NoteIndex) *Renderer {
	// NOTE: html.WithUnsafe() is deliberately NOT set. Without it, goldmark
	// escapes raw HTML from markdown sources (e.g. a malicious <script> block
	// becomes text). This matters even for a local-only previewer because a
	// note file cloned from an untrusted repo could otherwise run JS in the
	// notesview origin and hit the /api/edit endpoint.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			NoteLinkExtension,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	return &Renderer{md: md, index: idx}
}
```

- [ ] **Step 2: Update `noteLinkState` and lookup calls in `noteext.go`**

In `internal/renderer/noteext.go`:

Line 108 region — replace struct:

```go
type noteLinkState struct {
	idx        *index.NoteIndex
	currentDir string
}
```

Line 63 — replace:

```go
	relPath, ok := state.idx.NoteByUID(uid)
```

Line 173 — replace:

```go
		if relPath, ok := s.idx.NoteByUID(uid); ok {
```

- [ ] **Step 3: Update `setupTestIndex` in `noteext_test.go`**

In `internal/renderer/noteext_test.go`, line 60 area. Replace the function body so it constructs a `*index.NoteIndex`:

```go
func setupTestIndex(t *testing.T) *index.NoteIndex {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201.md"), []byte("# Note"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := index.New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatal(err)
	}
	return idx
}
```

Remove any lingering `index.NewLegacy` call that Task 2 introduced here — the renderer now uses the new type.

- [ ] **Step 4: Run renderer tests**

Run: `cd internal/renderer && go test -v`
Expected: all tests pass.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: all packages green. (`internal/server` still uses `index.NewLegacy` and `index.TagIndex`; those remain until Task 8.)

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/renderer.go internal/renderer/noteext.go internal/renderer/noteext_test.go
git commit -m "Switch renderer to NoteIndex"
```

---

### Task 8: Switch the server to `NoteIndex`

Consolidate `Server.index` + `Server.tagIndex` into a single `Server.index *index.NoteIndex`. Simplify `SSEHub` to hold one index and rebuild once per file event.

**Files:**
- Modify: `internal/server/server.go` (fields, constructor)
- Modify: `internal/server/sse.go` (fields, constructor, event loop)
- Modify: `internal/server/handlers.go` (method calls)

- [ ] **Step 1: Update `Server` struct and `NewServer`**

In `internal/server/server.go`:

Replace the `Server` struct (lines 16–25):

```go
type Server struct {
	root      string
	editor    string
	logger    *slog.Logger
	renderer  *renderer.Renderer
	index     *index.NoteIndex
	sseHub    *SSEHub
	templates *templateSet
}
```

Replace the body of `NewServer` (lines 31–57):

```go
func NewServer(root, editor string, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.Discard()
	}
	idx := index.New(root, logger)
	if err := idx.Build(); err != nil {
		return nil, fmt.Errorf("initial index build: %w", err)
	}
	tpls, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return &Server{
		root:      root,
		editor:    editor,
		logger:    logger,
		renderer:  renderer.NewRenderer(idx),
		index:     idx,
		sseHub:    NewSSEHub(root, logger, idx),
		templates: tpls,
	}, nil
}
```

- [ ] **Step 2: Update `SSEHub`**

In `internal/server/sse.go`:

Replace the `SSEHub` struct (lines 17–26):

```go
type SSEHub struct {
	root    string
	logger  *slog.Logger
	index   *index.NoteIndex
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	watcher *fsnotify.Watcher
	done    chan struct{}
}
```

Replace `NewSSEHub` (lines 33–45):

```go
func NewSSEHub(root string, logger *slog.Logger, idx *index.NoteIndex) *SSEHub {
	if logger == nil {
		logger = logging.Discard()
	}
	return &SSEHub{
		root:    root,
		logger:  logger,
		index:   idx,
		clients: make(map[*sseClient]struct{}),
		done:    make(chan struct{}),
	}
}
```

Replace the Create/Write branch in `eventLoop` (lines 78–88):

```go
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Content (frontmatter) and filenames (UIDs) can both
			// change on Write or Create — rebuild once either way.
			if h.index != nil {
				h.index.Rebuild()
			}
```

- [ ] **Step 3: Update handlers**

In `internal/server/handlers.go`:

Line 70 — replace `tagsCard := s.buildTagsIndex()` uses remain but the underlying call changes. Replace the body of `buildTagsIndex` (lines 216–230):

```go
func (s *Server) buildTagsIndex() *IndexCard {
	tags := s.index.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{
			Name:  tag,
			IsTag: true,
			Href:  "/tags/" + tagPath(tag),
		}
	}
	return &IndexCard{
		Entries: entries,
		Empty:   "No tags found.",
	}
}
```

Line 406 — replace `notes := s.tagIndex.NotesByTag(tag)` with:

```go
	notes := s.index.NotesByTag(tag)
```

The `s.index.Rebuild()` call at line 71 already targets `*NoteIndex` now; no change.

- [ ] **Step 4: Build and run full test suite**

Run: `go build ./...`
Expected: build succeeds. If there's a stray `s.tagIndex` reference, fix it — there should be none after this task.

Run: `go test ./...`
Expected: all packages pass. The `internal/server` tests continue to work because they exercise handler behavior, not the underlying index type.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/sse.go internal/server/handlers.go
git commit -m "Switch server and SSE hub to NoteIndex"
```

---

### Task 9: Delete `Index` and `TagIndex`

With all callers on `NoteIndex`, the old types and their tests can be removed.

**Files:**
- Delete: `internal/index/index.go`
- Delete: `internal/index/index_test.go`
- Delete: `internal/index/tags.go`
- Delete: `internal/index/tags_test.go`

- [ ] **Step 1: Confirm no remaining references to the old API**

Run:

```bash
grep -rn 'TagIndex\|NewTagIndex\|NewLegacy\|index\.Index\b\|idx\.Lookup\|s\.tagIndex\|state\.idx\.Lookup' internal/ cmd/
```

Expected: no results. (The pattern is narrow to avoid matching unrelated `template.Lookup` calls in `internal/server/templates_test.go`.)

- [ ] **Step 2: Move `uidPattern`, `fullUIDPattern`, and `IsUID` into `note_index.go`**

Before deleting `index.go`, move the symbols it still owns into the new file so compilation stays green across the delete.

In `internal/index/note_index.go`, add `"regexp"` to the import block (keeping it sorted) and add at file scope (below the `time` import, above the `NoteEntry` type):

```go
var uidPattern = regexp.MustCompile(`^(\d{5,}_\d+)`)

var fullUIDPattern = regexp.MustCompile(`^\d{5,}_\d+$`)

// IsUID reports whether s matches the UID format: 5+ digits, an
// underscore, then 1+ digits.
func IsUID(s string) bool {
	return fullUIDPattern.MatchString(s)
}
```

- [ ] **Step 3: Move `TestIsUID` into `note_index_test.go`**

Copy the `TestIsUID` function (as rewritten in Task 1) from `internal/index/index_test.go` into `internal/index/note_index_test.go` (append to the end of the file). The new file also needs no additional imports for it — `testing` is already imported.

- [ ] **Step 4: Delete the four legacy files**

```bash
git rm internal/index/index.go internal/index/index_test.go internal/index/tags.go internal/index/tags_test.go
```

- [ ] **Step 5: Full sweep**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: all green.

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/index/
git commit -m "Remove obsolete Index and TagIndex; consolidate into NoteIndex"
```

---

## Final verification checklist

- [ ] `go test ./...` passes.
- [ ] `go vet ./...` clean.
- [ ] `go mod tidy` leaves `gopkg.in/yaml.v2` in the direct-require block (no `// indirect`).
- [ ] `grep -rn 'TagIndex\|NewTagIndex\|NewLegacy\|index\.Index\b\|idx\.Lookup\|s\.tagIndex\|state\.idx\.Lookup' internal/ cmd/` returns nothing.
- [ ] Server boots locally; opening a note still resolves wiki-links (`[[UID]]`) and renders tag pages; file watcher triggers a reload after editing a note.
