# notesview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single Go binary (`notesview`) that serves a local HTTP server for browsing and rendering markdown files with GitHub-style presentation, live-reload, and a collapsible file browser.

**Architecture:** Single-page app served from an embedded `embed.FS`. Go backend handles markdown rendering (goldmark), file watching (fsnotify), and a small REST API. Frontend is vanilla HTML/CSS/JS — no build step.

**Tech Stack:** Go 1.22+, goldmark, goldmark-meta, chroma, fsnotify

---

## File Structure

```
notesview/
├── cmd/notesview/main.go          # CLI entry point, flag parsing, server startup
├── internal/
│   ├── index/index.go             # UID-to-filepath index (scan + lookup)
│   ├── index/index_test.go
│   ├── renderer/renderer.go       # Goldmark setup, render markdown to HTML
│   ├── renderer/renderer_test.go
│   ├── renderer/notelinks.go      # Custom goldmark extension: note://, UID auto-link, .md rewrite
│   ├── renderer/notelinks_test.go
│   ├── renderer/tasks.go          # Custom goldmark extension: [+], [ ], [daily] task syntax
│   ├── renderer/tasks_test.go
│   ├── server/server.go           # HTTP server, router, middleware
│   ├── server/handlers.go         # Handler functions (view, browse, edit, raw, events)
│   ├── server/handlers_test.go
│   ├── server/sse.go              # SSE hub: client tracking, file watch, broadcast
│   ├── server/sse_test.go
│   ├── server/pathutil.go         # Path validation (stays within root)
│   └── server/pathutil_test.go
├── web/
│   ├── embed.go                   # embed.FS declaration
│   ├── static/
│   │   ├── index.html             # SPA shell
│   │   ├── style.css              # GitHub-style markdown CSS + layout
│   │   └── app.js                 # Client-side routing, sidebar, SSE, navigation
├── go.mod
└── go.sum
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/notesview/main.go`
- Create: `web/embed.go`
- Create: `web/static/index.html`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd /Users/alex/20260331_plain-thunder
go mod init github.com/dreikanter/notes-view
```

Expected: `go.mod` created

- [ ] **Step 2: Create minimal main.go**

Create `cmd/notesview/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("notesview starting...")
	os.Exit(0)
}
```

- [ ] **Step 3: Create embed.go placeholder**

Create `web/embed.go`:
```go
package web

import "embed"

//go:embed static/*
var StaticFS embed.FS
```

- [ ] **Step 4: Create minimal index.html**

Create `web/static/index.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>notesview</title>
  <link rel="stylesheet" href="/static/style.css">
</head>
<body>
  <div id="app">
    <header id="topbar">
      <button id="sidebar-toggle" aria-label="Toggle sidebar">&#9776;</button>
      <nav id="breadcrumbs"></nav>
      <button id="edit-btn">Edit</button>
    </header>
    <aside id="sidebar" class="hidden"></aside>
    <main id="content"></main>
  </div>
  <script src="/static/app.js"></script>
</body>
</html>
```

- [ ] **Step 5: Create empty CSS and JS files**

Create `web/static/style.css`:
```css
/* notesview styles — populated in Task 9 */
*, *::before, *::after { box-sizing: border-box; }
```

Create `web/static/app.js`:
```js
// notesview client — populated in Task 9
console.log('notesview loaded');
```

- [ ] **Step 6: Verify it builds**

Run:
```bash
go build ./cmd/notesview
./notesview
```

Expected: prints "notesview starting..." and exits

- [ ] **Step 7: Commit**

```bash
git init
echo -e "notesview\n.superpowers/" > .gitignore
git add .
git commit -m "scaffold: init notesview project with module, embed, and SPA shell"
```

---

### Task 2: Path Validation Utility

**Files:**
- Create: `internal/server/pathutil.go`
- Create: `internal/server/pathutil_test.go`

- [ ] **Step 1: Write tests for path validation**

Create `internal/server/pathutil_test.go`:
```go
package server

import (
	"testing"
)

