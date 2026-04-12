# Clickable Tags Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make tags clickable to filter notes in the sidebar, with a Files/Tags mode toggle in the breadcrumb bar.

**Architecture:** Add a TagIndex that builds a tag→files map alongside the UID index. Introduce `/tags` and `/tags/{tag}` routes serving HTMX partials (or full pages). Replace the `?dir=` query param with localStorage-based sidebar state. Extend the breadcrumb bar with a segmented Files/Tags mode toggle.

**Tech Stack:** Go 1.22+ (net/http), HTMX, goldmark (YAML frontmatter parsing), Tailwind CSS v4, plain JS (localStorage)

---

### Task 1: Tag Index

**Files:**
- Create: `internal/index/tags.go`
- Create: `internal/index/tags_test.go`

- [ ] **Step 1: Write the failing test for `TagIndex.Build` — basic tag extraction**

Create `internal/index/tags_test.go`:

```go
package index

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTagFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)

	writeFile(t, filepath.Join(dir, "2026", "03", "note1.md"),
		"---\ntitle: Note One\ntags: [golang, testing]\n---\n# Note One\n")
	writeFile(t, filepath.Join(dir, "2026", "03", "note2.md"),
		"---\ntitle: Note Two\ntags: [golang, web]\n---\n# Note Two\n")
	writeFile(t, filepath.Join(dir, "notags.md"),
		"---\ntitle: No Tags\n---\n# No Tags\n")
	writeFile(t, filepath.Join(dir, "empty.md"),
		"No frontmatter at all.\n")
	writeFile(t, filepath.Join(dir, "image.png"),
		"not markdown")

	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTagIndexBuild(t *testing.T) {
	dir := setupTagFixtures(t)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	tags := idx.Tags()
	wantTags := []string{"golang", "testing", "web"}
	if len(tags) != len(wantTags) {
		t.Fatalf("Tags() = %v, want %v", tags, wantTags)
	}
	for i, want := range wantTags {
		if tags[i] != want {
			t.Errorf("Tags()[%d] = %q, want %q", i, tags[i], want)
		}
	}
}

func TestTagIndexNotesByTag(t *testing.T) {
	dir := setupTagFixtures(t)
	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	golang := idx.NotesByTag("golang")
	if len(golang) != 2 {
		t.Fatalf("NotesByTag(golang) = %v, want 2 entries", golang)
	}
	// Paths should be sorted
	want0 := filepath.Join("2026", "03", "note1.md")
	want1 := filepath.Join("2026", "03", "note2.md")
	if golang[0] != want0 || golang[1] != want1 {
		t.Errorf("NotesByTag(golang) = %v, want [%s, %s]", golang, want0, want1)
	}

	testing_ := idx.NotesByTag("testing")
	if len(testing_) != 1 {
		t.Fatalf("NotesByTag(testing) = %v, want 1 entry", testing_)
	}

	none := idx.NotesByTag("nonexistent")
	if len(none) != 0 {
		t.Errorf("NotesByTag(nonexistent) = %v, want empty", none)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/index && go test -run TestTagIndex -v`
Expected: FAIL — `NewTagIndex` undefined

- [ ] **Step 3: Implement TagIndex**

Create `internal/index/tags.go`:

```go
package index

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// TagIndex maps tags to the notes that carry them. Built by walking all
// .md files and parsing only the YAML frontmatter.
type TagIndex struct {
	root   string
	logger *slog.Logger
	mu     sync.RWMutex
	tags   map[string][]string // tag → sorted relative paths
	all    []string            // sorted unique tag names
}

func NewTagIndex(root string, logger *slog.Logger) *TagIndex {
	if logger == nil {
		logger = Discard()
	}
	return &TagIndex{
		root:   root,
		logger: logger,
		tags:   make(map[string][]string),
	}
}

func (ti *TagIndex) Build() error {
	tags := make(map[string][]string)

	err := filepath.WalkDir(ti.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				ti.logger.Warn("skipping path: permission denied", "path", path)
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(ti.root, path)
		if err != nil {
			return nil
		}
		fileTags := parseFrontmatterTags(path)
		for _, tag := range fileTags {
			tags[tag] = append(tags[tag], rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	all := make([]string, 0, len(tags))
	for tag, paths := range tags {
		sort.Strings(paths)
		all = append(all, tag)
	}
	sort.Strings(all)

	ti.mu.Lock()
	ti.tags = tags
	ti.all = all
	ti.mu.Unlock()
	return nil
}

// Tags returns a sorted list of all unique tag names.
func (ti *TagIndex) Tags() []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	out := make([]string, len(ti.all))
	copy(out, ti.all)
	return out
}

// NotesByTag returns the sorted list of relative paths for notes carrying
// the given tag. Returns nil if the tag doesn't exist.
func (ti *TagIndex) NotesByTag(tag string) []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	paths := ti.tags[tag]
	if paths == nil {
		return nil
	}
	out := make([]string, len(paths))
	copy(out, paths)
	return out
}

// parseFrontmatterTags reads only the YAML frontmatter from a file and
// extracts the tags field. Returns nil if no tags are found.
func parseFrontmatterTags(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// First line must be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil
	}

	var tags []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		// Match "tags: [tag1, tag2]" or "tags:" followed by "- tag" lines
		if strings.HasPrefix(line, "tags:") {
			value := strings.TrimPrefix(line, "tags:")
			value = strings.TrimSpace(value)
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				// Inline list: [tag1, tag2, tag3]
				inner := value[1 : len(value)-1]
				for _, t := range strings.Split(inner, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						tags = append(tags, t)
					}
				}
			} else if value == "" {
				// Block list: lines starting with "- "
				for scanner.Scan() {
					item := scanner.Text()
					if strings.TrimSpace(item) == "---" {
						return tags
					}
					trimmed := strings.TrimSpace(item)
					if strings.HasPrefix(trimmed, "- ") {
						tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
						if tag != "" {
							tags = append(tags, tag)
						}
					} else {
						break
					}
				}
			}
			return tags
		}
	}
	return nil
}
```