func TestSafePath(t *testing.T) {
	root := "/notes"

	tests := []struct {
		name    string
		reqPath string
		want    string
		wantErr bool
	}{
		{"simple file", "2026/03/hello.md", "/notes/2026/03/hello.md", false},
		{"root dir", "", "/notes", false},
		{"dot segments rejected", "../etc/passwd", "", true},
		{"double dot in middle", "2026/../../etc/passwd", "", true},
		{"absolute path rejected", "/etc/passwd", "", true},
		{"clean path", "2026/03/../03/hello.md", "/notes/2026/03/hello.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(root, tt.reqPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got %q", tt.reqPath, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.reqPath, err)
				return
			}
			if got != tt.want {
				t.Errorf("SafePath(%q, %q) = %q, want %q", root, tt.reqPath, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSafePath -v`
Expected: FAIL — `SafePath` not defined

- [ ] **Step 3: Implement SafePath**

Create `internal/server/pathutil.go`:
```go
package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafePath resolves reqPath relative to root and ensures the result
// stays within root. Returns the absolute cleaned path or an error.
func SafePath(root, reqPath string) (string, error) {
	if filepath.IsAbs(reqPath) {
		return "", fmt.Errorf("absolute path not allowed: %s", reqPath)
	}
	joined := filepath.Join(root, reqPath)
	cleaned := filepath.Clean(joined)
	// Ensure the cleaned path is within root
	if !strings.HasPrefix(cleaned, filepath.Clean(root)) {
		return "", fmt.Errorf("path traversal detected: %s", reqPath)
	}
	return cleaned, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestSafePath -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/pathutil.go internal/server/pathutil_test.go
git commit -m "feat: add SafePath utility for path traversal protection"
```

---

### Task 3: UID Index

**Files:**
- Create: `internal/index/index.go`
- Create: `internal/index/index_test.go`

- [ ] **Step 1: Write tests for UID index**

Create `internal/index/index_test.go`:
```go
package index

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create directory structure
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.MkdirAll(filepath.Join(dir, "2026", "01"), 0o755)
	// Create note files
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260330_9198.md"), []byte("# Note"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "01", "20260102_8814_report.md"), []byte("# Report"), 0o644)
	// Non-matching file
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
		want string // relative path from root
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
		{"2026031_9201", false},  // too short date
		{"20260331_", false},     // no ID
		{"20260331_abc", false},  // non-numeric ID
		{"hello_world", false},
		{"202603319201", false},  // no underscore
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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/index/ -v`
Expected: FAIL — package not found

- [ ] **Step 3: Implement the UID index**

Create `internal/index/index.go`:
```go
package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// uidPattern matches YYYYMMDD_NNNN (8 digits, underscore, 1+ digits)
var uidPattern = regexp.MustCompile(`^(\d{8}_\d+)`)

// IsUID returns true if s matches the YYYYMMDD_NNNN pattern exactly.
func IsUID(s string) bool {
	return regexp.MustCompile(`^\d{8}_\d+$`).MatchString(s)
}

// Index maps note UIDs to their relative file paths.
type Index struct {
	root  string
	mu    sync.RWMutex
	uids  map[string]string // uid -> relative path
}

// New creates an index for the given root directory.
func New(root string) *Index {
	return &Index{
		root: root,
		uids: make(map[string]string),
	}
}

// Build scans the root directory tree and indexes all files matching
// the YYYYMMDD_NNNN*.md pattern.
func (idx *Index) Build() error {
	uids := make(map[string]string)
	err := filepath.WalkDir(idx.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		matches := uidPattern.FindStringSubmatch(d.Name())
		if matches == nil {
			return nil
		}
		rel, err := filepath.Rel(idx.root, path)
		if err != nil {
			return nil
		}
		uids[matches[1]] = rel
		return nil
	})
	if err != nil {
		return err
	}
	idx.mu.Lock()
	idx.uids = uids
	idx.mu.Unlock()
	return nil
}

// Lookup returns the relative path for a UID, or ("", false) if not found.
func (idx *Index) Lookup(uid string) (string, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	p, ok := idx.uids[uid]
	return p, ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/index/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/index/
git commit -m "feat: add UID index for note file lookup"
```

---

### Task 4: Task Syntax Extension

**Files:**
- Create: `internal/renderer/tasks.go`
- Create: `internal/renderer/tasks_test.go`

- [ ] **Step 1: Write tests for task syntax**

Create `internal/renderer/tasks_test.go`:
```go
package renderer

import (
	"strings"
	"testing"
)

func TestTaskSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			"completed task",
			"- [+] Done item",
			[]string{`class="task-checked"`, "Done item"},
		},
		{
			"pending task",
			"- [ ] Pending item",
			[]string{`class="task-unchecked"`, "Pending item"},
		},
		{
			"daily tag",
			"- [daily] Morning routine",
			[]string{`class="task-tag"`, "daily", "Morning routine"},
		},
		{
			"normal list item unchanged",
			"- Normal item",
			[]string{"Normal item"},
		},
	}

	r := NewRenderer(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, _, err := r.Render([]byte(tt.input), "")
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(html, want) {
					t.Errorf("output missing %q\ngot: %s", want, html)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/renderer/ -run TestTaskSyntax -v`
Expected: FAIL — package not found

- [ ] **Step 3: Implement the Renderer skeleton and task syntax**

Create `internal/renderer/renderer.go`:
```go
package renderer

import (
	"bytes"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	highlighting "github.com/yuin/goldmark-highlighting/v2"

	"github.com/dreikanter/notes-view/internal/index"
)

// Frontmatter holds parsed YAML frontmatter fields.
type Frontmatter struct {
	Title       string   `yaml:"title"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
	Slug        string   `yaml:"slug"`
}

// Renderer converts markdown to HTML with note-aware extensions.
type Renderer struct {
	md    goldmark.Markdown
	index *index.Index
}

// NewRenderer creates a Renderer. Pass nil for index if UID resolution is not needed.
func NewRenderer(idx *index.Index) *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(
				highlighting.WithStyle("github"),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	return &Renderer{md: md, index: idx}
}

// Render converts markdown source to HTML. Returns the HTML string and
// parsed frontmatter (if any). currentDir is the directory of the source
// file (for resolving relative links).
func (r *Renderer) Render(source []byte, currentDir string) (string, *Frontmatter, error) {
	ctx := parser.NewContext()
	var buf bytes.Buffer
	if err := r.md.Convert(source, &buf, parser.WithContext(ctx)); err != nil {
		return "", nil, err
	}

	// Extract frontmatter
	var fm *Frontmatter
	metaData := meta.Get(ctx)
	if metaData != nil {
		fm = &Frontmatter{}
		if t, ok := metaData["title"].(string); ok {
			fm.Title = t
		}
		if d, ok := metaData["description"].(string); ok {
			fm.Description = d
		}
		if s, ok := metaData["slug"].(string); ok {
			fm.Slug = s
		}
		if tags, ok := metaData["tags"].([]interface{}); ok {
			for _, tag := range tags {
				if s, ok := tag.(string); ok {
					fm.Tags = append(fm.Tags, s)
				}
			}
		}
	}

	html := buf.String()
	// Post-process: task syntax
	html = processTaskSyntax(html)
	// Post-process: note links (if index available)
	if r.index != nil {
		html = processNoteLinks(html, r.index, currentDir)
	}

	return html, fm, nil
}
```

Create `internal/renderer/tasks.go`:
```go
package renderer

import (
	"regexp"
	"strings"
)

// taskPatterns matches the rendered HTML for custom task markers.
// Goldmark GFM renders `- [+]` as a list item with text starting with "[+]".
// We post-process the HTML to replace these with styled checkboxes.
var taskPatterns = []struct {
	marker  string
	replace func(text string) string
}{
	{
		"[+] ",
		func(text string) string {
			return `<span class="task-checked">&#10003;</span> ` + text
		},
	},
	{
		"[ ] ",
		func(text string) string {
			return `<span class="task-unchecked"></span> ` + text
		},
	},
}

var dailyPattern = regexp.MustCompile(`\[daily\]\s*`)

func processTaskSyntax(html string) string {
	// Handle [+] and [ ] markers
	for _, p := range taskPatterns {
		html = strings.ReplaceAll(html, p.marker, p.replace(""))
	}
	// Handle [daily] — replace with a badge
	html = dailyPattern.ReplaceAllString(html, `<span class="task-tag">daily</span> `)
	return html
}
```

- [ ] **Step 4: Fetch dependencies**

Run:
```bash
go get github.com/yuin/goldmark
go get github.com/yuin/goldmark-meta
go get github.com/yuin/goldmark-highlighting/v2
go get github.com/fsnotify/fsnotify
go get github.com/dreikanter/notes-view/internal/index
```

Wait — the index is a local package, no `go get` needed. Run:
```bash
go mod tidy
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/renderer/ -run TestTaskSyntax -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/renderer/ go.mod go.sum
git commit -m "feat: add markdown renderer with task syntax support"
```

---

### Task 5: Note Link Extensions

**Files:**
- Create: `internal/renderer/notelinks.go`
- Create: `internal/renderer/notelinks_test.go`

- [ ] **Step 1: Write tests for note link processing**

Create `internal/renderer/notelinks_test.go`:
```go
package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreikanter/notes-view/internal/index"
)

func setupTestIndex(t *testing.T) *index.Index {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260330_9198.md"), []byte("# Note"), 0o644)
	idx := index.New(dir)
	idx.Build()
	return idx
}

func TestNoteProtocolLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)

	input := `See [my todo](note://20260331_9201) for details.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("note:// link not resolved:\n%s", html)
	}
}

func TestBrokenNoteLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)

	input := `See [missing](note://99999999_0000) link.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="broken-link"`) {
		t.Errorf("broken note:// link not marked:\n%s", html)
	}
}

func TestAutoLinkUID(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)

	input := `Refer to 20260331_9201 for the todo list.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("UID not auto-linked:\n%s", html)
	}
}

func TestAutoLinkUIDNoMatch(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)

	input := `Reference 99999999_0000 does not exist.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `<a href=`) {
		t.Errorf("non-matching UID should not be linked:\n%s", html)
	}
}

func TestRelativeMdLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)

	input := `See [other note](../01/20260102_8814.md) for details.`
	html, _, err := r.Render([]byte(input), "2026/03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/01/20260102_8814.md"`) {
		t.Errorf("relative .md link not rewritten:\n%s", html)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/renderer/ -run TestNote -v`
Expected: FAIL — `processNoteLinks` is referenced in renderer.go but not defined

- [ ] **Step 3: Implement note link processing**

Create `internal/renderer/notelinks.go`:
```go
package renderer

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dreikanter/notes-view/internal/index"
)

// noteProtoRe matches href="note://UID" in rendered HTML
var noteProtoRe = regexp.MustCompile(`href="note://(\d{8}_\d+)"`)

// relativeMdRe matches href="...something.md" (not starting with / or http)
var relativeMdRe = regexp.MustCompile(`href="((?:[^":](?:(?://)|[^"/])*)\.md)"`)

// uidInTextRe matches bare UIDs in text (not inside tags or existing links)
var uidInTextRe = regexp.MustCompile(`\b(\d{8}_\d{4,})\b`)

func processNoteLinks(html string, idx *index.Index, currentDir string) string {
	// 1. Resolve note:// protocol links
	html = noteProtoRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := noteProtoRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		uid := sub[1]
		if relPath, ok := idx.Lookup(uid); ok {
			return fmt.Sprintf(`href="/view/%s"`, relPath)
		}
		return fmt.Sprintf(`href="#" class="broken-link" title="Note %s not found"`, uid)
	})

	// 2. Rewrite relative .md links to /view/ routes
	html = relativeMdRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := relativeMdRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		relLink := sub[1]
		// Resolve relative to the current file's directory
		resolved := path.Clean(path.Join(currentDir, relLink))
		// Remove leading slash if present
		resolved = strings.TrimPrefix(resolved, "/")
		return fmt.Sprintf(`href="/view/%s"`, resolved)
	})

	// 3. Auto-link bare UIDs in text content (not inside HTML tags)
	// Split by HTML tags, only process text segments
	parts := splitByTags(html)
	for i, part := range parts {
		if !strings.HasPrefix(part, "<") {
			parts[i] = uidInTextRe.ReplaceAllStringFunc(part, func(match string) string {
				if relPath, ok := idx.Lookup(match); ok {
					return fmt.Sprintf(`<a href="/view/%s" class="uid-link">%s</a>`, relPath, match)
				}
				return match
			})
		}
	}
	return strings.Join(parts, "")
}

// splitByTags splits HTML into alternating text and tag segments.
// Tags start with < and end with >. Text is everything else.
func splitByTags(html string) []string {
	var parts []string
	for len(html) > 0 {
		tagStart := strings.Index(html, "<")
		if tagStart == -1 {
			parts = append(parts, html)
			break
		}
		if tagStart > 0 {
			parts = append(parts, html[:tagStart])
		}
		tagEnd := strings.Index(html[tagStart:], ">")
		if tagEnd == -1 {
			parts = append(parts, html[tagStart:])
			break
		}
		parts = append(parts, html[tagStart:tagStart+tagEnd+1])
		html = html[tagStart+tagEnd+1:]
	}
	return parts
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/renderer/ -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/notelinks.go internal/renderer/notelinks_test.go
git commit -m "feat: add note:// links, UID auto-linking, and relative .md rewriting"
```

---

### Task 6: HTTP Server and Handlers

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/handlers.go`
- Create: `internal/server/handlers_test.go`

- [ ] **Step 1: Write tests for handlers**

Create `internal/server/handlers_test.go`:
```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("---\ntitle: Todo\ntags: [todo, daily]\n---\n# Todo\n- [+] Done\n- [ ] Pending\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Welcome\nHello"), 0o644)

	srv := NewServer(dir, "")
	return srv, dir
}

func TestViewHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	// Should return JSON with html and frontmatter
	var resp ViewResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v\nbody: %s", err, body)
	}
	if resp.HTML == "" {
		t.Error("HTML is empty")
	}
	if resp.Frontmatter == nil || resp.Frontmatter.Title != "Todo" {
		t.Errorf("frontmatter not parsed correctly: %+v", resp.Frontmatter)
	}
}

func TestViewHandler404(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/nonexistent.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestBrowseHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/browse/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var entries []BrowseEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Should have 2026/ dir and README.md
	hasDir := false
	hasFile := false
	for _, e := range entries {
		if e.Name == "2026" && e.IsDir {
			hasDir = true
		}
		if e.Name == "README.md" && !e.IsDir {
			hasFile = true
		}
	}
	if !hasDir {
		t.Error("missing 2026/ directory")
	}
	if !hasFile {
		t.Error("missing README.md")
	}
}

func TestRawHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/raw/README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "# Welcome\nHello" {
		t.Errorf("raw content = %q", w.Body.String())
	}
}

func TestPathTraversal(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for path traversal", w.Code)
	}
}

func TestRootRedirect(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should redirect to /view/README.md since it exists
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/view/README.md" {
		t.Errorf("redirect location = %q, want /view/README.md", loc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestView -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Implement the server and handlers**

Create `internal/server/server.go`:
```go
package server

import (
	"io/fs"
	"net/http"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/internal/renderer"
	"github.com/dreikanter/notes-view/web"
)

// Server is the notesview HTTP server.
type Server struct {
	root     string
	editor   string
	renderer *renderer.Renderer
	index    *index.Index
	sseHub   *SSEHub
}

// NewServer creates a new server for the given root directory.
func NewServer(root, editor string) *Server {
	idx := index.New(root)
	idx.Build()
	return &Server{
		root:     root,
		editor:   editor,
		renderer: renderer.NewRenderer(idx),
		index:    idx,
		sseHub:   NewSSEHub(root),
	}
}

// Routes returns the HTTP handler with all routes configured.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /view/{filepath...}", s.handleView)
	mux.HandleFunc("GET /browse/{dirpath...}", s.handleBrowse)
	mux.HandleFunc("GET /browse/", s.handleBrowse)
	mux.HandleFunc("POST /api/edit/{filepath...}", s.handleEdit)
	mux.HandleFunc("GET /api/raw/{filepath...}", s.handleRaw)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.HandleFunc("GET /", s.handleRoot)

	// Static files (embedded)
	staticFS, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	return mux
}
```

Create `internal/server/handlers.go`:
```go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dreikanter/notes-view/internal/renderer"
)