Note: `Discard()` is already available in the `index` package via `internal/logging`. We need to check — if `Discard` is in `internal/logging`, import it. If the UID index already uses `logging.Discard()`, follow the same pattern.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd internal/index && go test -run TestTagIndex -v`
Expected: PASS

- [ ] **Step 5: Write test for duplicate tags and edge cases**

Append to `internal/index/tags_test.go`:

```go
func TestTagIndexDuplicateTags(t *testing.T) {
	dir := t.TempDir()
	// Same tag appears in multiple notes
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntags: [go, go]\n---\n")
	writeFile(t, filepath.Join(dir, "b.md"), "---\ntags: [go]\n---\n")

	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	tags := idx.Tags()
	if len(tags) != 1 || tags[0] != "go" {
		t.Errorf("Tags() = %v, want [go]", tags)
	}

	notes := idx.NotesByTag("go")
	// a.md has "go" twice but should appear only once in the file list
	if len(notes) != 2 {
		t.Errorf("NotesByTag(go) = %v, want 2 entries", notes)
	}
}

func TestTagIndexBlockListFormat(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "block.md"),
		"---\ntitle: Block\ntags:\n  - alpha\n  - beta\n---\n# Block\n")

	idx := NewTagIndex(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	tags := idx.Tags()
	if len(tags) != 2 || tags[0] != "alpha" || tags[1] != "beta" {
		t.Errorf("Tags() = %v, want [alpha, beta]", tags)
	}
}
```

- [ ] **Step 6: Run all tag index tests**

Run: `cd internal/index && go test -run TestTagIndex -v`
Expected: PASS

- [ ] **Step 7: Handle duplicate tags within a single file**

If `parseFrontmatterTags` returns `[go, go]` for a single file, the file should only appear once in that tag's list. Update `Build()` — after collecting tags per file, deduplicate before appending. Add this inside the `WalkDir` callback, after `parseFrontmatterTags`:

```go
		// Deduplicate tags within a single file
		seen := make(map[string]bool, len(fileTags))
		for _, tag := range fileTags {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			tags[tag] = append(tags[tag], rel)
		}
```

Replace the existing simple loop in `Build()`.

- [ ] **Step 8: Run all tag index tests again**

Run: `cd internal/index && go test -run TestTagIndex -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/index/tags.go internal/index/tags_test.go
git commit -m "Add TagIndex for tag-to-file mapping (#27)"
```

---

### Task 2: Wire TagIndex into Server

**Files:**
- Modify: `internal/server/server.go:16-50`
- Modify: `internal/server/sse.go` (Rebuild call)

- [ ] **Step 1: Add TagIndex field to Server struct and build it on startup**

In `internal/server/server.go`, add the tag index field and initialize it:

```go
// In the Server struct, add:
	tagIndex  *index.TagIndex

// In NewServer, after idx.Build():
	tagIdx := index.NewTagIndex(root, logger)
	if err := tagIdx.Build(); err != nil {
		return nil, fmt.Errorf("initial tag index build: %w", err)
	}

// In the return struct:
	tagIndex:  tagIdx,