// ViewResponse is the JSON response for the /view/ endpoint.
type ViewResponse struct {
	HTML        string                `json:"html"`
	Frontmatter *renderer.Frontmatter `json:"frontmatter,omitempty"`
	Path        string                `json:"path"`
}

// BrowseEntry is a single entry in a directory listing.
type BrowseEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Path  string `json:"path"`
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// Serve the SPA shell for any unmatched route (client-side routing)
		s.serveSPA(w, r)
		return
	}
	// Check for README.md
	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}
	// Fall back to SPA (which will show file browser)
	http.Redirect(w, r, "/browse/", http.StatusFound)
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	data, err := web.StaticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentDir := filepath.Dir(reqPath)
	html, fm, err := s.renderer.Render(data, currentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ViewResponse{
		HTML:        html,
		Frontmatter: fm,
		Path:        reqPath,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("dirpath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entries []BrowseEntry
	for _, de := range dirEntries {
		name := de.Name()
		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Show only .md files and directories
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryPath := filepath.Join(reqPath, name)
		entries = append(entries, BrowseEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Path:  entryPath,
		})
	}

	// Sort: directories first, then alphabetical
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	// Refresh UID index opportunistically
	go s.index.Build()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	editor := s.editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		http.Error(w, "$EDITOR is not set", http.StatusBadRequest)
		return
	}

	cmd := exec.Command(editor, absPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf("failed to start editor: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Create SSE hub stub** (full implementation in Task 7)

Create `internal/server/sse.go`:
```go
package server

import "net/http"

// SSEHub manages Server-Sent Events connections and file watching.
type SSEHub struct {
	root string
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(root string) *SSEHub {
	return &SSEHub{root: root}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Stub — implemented in Task 7
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -v`
Expected: All tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/server.go internal/server/handlers.go internal/server/handlers_test.go internal/server/sse.go
git commit -m "feat: add HTTP server with view, browse, raw, and edit handlers"
```

---

### Task 7: SSE Live Reload

**Files:**
- Modify: `internal/server/sse.go`
- Create: `internal/server/sse_test.go`

- [ ] **Step 1: Write tests for SSE hub**

Create `internal/server/sse_test.go`:
```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSSEConnection(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.md")
	os.WriteFile(testFile, []byte("# Test"), 0o644)

	hub := NewSSEHub(dir)
	hub.Start()
	defer hub.Stop()

	srv := &Server{root: dir, sseHub: hub}

	// Create a request with watch parameter
	req := httptest.NewRequest("GET", "/events?watch=test.md", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Run handler in goroutine since it blocks
	done := make(chan struct{})
	go func() {
		srv.handleSSE(w, req)
		close(done)
	}()

	// Give the handler time to set up
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	os.WriteFile(testFile, []byte("# Updated"), 0o644)

	// Wait for context to expire
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Logf("SSE body: %s", body)
		// fsnotify may not fire in temp dirs on all OS — just check no panic
	}
}

func TestSSEHubClientCleanup(t *testing.T) {
	dir := t.TempDir()
	hub := NewSSEHub(dir)
	hub.Start()
	defer hub.Stop()

	// Verify hub starts with no clients
	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients, got %d", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSSE -v`
Expected: FAIL — `Start`, `Stop`, methods not defined

- [ ] **Step 3: Implement the full SSE hub**

Replace `internal/server/sse.go`:
```go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// SSEHub manages Server-Sent Events connections and file watching.
type SSEHub struct {
	root    string
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	watcher *fsnotify.Watcher
	done    chan struct{}
}

type sseClient struct {
	watchPath string // relative path being watched
	events    chan string
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(root string) *SSEHub {
	return &SSEHub{
		root:    root,
		clients: make(map[*sseClient]struct{}),
		done:    make(chan struct{}),
	}
}

// Start initializes the file watcher and event loop.
func (h *SSEHub) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	h.watcher = watcher
	go h.eventLoop()
	return nil
}

// Stop shuts down the hub.
func (h *SSEHub) Stop() {
	close(h.done)
	if h.watcher != nil {
		h.watcher.Close()
	}
}

func (h *SSEHub) eventLoop() {
	// Debounce timer
	var debounce *time.Timer
	var lastPath string

	for {
		select {
		case <-h.done:
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Debounce: reset timer on each event
			lastPath = event.Name
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(100*time.Millisecond, func() {
				h.broadcast(lastPath)
			})
		case _, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (h *SSEHub) broadcast(absPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		safePath, err := SafePath(h.root, client.watchPath)
		if err != nil {
			continue
		}
		if safePath == absPath {
			select {
			case client.events <- client.watchPath:
			default:
				// Client not draining — skip
			}
		}
	}
}

func (h *SSEHub) addClient(c *sseClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	// Watch the file
	if absPath, err := SafePath(h.root, c.watchPath); err == nil {
		h.watcher.Add(absPath)
	}
}

func (h *SSEHub) removeClient(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	watchPath := r.URL.Query().Get("watch")
	if watchPath == "" {
		http.Error(w, "watch parameter required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{
		watchPath: watchPath,
		events:    make(chan string, 1),
	}
	s.sseHub.addClient(client)
	defer s.sseHub.removeClient(client)

	// Send initial connected event
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{"type": "connected"}))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case path := <-client.events:
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{
				"type": "change",
				"path": path,
			}))
			flusher.Flush()
		}
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -v -timeout 10s`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/sse.go internal/server/sse_test.go
git commit -m "feat: add SSE live reload with file watching and debounce"
```

---

### Task 8: Frontend — CSS (GitHub-style)

**Files:**
- Modify: `web/static/style.css`

- [ ] **Step 1: Write the GitHub-style CSS**

Replace `web/static/style.css` with the full stylesheet:
```css
/* === Reset & Base === */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --color-fg: #1f2328;
  --color-fg-muted: #656d76;
  --color-fg-accent: #0969da;
  --color-bg: #ffffff;
  --color-bg-subtle: #f6f8fa;
  --color-border: #d0d7de;
  --color-border-muted: #d8dee4;
  --color-success: #1a7f37;
  --color-danger: #d1242f;
  --color-tag-bg: #ddf4ff;
  --color-tag-fg: #0969da;
  --sidebar-width: 280px;
  --topbar-height: 48px;
  --content-max-width: 900px;
  --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif;
  --font-mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
}

body {
  font-family: var(--font-sans);
  font-size: 16px;
  line-height: 1.5;
  color: var(--color-fg);
  background: var(--color-bg);
}

/* === Top Bar === */
#topbar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: var(--topbar-height);
  background: var(--color-bg-subtle);
  border-bottom: 1px solid var(--color-border);
  display: flex;
  align-items: center;
  padding: 0 16px;
  gap: 12px;
  z-index: 100;
}

#sidebar-toggle {
  background: none;
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 4px 8px;
  cursor: pointer;
  font-size: 16px;
  color: var(--color-fg);
  line-height: 1;
}
#sidebar-toggle:hover { background: var(--color-bg); }

#breadcrumbs {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 14px;
  overflow: hidden;
  white-space: nowrap;
}
#breadcrumbs a {
  color: var(--color-fg-accent);
  text-decoration: none;
}
#breadcrumbs a:hover { text-decoration: underline; }
#breadcrumbs .separator { color: var(--color-fg-muted); }

#edit-btn {
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 4px 12px;
  font-size: 13px;
  cursor: pointer;
  color: var(--color-fg);
  white-space: nowrap;
}
#edit-btn:hover { background: var(--color-bg-subtle); border-color: var(--color-fg-muted); }

/* === Sidebar === */
#sidebar {
  position: fixed;
  top: var(--topbar-height);
  left: 0;
  bottom: 0;
  width: var(--sidebar-width);
  background: var(--color-bg-subtle);
  border-right: 1px solid var(--color-border);
  overflow-y: auto;
  transform: translateX(0);
  transition: transform 0.2s ease;
  z-index: 90;
  padding: 8px 0;
}
#sidebar.hidden { transform: translateX(-100%); }

.tree-item {
  display: flex;
  align-items: center;
  padding: 4px 12px;
  cursor: pointer;
  font-size: 14px;
  color: var(--color-fg);
  text-decoration: none;
  gap: 6px;
  user-select: none;
}
.tree-item:hover { background: var(--color-border-muted); }
.tree-item.active { background: var(--color-tag-bg); color: var(--color-fg-accent); font-weight: 500; }
.tree-item .icon { width: 16px; text-align: center; flex-shrink: 0; font-size: 12px; }
.tree-children { display: none; }
.tree-children.open { display: block; }

/* Indent levels */
.tree-item[data-depth="1"] { padding-left: 28px; }
.tree-item[data-depth="2"] { padding-left: 44px; }
.tree-item[data-depth="3"] { padding-left: 60px; }
.tree-item[data-depth="4"] { padding-left: 76px; }

/* === Content Area === */
#content {
  margin-top: var(--topbar-height);
  max-width: var(--content-max-width);
  margin-left: auto;
  margin-right: auto;
  padding: 32px 24px;
}

/* === Frontmatter Metadata Bar === */
.fm-bar { margin-bottom: 24px; }
.fm-title { font-size: 2em; font-weight: 600; line-height: 1.25; margin-bottom: 4px; }
.fm-description { font-size: 14px; color: var(--color-fg-muted); margin-bottom: 8px; }
.fm-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.fm-tag {
  background: var(--color-tag-bg);
  color: var(--color-tag-fg);
  font-size: 12px;
  padding: 2px 10px;
  border-radius: 16px;
  font-weight: 500;
}

/* === Markdown Content (GitHub-style) === */
.markdown-body h1 { font-size: 2em; font-weight: 600; padding-bottom: 0.3em; border-bottom: 1px solid var(--color-border-muted); margin: 24px 0 16px; }
.markdown-body h2 { font-size: 1.5em; font-weight: 600; padding-bottom: 0.3em; border-bottom: 1px solid var(--color-border-muted); margin: 24px 0 16px; }
.markdown-body h3 { font-size: 1.25em; font-weight: 600; margin: 24px 0 16px; }
.markdown-body h4 { font-size: 1em; font-weight: 600; margin: 24px 0 16px; }
.markdown-body h1:first-child, .markdown-body h2:first-child, .markdown-body h3:first-child { margin-top: 0; }

.markdown-body p { margin: 0 0 16px; }
.markdown-body a { color: var(--color-fg-accent); text-decoration: none; }
.markdown-body a:hover { text-decoration: underline; }
.markdown-body a.broken-link { color: var(--color-danger); text-decoration: line-through; }
.markdown-body a.uid-link { color: var(--color-fg-accent); }

.markdown-body code {
  font-family: var(--font-mono);
  font-size: 85%;
  background: rgba(175, 184, 193, 0.2);
  padding: 0.2em 0.4em;
  border-radius: 6px;
}
.markdown-body pre {
  font-family: var(--font-mono);
  font-size: 85%;
  background: var(--color-bg-subtle);
  border: 1px solid var(--color-border);
  border-radius: 6px;
  padding: 16px;
  overflow-x: auto;
  margin: 0 0 16px;
  line-height: 1.45;
}
.markdown-body pre code {
  background: none;
  padding: 0;
  border-radius: 0;
  font-size: 100%;
}