```

- [ ] **Step 2: Add TagIndex.Rebuild method and wire into SSE hub**

Add to `internal/index/tags.go`:

```go
// Rebuild triggers a background tag index rebuild, coalescing concurrent
// calls. Mirrors Index.Rebuild().
func (ti *TagIndex) Rebuild() {
	if !ti.building.TryLock() {
		return
	}
	go func() {
		defer ti.building.Unlock()
		if err := ti.Build(); err != nil {
			ti.logger.Error("tag index rebuild failed", "err", err)
		}
	}()
}
```

Add `building sync.Mutex` to the `TagIndex` struct.

In `internal/server/sse.go`, find where `idx.Rebuild()` is called on file change and add `tagIdx.Rebuild()` next to it. The SSEHub struct needs a reference to the tag index. Update `NewSSEHub` to accept and store it:

```go
// In SSEHub struct:
	tagIndex *index.TagIndex

// In NewSSEHub signature:
func NewSSEHub(root string, logger *slog.Logger, idx *index.Index, tagIdx *index.TagIndex) *SSEHub {

// Where idx.Rebuild() is called, add:
	h.tagIndex.Rebuild()
```

Update the `NewSSEHub` call in `server.go`:

```go
	sseHub: NewSSEHub(root, logger, idx, tagIdx),
```

- [ ] **Step 3: Run existing tests to verify nothing is broken**

Run: `go test ./...`
Expected: PASS (existing tests may need the NewSSEHub signature update in test helpers if any exist)

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go internal/server/sse.go internal/index/tags.go
git commit -m "Wire TagIndex into server startup and SSE rebuild (#27)"
```

---

### Task 3: Remove `?dir=` Sticky Directory

This is a refactor that simplifies the codebase before adding new features. The `?dir=` query parameter, `parseDirParam()`, `dirQuery()`, `dirLinkHref()`, and related threading are all removed.

**Files:**
- Modify: `internal/server/chrome.go`
- Modify: `internal/server/chrome_test.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`
- Modify: `internal/server/templates.go`
- Modify: `internal/renderer/noteext.go`
- Modify: `internal/renderer/noteext_test.go`
- Modify: `web/templates/note_pane_body.html`
- Modify: `web/src/app.js`

- [ ] **Step 1: Simplify chrome.go — remove dirQuery, dirLinkHref, simplify fileLinkHref and buildBreadcrumbs**

Replace `internal/server/chrome.go` with:

```go
package server

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// viewPath percent-encodes each segment of a relative file/dir path for
// use in /view/ URLs, so names with spaces, #, ? etc. produce valid hrefs.
func viewPath(relPath string) string {
	segments := strings.Split(relPath, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// tagPath percent-encodes a tag name for use in /tags/ URLs.
func tagPath(tag string) string {
	return url.PathEscape(tag)
}

// buildDirBreadcrumbs constructs the sidebar breadcrumb trail for files mode.
// Each intermediate directory segment is a clickable link that loads that
// directory in the sidebar. The final segment is marked Current (no link).
func buildDirBreadcrumbs(sidebarDir string) BreadcrumbsData {
	data := BreadcrumbsData{Mode: "dir"}
	sidebarDir = strings.Trim(sidebarDir, "/")
	if sidebarDir == "" {
		return data
	}
	segments := strings.Split(sidebarDir, "/")
	accumulated := ""
	for i, seg := range segments {
		if accumulated == "" {
			accumulated = seg
		} else {
			accumulated += "/" + seg
		}
		if i == len(segments)-1 {
			data.Crumbs = append(data.Crumbs, Crumb{Label: seg, Current: true})
			continue
		}
		data.Crumbs = append(data.Crumbs, Crumb{
			Label: seg,
			Href:  "/dir/" + viewPath(accumulated),
		})
	}
	return data
}

// buildTagBreadcrumbs constructs the sidebar breadcrumb trail for tag-filter mode.
func buildTagBreadcrumbs(tag string) BreadcrumbsData {
	data := BreadcrumbsData{Mode: "tag"}
	data.Crumbs = append(data.Crumbs, Crumb{Label: tag, Current: true})
	return data
}

// buildTagsListBreadcrumbs constructs the breadcrumb trail for tags-list mode.
func buildTagsListBreadcrumbs() BreadcrumbsData {
	return BreadcrumbsData{Mode: "tags"}
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. Directory entries link to the sidebar directory
// endpoint; file entries link to /view/{path}.
func readDirEntries(absPath, relPath string) ([]IndexEntry, error) {
	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	entries := make([]IndexEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryRel := name
		if relPath != "" {
			entryRel = filepath.ToSlash(filepath.Join(relPath, name))
		}
		var href string
		if de.IsDir() {
			href = "/dir/" + viewPath(entryRel)
		} else {
			href = "/view/" + viewPath(entryRel)
		}
		entries = append(entries, IndexEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Href:  href,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
```

Key changes:
- Removed `dirQuery()`, `dirLinkHref()`, `fileLinkHref()`
- Added `tagPath()` for tag URL encoding
- `readDirEntries` no longer takes `notePath` — directory entries link to `/dir/{path}` (a new sidebar-only endpoint for directory browsing), file entries link to `/view/{path}`
- `buildBreadcrumbs` split into `buildDirBreadcrumbs`, `buildTagBreadcrumbs`, `buildTagsListBreadcrumbs`
- `BreadcrumbsData` gains `Mode` (defined in next task's type changes)

- [ ] **Step 2: Update templates.go types — remove DirQuery, add Mode to BreadcrumbsData**

In `internal/server/templates.go`:

Remove `DirQuery string` from `layoutFields`.

Remove `DirQuery string` from `NotePartialData`.

Add `Mode string` to `BreadcrumbsData`:

```go
type BreadcrumbsData struct {
	Mode   string // "dir", "tags", or "tag"
	Crumbs []Crumb
}
```

Remove `HomeHref` from `BreadcrumbsData` — the mode toggle replaces the old root link.

- [ ] **Step 3: Update handlers.go — remove parseDirParam, simplify handlers**

Remove `parseDirParam()` entirely.

Simplify `buildLayoutFields`:

```go
func (s *Server) buildLayoutFields(title, editPath string) layoutFields {
	lf := layoutFields{
		Title:    title,
		EditPath: editPath,
	}
	if editPath != "" {
		lf.EditHref = "/api/edit/" + viewPath(editPath)
	}
	return lf
}
```

Simplify `handleRoot`:

```go
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}

	lf := s.buildLayoutFields("", "")
	card, err := s.buildDirIndex("")
	if err != nil {
		s.logger.Warn("sidebar build failed", "err", err)
	}
	s.index.Rebuild()

	view := ViewData{
		layoutFields: lf,
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 text-center py-8">No note selected.</p>`),
		IndexCard:    card,
		ViewHref:     "/",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Remove the `hxTargetedAt(r, "sidebar")` branch from `handleRoot` — the sidebar is no longer loaded via `/`.

Simplify `handleView`:

```go
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
			if hxTargetedAt(r, "note-pane") {
				s.writeNoteNotFoundPartial(w, reqPath)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentDir := noteParentDir(reqPath)
	html, fm, err := s.renderer.Render(data, currentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	title := filepath.Base(reqPath)
	if fm != nil && fm.Title != "" {
		title = fm.Title
	}
	noteTitle := title

	editPath := ""
	if s.editor != "" {
		editPath = reqPath
	}

	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:    reqPath,
			NoteTitle:   noteTitle,
			Frontmatter: fm,
			HTML:        template.HTML(html),
			SSEWatch:    viewSSEWatch(reqPath),
			ViewHref:    "/view/" + viewPath(reqPath),
			EditPath:    editPath,
			EditHref:    editHref(editPath),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.templates.renderNotePartial(w, partial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	lf := s.buildLayoutFields(title, editPath)
	card, err := s.buildDirIndex(currentDir)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", currentDir, "err", err)
	}

	view := ViewData{
		layoutFields: lf,
		NotePath:     reqPath,
		NoteTitle:    noteTitle,
		Frontmatter:  fm,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(reqPath),
		ViewHref:     "/view/" + viewPath(reqPath),
		IndexCard:    card,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Remove the `hxTargetedAt(r, "sidebar")` branch from `handleView` — sidebar content is now loaded via `/dir/` or `/tags/` endpoints.

Remove `writeSidebarPartial` — replaced by dedicated handlers.

Add helper:

```go
func editHref(editPath string) string {
	if editPath == "" {
		return ""
	}
	return "/api/edit/" + viewPath(editPath)
}
```

Simplify `buildDirIndex`:

```go
func (s *Server) buildDirIndex(sidebarDir string) (*IndexCard, error) {
	absPath, err := SafePath(s.root, sidebarDir)
	if err != nil {
		return nil, err
	}
	entries, err := readDirEntries(absPath, sidebarDir)
	if err != nil {
		return nil, err
	}
	return &IndexCard{
		Mode:        "dir",
		Breadcrumbs: buildDirBreadcrumbs(sidebarDir),
		Entries:     entries,
		Empty:       "No files here.",
	}, nil
}
```

- [ ] **Step 4: Remove dirQuery from Renderer.Render and noteext.go**

In `internal/renderer/renderer.go`, change the `Render` signature:

```go
func (r *Renderer) Render(source []byte, currentDir string) (string, *Frontmatter, error) {
```

Remove `dirQuery` from `noteLinkState`:

```go
type noteLinkState struct {
	idx        *index.Index
	currentDir string
}
```

In `noteext.go`, remove all references to `state.dirQuery` — internal links become just `/view/{path}` with no query string.

Update `rewriteLinkDestination`:

```go
func rewriteLinkDestination(n *ast.Link, s *noteLinkState) {
	dest := string(n.Destination)

	if strings.HasPrefix(dest, "note://") {
		uid := strings.TrimPrefix(dest, "note://")
		if relPath, ok := s.idx.Lookup(uid); ok {
			n.Destination = []byte("/view/" + relPath)
		} else {
			n.Destination = []byte("#")
			n.SetAttributeString("class", []byte("broken-link"))
			n.SetAttributeString("title", []byte(fmt.Sprintf("Note %s not found", uid)))
		}
		return
	}

	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") {
		return
	}
	if !strings.HasSuffix(dest, ".md") {
		return
	}
	resolved := path.Clean(path.Join(s.currentDir, dest))
	resolved = strings.TrimPrefix(resolved, "/")
	n.Destination = []byte("/view/" + resolved)
}
```

In `wikiLinkParser.Parse`, same change:

```go
	link.Destination = []byte("/view/" + relPath)
```

Update the `Render` call in `renderer.go`:

```go
	ctx := parser.NewContext()
	if r.index != nil {
		ctx.Set(noteLinkStateKey, &noteLinkState{
			idx:        r.index,
			currentDir: currentDir,
		})
	}
```

- [ ] **Step 5: Update note_pane_body.html — remove DirQuery from SSE reload URL**

In `web/templates/note_pane_body.html`, change:

```html
hx-get="{{ .ViewHref }}"
```

The `ViewHref` is now just `/view/{filepath}` with no `?dir=`.

No template change needed if the Go code already produces the clean URL (which it does after step 3).

- [ ] **Step 6: Update app.js — remove ?dir= handling**

Replace `currentSidebarUrl()` and the closing-sidebar dir cleanup:

```javascript
function toggleSidebar() {
  const root = document.documentElement;
  const btn = document.getElementById('sidebar-toggle');
  const open = root.classList.toggle('sidebar-open');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  try {
    localStorage.setItem('notesview.sidebarOpen', open ? '1' : '0');
  } catch (e) {}

  if (open) {
    refreshSidebar();
  }
}

function refreshSidebar() {
  const mode = getSidebarMode();
  let url;
  if (mode === 'tags') {
    url = '/tags';
  } else if (mode === 'tag') {
    const tag = getSidebarTag();
    url = tag ? `/tags/${encodeURIComponent(tag)}` : '/tags';
  } else {
    const dir = getSidebarDir();
    url = `/dir/${dir}`;
  }
  window.htmx && window.htmx.ajax('GET', url, {
    target: '#sidebar',
    swap: 'innerHTML',
  });
}

function getSidebarMode() {
  try { return localStorage.getItem('notesview.sidebarMode') || 'files'; } catch (e) { return 'files'; }
}

function getSidebarTag() {
  try { return localStorage.getItem('notesview.sidebarTag') || ''; } catch (e) { return ''; }
}

function getSidebarDir() {
  try { return localStorage.getItem('notesview.sidebarDir') || ''; } catch (e) { return ''; }
}
```

Note: `refreshSidebar` uses `/dir/{path}` — a new endpoint we'll add in Task 5. For now the function is defined but the endpoint doesn't exist yet. The existing tests will catch this.

- [ ] **Step 7: Update renderer_test.go and noteext_test.go for removed dirQuery**

Update all calls to `Render()` to remove the `dirQuery` parameter. Update expected link destinations to not include `?dir=...`.

- [ ] **Step 8: Rewrite chrome_test.go for new signatures**

Replace `TestBuildBreadcrumbs` with tests for `buildDirBreadcrumbs`:

```go
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
			name:       "trailing slash stripped",
			sidebarDir: "2026/",
			wantMode:   "dir",
			wantCrumbs: []Crumb{
				{Label: "2026", Current: true},
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
```

Update `TestReadDirEntries` to remove the `notePath` argument and update expected hrefs to use `/dir/` and `/view/` patterns.

- [ ] **Step 9: Rewrite handlers_test.go — remove all ?dir= tests, update remaining**

Remove these tests entirely:
- `TestViewHandlerLiveReloadPreservesDir`
- `TestViewHandlerStickyPath`
- `TestViewHandlerDirSurvivesFileClicks`
- `TestViewHandlerSidebarPartial` (sidebar no longer loaded via `/view/`)
- `TestCleanURLWhenNoStickyDir`

Update `TestViewHandler` — remove `?dir=` from assertions, update SSE and link expectations.

Update `TestViewHandlerNotePanePartial` — remove `?dir=2026` from the request URL.

The root handler sidebar partial test is removed (sidebar loaded via `/dir/` now).

- [ ] **Step 10: Run all tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "Remove ?dir= sticky directory in favor of localStorage (#27)"
```

---

### Task 4: `/dir/{path}` Sidebar Endpoint

The sidebar's file-browsing mode now fetches directory content via a dedicated endpoint instead of piggybacking on `/view/`.

**Files:**
- Modify: `internal/server/server.go:59-76` (add route)
- Modify: `internal/server/handlers.go` (add handler)
- Modify: `internal/server/handlers_test.go` (add tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/server/handlers_test.go`:

```go
func TestDirHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/dir/2026/03", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "20260331_9201_todo.md") {
		t.Errorf("expected todo file in sidebar, got: %s", body)
	}
}

func TestDirHandlerRoot(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/dir/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "2026") {
		t.Errorf("expected year dir in root sidebar, got: %s", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestDirHandler -v`
Expected: FAIL — 404

- [ ] **Step 3: Implement handleDir and register route**

Add to `internal/server/handlers.go`:

```go
func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dirPath := r.PathValue("path")

	card, err := s.buildDirIndex(dirPath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", dirPath, "err", err)
		card = &IndexCard{Mode: "dir", Breadcrumbs: buildDirBreadcrumbs(dirPath), Empty: "Failed to read directory."}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Register in `server.go` Routes():

```go
	mux.HandleFunc("GET /dir/{path...}", s.handleDir)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestDirHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go internal/server/server.go
git commit -m "Add /dir/ endpoint for sidebar directory browsing (#27)"
```

---

### Task 5: `/tags` and `/tags/{tag}` Endpoints

**Files:**
- Modify: `internal/server/server.go` (add routes)
- Modify: `internal/server/handlers.go` (add handlers)
- Modify: `internal/server/handlers_test.go` (add tests)

- [ ] **Step 1: Write failing tests**

Add to `internal/server/handlers_test.go`:

```go
func TestTagsHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/tags", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The test fixture has tags: [todo, daily]
	if !strings.Contains(body, "todo") || !strings.Contains(body, "daily") {
		t.Errorf("expected tags in sidebar, got: %s", body)
	}
}

func TestTagNotesHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/tags/todo", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "20260331_9201_todo.md") {
		t.Errorf("expected todo note in filtered list, got: %s", body)
	}
}

func TestTagNotesHandlerUnknownTag(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/tags/nonexistent", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "No notes") {
		t.Errorf("expected empty state message, got: %s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run "TestTags" -v`
Expected: FAIL — 404

- [ ] **Step 3: Implement handlers**

Add to `internal/server/handlers.go`:

```go
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags := s.tagIndex.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{
			Name: tag,
			Href: "/tags/" + tagPath(tag),
		}
	}
	card := &IndexCard{
		Mode:        "tags",
		Breadcrumbs: buildTagsListBreadcrumbs(),
		Entries:     entries,
		Empty:       "No tags found.",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTagNotes(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	notes := s.tagIndex.NotesByTag(tag)
	entries := make([]IndexEntry, len(notes))
	for i, notePath := range notes {
		entries[i] = IndexEntry{
			Name: filepath.Base(notePath),
			Href: "/view/" + viewPath(notePath),
		}
	}
	card := &IndexCard{
		Mode:        "tag",
		Breadcrumbs: buildTagBreadcrumbs(tag),
		Entries:     entries,
		Empty:       "No notes with this tag.",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Register routes in `server.go`:

```go
	mux.HandleFunc("GET /tags", s.handleTags)
	mux.HandleFunc("GET /tags/{tag}", s.handleTagNotes)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -run "TestTags" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/handlers.go internal/server/handlers_test.go internal/server/server.go
git commit -m "Add /tags and /tags/{tag} sidebar endpoints (#27)"
```

---

### Task 6: Template Changes — Breadcrumbs and Index Card

**Files:**
- Modify: `web/templates/breadcrumbs.html`
- Modify: `web/templates/index_card.html`
- Modify: `web/templates/note_pane_body.html`

- [ ] **Step 1: Rewrite breadcrumbs.html with mode toggle**

```html
{{ define "breadcrumbs" }}<nav class="flex-1 flex items-center gap-1.5 text-sm text-gray-500 overflow-hidden whitespace-nowrap">
  <span class="inline-flex rounded-md border border-gray-300 overflow-hidden flex-shrink-0">
    <button type="button"
       onclick="switchToFiles()"
       class="px-1.5 py-0.5 text-xs font-medium transition-colors {{ if eq .Mode "dir" }}bg-gray-700 text-white{{ else }}text-gray-500 hover:bg-gray-100{{ end }}"
       aria-label="Browse files">&#128193;</button>
    <button type="button"
       onclick="switchToTags()"
       class="px-1.5 py-0.5 text-xs font-medium border-l border-gray-300 transition-colors {{ if or (eq .Mode "tags") (eq .Mode "tag") }}bg-gray-700 text-white{{ else }}text-gray-500 hover:bg-gray-100{{ end }}"
       aria-label="Browse tags">&#127991;&#65039;</button>
  </span>
  {{ range .Crumbs }}
  <span class="text-gray-400 select-none">/</span>
  {{ if .Current }}
  <span aria-current="page" class="text-gray-900 overflow-hidden text-ellipsis">{{ .Label }}</span>
  {{ else }}
  <a class="text-blue-600 no-underline hover:underline" href="{{ .Href }}" hx-get="{{ .Href }}" hx-target="#sidebar" hx-swap="innerHTML">{{ .Label }}</a>
  {{ end }}
  {{ end }}
</nav>{{ end }}
```

- [ ] **Step 2: Extend index_card.html for tags and tag modes**

```html
{{ define "index_card" }}
<div class="w-full">
  <header class="bg-gray-50 px-4 py-2 border-b border-gray-200 flex items-center">
    {{ template "breadcrumbs" .Breadcrumbs }}
  </header>
  {{ if .Entries }}
  <ul class="list-none m-0 p-0">
    {{ range .Entries }}
    {{ if eq $.Mode "tags" }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-get="{{ .Href }}"
        hx-target="#sidebar"
        hx-swap="innerHTML"
        onclick="setSidebarTag('{{ .Name }}')"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300">&#127991;&#65039; {{ .Name }}</a>
    </li>
    {{ else if .IsDir }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-get="{{ .Href }}"
        hx-target="#sidebar"
        hx-swap="innerHTML"
        onclick="setSidebarDir('{{ .Href }}')"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 font-medium no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300">&#128193; {{ .Name }}</a>
    </li>
    {{ else }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-boost="true"
        hx-target="#note-pane"
        hx-swap="innerHTML"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300">&#128196; {{ .Name }}</a>
    </li>
    {{ end }}
    {{ end }}
  </ul>
  {{ else }}
  <p class="px-4 py-6 text-gray-500 text-center">{{ .Empty }}</p>
  {{ end }}
</div>
{{ end }}
```

Note: the `{{ if eq $.Mode "tags" }}` branch handles tag list entries (which target the sidebar), while directory and file entries use their existing patterns. The `hx-boost` on directories is replaced with explicit `hx-get`/`hx-target` to load the sidebar partial.

- [ ] **Step 3: Make tag pills clickable in note_pane_body.html**

Replace the tags `<ul>` in `note_pane_body.html`:

```html
      {{ if .Tags }}
      <ul class="flex flex-wrap gap-1.5 m-0 p-0 list-none">
        {{ range .Tags }}<li><a href="/tags/{{ urlquery . }}" hx-get="/tags/{{ urlquery . }}" hx-target="#sidebar" hx-swap="innerHTML" onclick="setSidebarTag('{{ . }}')" class="inline-flex items-center bg-blue-100 text-blue-600 text-xs font-medium px-2 py-0.5 rounded-full leading-relaxed no-underline cursor-pointer hover:bg-blue-200">{{ . }}</a></li>{{ end }}
      </ul>
      {{ end }}
```

- [ ] **Step 4: Update template parsing to include funcmap for urlquery**

Check if `urlquery` is already available — it's a built-in Go template function, so no changes needed.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/templates/breadcrumbs.html web/templates/index_card.html web/templates/note_pane_body.html
git commit -m "Update templates with mode toggle and clickable tags (#27)"
```

---

### Task 7: Client-Side JavaScript — Sidebar State Management

**Files:**
- Modify: `web/src/app.js`

- [ ] **Step 1: Implement sidebar state functions**

Add to `web/src/app.js`, replacing the old `currentSidebarUrl` and updating `toggleSidebar`:

```javascript
// notesview front-end bootstrap.
//
// Loads HTMX + SSE, runs syntax highlighting on every swap, and owns
// the sidebar toggle and sidebar mode state (files/tags/tag).

import 'htmx.org';
import 'htmx-ext-sse';
import hljs from 'highlight.js/lib/common';

function highlightIn(root) {
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el);
  });
}

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document);
  wireSidebarToggle();
  restoreSidebarState();
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
});

// --- Sidebar toggle ---

function wireSidebarToggle() {
  const btn = document.getElementById('sidebar-toggle');
  if (!btn) return;
  const initiallyOpen = document.documentElement.classList.contains('sidebar-open');
  btn.setAttribute('aria-expanded', initiallyOpen ? 'true' : 'false');
  btn.addEventListener('click', toggleSidebar);
}

function toggleSidebar() {
  const root = document.documentElement;
  const btn = document.getElementById('sidebar-toggle');
  const open = root.classList.toggle('sidebar-open');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  try {
    localStorage.setItem('notesview.sidebarOpen', open ? '1' : '0');
  } catch (e) {}

  if (open) {
    refreshSidebar();
  }
}

// --- Sidebar mode state ---

function refreshSidebar() {
  const mode = getSidebarMode();
  let url;
  if (mode === 'tags') {
    url = '/tags';
  } else if (mode === 'tag') {
    const tag = getSidebarTag();
    url = tag ? `/tags/${encodeURIComponent(tag)}` : '/tags';
  } else {
    const dir = getSidebarDir();
    url = `/dir/${dir}`;
  }
  window.htmx && window.htmx.ajax('GET', url, {
    target: '#sidebar',
    swap: 'innerHTML',
  });
}

function restoreSidebarState() {
  const mode = getSidebarMode();
  if (mode === 'files') return; // Server already rendered files mode
  refreshSidebar();
}

function getSidebarMode() {
  try { return localStorage.getItem('notesview.sidebarMode') || 'files'; } catch (e) { return 'files'; }
}

function getSidebarTag() {
  try { return localStorage.getItem('notesview.sidebarTag') || ''; } catch (e) { return ''; }
}

function getSidebarDir() {
  try { return localStorage.getItem('notesview.sidebarDir') || ''; } catch (e) { return ''; }
}

// Global functions called from template onclick handlers.
// These update localStorage before HTMX fires the request.

// switchToFiles navigates the sidebar to the current note's parent dir.
// Called by the Files button in the breadcrumb toggle. Uses JS to compute
// the URL dynamically (the note path isn't known at template render time
// when the sidebar is in tags mode).
window.switchToFiles = function() {
  const notePath = document.querySelector('#note-card')?.dataset?.notePath || '';
  const parent = notePath ? notePath.replace(/[^/]*$/, '').replace(/\/$/, '') : '';
  try {
    localStorage.setItem('notesview.sidebarMode', 'files');
    localStorage.setItem('notesview.sidebarDir', parent);
  } catch (e) {}
  window.htmx && window.htmx.ajax('GET', `/dir/${encodeURIComponent(parent)}`, {
    target: '#sidebar',
    swap: 'innerHTML',
  });
};

window.switchToTags = function() {
  try {
    localStorage.setItem('notesview.sidebarMode', 'tags');
  } catch (e) {}
  window.htmx && window.htmx.ajax('GET', '/tags', {
    target: '#sidebar',
    swap: 'innerHTML',
  });
};

window.setSidebarTag = function(tag) {
  try {
    localStorage.setItem('notesview.sidebarMode', 'tag');
    localStorage.setItem('notesview.sidebarTag', tag);
  } catch (e) {}
};

window.setSidebarDir = function(href) {
  try {
    // Extract dir path from /dir/... href
    const dir = href.replace(/^\/dir\//, '');
    localStorage.setItem('notesview.sidebarMode', 'files');
    localStorage.setItem('notesview.sidebarDir', decodeURIComponent(dir));
  } catch (e) {}
};
```

- [ ] **Step 2: Build frontend assets**

Run: `cd web/src && npm run build` (or whatever the build command is — check `package.json`)

- [ ] **Step 3: Run all Go tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/app.js
git commit -m "Add sidebar mode state management in localStorage (#27)"
```

---

### Task 8: Manual Testing and Polish

- [ ] **Step 1: Start the dev server**

Run: `go run ./cmd/notesview --root <notes-dir>`

- [ ] **Step 2: Test tag pill click in note body**

Open a note with tags. Click a tag pill. Verify:
- Sidebar switches to show notes with that tag (flat filename list)
- Breadcrumb shows `[Files|Tags] / tagname`
- Tags button is highlighted in the toggle

- [ ] **Step 3: Test Files/Tags toggle**

Click "Tags" in the toggle. Verify:
- Sidebar shows alphabetical list of all tags
- Click a tag → sidebar shows filtered notes
- Click "Files" → sidebar returns to directory tree, showing the current note's parent

- [ ] **Step 4: Test page reload persistence**

While in tag mode, reload the page. Verify:
- Sidebar restores to tag mode from localStorage
- The currently viewed note is preserved

- [ ] **Step 5: Test clicking active mode button**

While in files mode, click "Files" again. Verify the sidebar reloads.
While viewing a tag, click "Tags" to go back to tags list.

- [ ] **Step 6: Test note navigation in tag mode**

While in tag filter mode, click a note in the list. Verify:
- The note loads in the note pane
- The sidebar stays in tag filter mode

- [ ] **Step 7: Fix any issues found during manual testing**

Address any bugs discovered. Each fix gets its own commit.

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "Polish clickable tags feature (#27)"
```