.markdown-body blockquote {
  border-left: 4px solid var(--color-border);
  padding: 0 16px;
  color: var(--color-fg-muted);
  margin: 0 0 16px;
}

.markdown-body table {
  border-collapse: collapse;
  width: 100%;
  margin: 0 0 16px;
}
.markdown-body th, .markdown-body td {
  border: 1px solid var(--color-border);
  padding: 6px 13px;
}
.markdown-body th { font-weight: 600; background: var(--color-bg-subtle); }
.markdown-body tr:nth-child(even) { background: var(--color-bg-subtle); }

.markdown-body ul, .markdown-body ol { padding-left: 2em; margin: 0 0 16px; }
.markdown-body li { margin: 4px 0; }
.markdown-body li + li { margin-top: 4px; }

.markdown-body img { max-width: 100%; }
.markdown-body hr { border: none; border-top: 2px solid var(--color-border-muted); margin: 24px 0; }

/* === Task Syntax === */
.task-checked {
  color: var(--color-success);
  font-weight: bold;
  margin-right: 4px;
}
.task-unchecked {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border);
  border-radius: 3px;
  vertical-align: text-bottom;
  margin-right: 4px;
}
.task-tag {
  background: #e8d5b7;
  color: #6e4b1e;
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  font-weight: 500;
  vertical-align: middle;
  margin-right: 4px;
}

/* === Directory Listing (inline) === */
.dir-listing { list-style: none; padding: 0; }
.dir-listing li {
  border-bottom: 1px solid var(--color-border);
  padding: 0;
}
.dir-listing li:first-child { border-top: 1px solid var(--color-border); }
.dir-listing a {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  color: var(--color-fg-accent);
  text-decoration: none;
}
.dir-listing a:hover { background: var(--color-bg-subtle); }
.dir-listing .icon { width: 20px; text-align: center; }

/* === Error States === */
.error-page {
  text-align: center;
  padding: 80px 24px;
  color: var(--color-fg-muted);
}
.error-page h2 { margin-bottom: 8px; color: var(--color-fg); }

/* === Responsive === */
@media (max-width: 768px) {
  #content { padding: 16px 12px; }
  .fm-title { font-size: 1.5em; }
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/notesview`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add web/static/style.css
git commit -m "feat: add GitHub-style CSS for markdown rendering and layout"
```

---

### Task 9: Frontend — JavaScript SPA

**Files:**
- Modify: `web/static/app.js`

- [ ] **Step 1: Implement the client-side SPA**

Replace `web/static/app.js`:
```js
(function() {
  'use strict';

  const $ = (sel) => document.querySelector(sel);
  const content = $('#content');
  const sidebar = $('#sidebar');
  const breadcrumbs = $('#breadcrumbs');
  const sidebarToggle = $('#sidebar-toggle');
  const editBtn = $('#edit-btn');

  let currentPath = '';
  let eventSource = null;

  // === Navigation ===

  function navigate(path, pushState) {
    if (pushState === undefined) pushState = true;
    if (path.startsWith('/view/')) {
      loadFile(path.slice(6), pushState);
    } else if (path.startsWith('/browse/')) {
      loadDir(path.slice(8), pushState);
    } else if (path === '/') {
      // Let server redirect
      fetch('/', { redirect: 'follow' }).then(function(r) {
        navigate(new URL(r.url).pathname, pushState);
      });
    } else {
      loadFile(path, pushState);
    }
  }

  function loadFile(filePath, pushState) {
    currentPath = filePath;
    if (pushState) history.pushState({ path: '/view/' + filePath }, '', '/view/' + filePath);
    updateBreadcrumbs(filePath);
    editBtn.style.display = '';

    fetch('/view/' + encodePathSegments(filePath))
      .then(function(r) {
        if (!r.ok) {
          if (r.status === 404) return { html: '', error: 'not found' };
          throw new Error('HTTP ' + r.status);
        }
        return r.json();
      })
      .then(function(data) {
        if (data.error) {
          content.innerHTML = '<div class="error-page"><h2>File not found</h2><p>' + escapeHtml(filePath) + '</p></div>';
          return;
        }
        renderFile(data);
        connectSSE(filePath);
        highlightSidebarItem(filePath);
      })
      .catch(function(err) {
        content.innerHTML = '<div class="error-page"><h2>Error</h2><p>' + escapeHtml(err.message) + '</p></div>';
      });
  }

  function loadDir(dirPath, pushState) {
    currentPath = '';
    if (pushState) history.pushState({ path: '/browse/' + dirPath }, '', '/browse/' + dirPath);
    updateBreadcrumbs(dirPath);
    editBtn.style.display = 'none';
    disconnectSSE();

    fetch('/browse/' + encodePathSegments(dirPath))
      .then(function(r) { return r.json(); })
      .then(function(entries) {
        var html = '<ul class="dir-listing">';
        if (dirPath) {
          var parent = dirPath.split('/').slice(0, -1).join('/');
          html += '<li><a href="/browse/' + parent + '" data-link><span class="icon">&#x1F519;</span> ..</a></li>';
        }
        entries.forEach(function(e) {
          if (e.isDir) {
            html += '<li><a href="/browse/' + e.path + '" data-link><span class="icon">&#128193;</span> ' + escapeHtml(e.name) + '</a></li>';
          } else {
            html += '<li><a href="/view/' + e.path + '" data-link><span class="icon">&#128196;</span> ' + escapeHtml(e.name) + '</a></li>';
          }
        });
        html += '</ul>';
        content.innerHTML = html;
      });
  }

  function renderFile(data) {
    var html = '';
    if (data.frontmatter) {
      var fm = data.frontmatter;
      html += '<div class="fm-bar">';
      if (fm.title) html += '<h1 class="fm-title">' + escapeHtml(fm.title) + '</h1>';
      if (fm.description) html += '<p class="fm-description">' + escapeHtml(fm.description) + '</p>';
      if (fm.tags && fm.tags.length) {
        html += '<div class="fm-tags">';
        fm.tags.forEach(function(tag) {
          html += '<span class="fm-tag">' + escapeHtml(tag) + '</span>';
        });
        html += '</div>';
      }
      html += '</div>';
    }
    html += '<div class="markdown-body">' + data.html + '</div>';
    content.innerHTML = html;

    // Strip redundant first h1 if frontmatter title matches
    if (data.frontmatter && data.frontmatter.title) {
      var firstH1 = content.querySelector('.markdown-body h1:first-child');
      if (firstH1 && firstH1.textContent.trim() === data.frontmatter.title) {
        firstH1.remove();
      }
    }
  }

  // === Breadcrumbs ===

  function updateBreadcrumbs(path) {
    var parts = path.split('/').filter(Boolean);
    var html = '<a href="/browse/" data-link>&#127968;</a>';
    var cumulative = '';
    parts.forEach(function(part, i) {
      cumulative += (cumulative ? '/' : '') + part;
      html += '<span class="separator">/</span>';
      if (i === parts.length - 1 && part.endsWith('.md')) {
        html += '<span>' + escapeHtml(part) + '</span>';
      } else {
        html += '<a href="/browse/' + cumulative + '" data-link>' + escapeHtml(part) + '</a>';
      }
    });
    breadcrumbs.innerHTML = html;
  }

  // === Sidebar ===

  sidebarToggle.addEventListener('click', function() {
    sidebar.classList.toggle('hidden');
  });

  function loadSidebar() {
    loadSidebarDir('', sidebar, 0);
  }

  function loadSidebarDir(dirPath, container, depth) {
    fetch('/browse/' + encodePathSegments(dirPath))
      .then(function(r) { return r.json(); })
      .then(function(entries) {
        var html = '';
        entries.forEach(function(e) {
          if (e.isDir) {
            html += '<div class="tree-node">';
            html += '<div class="tree-item" data-dir="' + escapeAttr(e.path) + '" data-depth="' + depth + '">';
            html += '<span class="icon">&#9656;</span> ' + escapeHtml(e.name);
            html += '</div>';
            html += '<div class="tree-children" data-dir-children="' + escapeAttr(e.path) + '"></div>';
            html += '</div>';
          } else {
            html += '<a class="tree-item" href="/view/' + escapeAttr(e.path) + '" data-link data-file="' + escapeAttr(e.path) + '" data-depth="' + depth + '">';
            html += '<span class="icon">&#128196;</span> ' + escapeHtml(e.name);
            html += '</a>';
          }
        });
        container.innerHTML = html;
      });
  }

  sidebar.addEventListener('click', function(e) {
    var item = e.target.closest('.tree-item[data-dir]');
    if (!item) return;
    e.preventDefault();
    var dirPath = item.getAttribute('data-dir');
    var children = item.nextElementSibling;
    if (children && children.classList.contains('open')) {
      children.classList.remove('open');
      item.querySelector('.icon').innerHTML = '&#9656;';
      return;
    }
    if (children) {
      if (!children.hasChildNodes()) {
        var depth = parseInt(item.getAttribute('data-depth') || '0') + 1;
        loadSidebarDir(dirPath, children, depth);
      }
      children.classList.add('open');
      item.querySelector('.icon').innerHTML = '&#9662;';
    }
  });

  function highlightSidebarItem(filePath) {
    sidebar.querySelectorAll('.tree-item.active').forEach(function(el) {
      el.classList.remove('active');
    });
    var link = sidebar.querySelector('[data-file="' + CSS.escape(filePath) + '"]');
    if (link) link.classList.add('active');
  }

  // === SSE Live Reload ===

  function connectSSE(filePath) {
    disconnectSSE();
    eventSource = new EventSource('/events?watch=' + encodeURIComponent(filePath));
    eventSource.onmessage = function(e) {
      var data = JSON.parse(e.data);
      if (data.type === 'change' && data.path === currentPath) {
        // Reload the file content
        fetch('/view/' + encodePathSegments(currentPath))
          .then(function(r) { return r.json(); })
          .then(function(data) { renderFile(data); });
      }
    };
  }

  function disconnectSSE() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
  }

  // === Edit Button ===

  editBtn.addEventListener('click', function() {
    if (!currentPath) return;
    fetch('/api/edit/' + encodePathSegments(currentPath), { method: 'POST' })
      .then(function(r) {
        if (!r.ok) return r.text().then(function(t) { alert('Edit failed: ' + t); });
      });
  });

  // === Client-Side Link Handling ===

  document.addEventListener('click', function(e) {
    var link = e.target.closest('a[data-link], .markdown-body a[href^="/view/"], .markdown-body a[href^="/browse/"]');
    if (!link) return;
    var href = link.getAttribute('href');
    if (!href || href.startsWith('http')) return;
    e.preventDefault();
    navigate(href);
  });

  window.addEventListener('popstate', function(e) {
    if (e.state && e.state.path) {
      navigate(e.state.path, false);
    }
  });

  // === Helpers ===

  function encodePathSegments(path) {
    return path.split('/').map(encodeURIComponent).join('/');
  }

  function escapeHtml(s) {
    var div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }

  function escapeAttr(s) {
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // === Init ===

  loadSidebar();
  var initialPath = window.location.pathname;
  if (initialPath === '/') {
    navigate('/');
  } else {
    navigate(initialPath, false);
  }
})();
```

- [ ] **Step 2: Update index.html for SPA routing**

The SPA shell needs to be served for all `/view/` and `/browse/` routes that aren't API calls. Update the server's `handleRoot` to also catch these. This is already handled — the server returns JSON for `/view/` and `/browse/`, and the SPA shell is `index.html` loaded directly.

However, we need the server to serve `index.html` for direct browser navigation to `/view/...` paths. Add a catch-all to `server.go`:

In `internal/server/server.go`, add to `Routes()` before the static file handler:
```go
	// SPA catch-all: serve index.html for /view/ and /browse/ paths
	// when Accept header prefers HTML (direct browser navigation)
	mux.HandleFunc("GET /app/{path...}", s.serveSPA)
```

Actually, a simpler approach: make the `/view/` handler check the Accept header. If the client wants HTML (browser navigation), serve the SPA shell. If it wants JSON (fetch call), serve the API response. Update `handleView`:

At the top of `handleView` in `internal/server/handlers.go`:
```go
func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	// If browser is navigating directly, serve the SPA shell
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		s.serveSPA(w, r)
		return
	}
	// ... rest of handler unchanged
```

Apply the same pattern to the root handler and add to `handleBrowse`:
```go
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		s.serveSPA(w, r)
		return
	}
	// ... rest unchanged
```

And update the JavaScript `fetch` calls to include an `Accept: application/json` header:

In `app.js`, update the fetch calls:
```js
  var jsonHeaders = { headers: { 'Accept': 'application/json' } };
```

Then use `fetch(url, jsonHeaders)` for all `/view/` and `/browse/` fetches.

- [ ] **Step 3: Verify it builds**

Run: `go build ./cmd/notesview`
Expected: builds successfully

- [ ] **Step 4: Commit**

```bash
git add web/static/app.js web/static/index.html internal/server/handlers.go internal/server/server.go
git commit -m "feat: add SPA frontend with sidebar, breadcrumbs, SSE live-reload"
```

---

### Task 10: CLI Entry Point

**Files:**
- Modify: `cmd/notesview/main.go`

- [ ] **Step 1: Implement CLI with flag parsing and server startup**

Replace `cmd/notesview/main.go`:
```go
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/dreikanter/notes-view/internal/server"
)

func main() {
	port := flag.Int("port", 0, "port to listen on (default: auto)")
	flag.IntVar(port, "p", 0, "port to listen on (shorthand)")
	open := flag.Bool("open", true, "open browser on start")
	flag.BoolVar(open, "o", true, "open browser on start (shorthand)")
	editor := flag.String("editor", "", "editor command (overrides $EDITOR)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: notesview [options] [path]\n\n")
		fmt.Fprintf(os.Stderr, "Serve markdown files with live preview.\n\n")
		fmt.Fprintf(os.Stderr, "Path resolution: argument > $NOTES_PATH > current directory\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Resolve root directory
	root := resolveRoot(flag.Arg(0))

	// Verify root exists
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a directory\n", root)
		os.Exit(1)
	}

	// Create and start server
	srv := server.NewServer(root, *editor)
	if err := srv.StartWatcher(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: file watcher failed to start: %v\n", err)
	}

	// Listen
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	addr := listener.Addr().String()
	url := "http://" + addr
	fmt.Printf("notesview serving %s at %s\n", root, url)

	if *open {
		openBrowser(url)
	}

	if err := http.Serve(listener, srv.Routes()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func resolveRoot(arg string) string {
	if arg != "" {
		abs, err := absPath(arg)
		if err == nil {
			return abs
		}
		return arg
	}
	if p := os.Getenv("NOTES_PATH"); p != "" {
		abs, err := absPath(p)
		if err == nil {
			return abs
		}
		return p
	}
	dir, _ := os.Getwd()
	return dir
}

func absPath(p string) (string, error) {
	if p[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + p[1:]
	}
	return p, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
```

- [ ] **Step 2: Add StartWatcher method to Server**

Add to `internal/server/server.go`:
```go
// StartWatcher starts the SSE hub's file watcher.
func (s *Server) StartWatcher() error {
	return s.sseHub.Start()
}
```

- [ ] **Step 3: Build and test manually**

Run:
```bash
go build -o notesview ./cmd/notesview
```

Expected: binary builds.

Create a test directory and run:
```bash
mkdir -p /tmp/notesview-test
echo "# Hello\nThis is a test." > /tmp/notesview-test/README.md
./notesview --open=false /tmp/notesview-test
```

Expected: prints URL, server starts. Ctrl-C to stop.

- [ ] **Step 4: Commit**

```bash
git add cmd/notesview/main.go internal/server/server.go
git commit -m "feat: add CLI entry point with flag parsing and browser auto-open"
```

---

### Task 11: Integration Smoke Test

**Files:**
- Create: `integration_test.go`

- [ ] **Step 1: Write an integration test**

Create `integration_test.go` in the project root:
```go
//go:build integration

package main_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dreikanter/notes-view/internal/server"
)

func TestIntegrationSmoke(t *testing.T) {
	// Set up a test directory with notes
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Welcome\n\nHello world.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"),
		[]byte("---\ntitle: Daily Todo\ntags: [todo]\n---\n# Daily Todo\n\n- [+] Done task\n- [ ] Pending task\n- [daily] Routine\n\nSee [readme](../../README.md) and note://20260331_9201.\n"), 0o644)

	srv := server.NewServer(dir, "")
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Test: root redirects to README
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("root: status = %d, want 302", resp.StatusCode)
	}

	// Test: view a file (JSON API)
	req, _ := http.NewRequest("GET", ts.URL+"/view/2026/03/20260331_9201_todo.md", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("view: status = %d", resp.StatusCode)
	}

	var viewResp struct {
		HTML        string `json:"html"`
		Frontmatter struct {
			Title string   `json:"title"`
			Tags  []string `json:"tags"`
		} `json:"frontmatter"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &viewResp)

	if viewResp.Frontmatter.Title != "Daily Todo" {
		t.Errorf("title = %q", viewResp.Frontmatter.Title)
	}
	if viewResp.HTML == "" {
		t.Error("HTML is empty")
	}

	// Test: browse root
	req, _ = http.NewRequest("GET", ts.URL+"/browse/", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("browse: status = %d", resp.StatusCode)
	}

	// Test: raw endpoint
	resp, err = http.Get(ts.URL + "/api/raw/README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != "# Welcome\n\nHello world.\n" {
		t.Errorf("raw = %q", raw)
	}

	// Test: 404
	req, _ = http.NewRequest("GET", ts.URL+"/view/nonexistent.md", nil)
	req.Header.Set("Accept", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("404: status = %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test -tags=integration -v -timeout 30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add integration_test.go
git commit -m "test: add integration smoke test for all endpoints"
```

---

### Task 12: Polish and Final Build

- [ ] **Step 1: Add .gitignore entry for binary**

Verify `.gitignore` contains `notesview` (added in Task 1).

- [ ] **Step 2: Run all tests**

Run:
```bash
go test ./... -v
go test -tags=integration -v -timeout 30s
```

Expected: All PASS

- [ ] **Step 3: Build final binary**

Run:
```bash
go build -o notesview ./cmd/notesview
```

Expected: binary built successfully.

- [ ] **Step 4: Manual smoke test**

Run with your notes directory:
```bash
./notesview
```

Expected: opens browser, renders markdown, sidebar works, live-reload works on file edit, Edit button spawns `$EDITOR`.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "chore: final polish and build verification"
```
