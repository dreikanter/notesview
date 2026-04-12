# Sidebar Tree Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sidebar's hidden Files/Tags mode toggle with a GitHub-repo-browser-style tree where FILES and TAGS are permanent roots, and the main panel shows directory/tag listings or notes.

**Architecture:** The sidebar becomes a persistent two-section tree (FILES, TAGS) rendered server-side on initial load. HTMX partial requests update individual section contents and the main panel independently. Clicking dirs/tags loads a listing into the main panel; clicking notes loads rendered markdown. The selected item gets a highlight in the sidebar.

**Tech Stack:** Go templates, HTMX, Tailwind CSS, vanilla JS (localStorage)

---

## File Map

**Create:**
- `web/templates/sidebar_tree.html` — new sidebar template with FILES/TAGS sections, collapse toggles, and HTMX targets
- `web/templates/entry_list.html` — shared entry list partial (`<ul>` of entries) used by both sidebar sections and main panel listing
- `web/templates/dir_listing.html` — main panel wrapper for directory/tag listings (reuses `entry_list`)

**Modify:**
- `internal/server/templates.go` — remove `BreadcrumbsData`/`Crumb`, add `DirListingData`, update `IndexCard`, update `parsePartial`, add `renderDirListing` method
- `internal/server/chrome.go` — remove breadcrumb builders, keep `readDirEntries`/`viewPath`/`tagPath`
- `internal/server/handlers.go` — update `handleDir`/`handleTags`/`handleTagNotes` to support `HX-Target: note-pane`, update `handleView`/`handleRoot` for new sidebar shape
- `internal/server/server.go` — no changes (routes stay the same)
- `web/templates/sidebar_body.html` — rewrite to render `sidebar_tree` instead of `index_card`
- `web/templates/note_pane_body.html` — update tag click handlers
- `web/src/app.js` — replace sidebar mode logic with tree section toggle/drill/select logic
- `web/src/style.css` — add selected-state and section-heading styles

**Remove:**
- `web/templates/breadcrumbs.html`

**Test:**
- `internal/server/chrome_test.go` — remove breadcrumb tests, keep `readDirEntries` tests
- `internal/server/handlers_test.go` — update sidebar-related assertions, add dir-listing-in-note-pane tests
- `internal/server/templates_test.go` — update for removed breadcrumbs, new templates

---

### Task 1: Remove Breadcrumbs (Backend)

**Files:**
- Modify: `internal/server/templates.go:12-21` (remove `Crumb`, `BreadcrumbsData`)
- Modify: `internal/server/templates.go:29-37` (remove `Breadcrumbs` from `IndexCard`)
- Modify: `internal/server/chrome.go:26-74` (remove breadcrumb builders)
- Modify: `internal/server/handlers.go:189-204` (remove `Breadcrumbs` from `buildDirIndex`)
- Modify: `internal/server/handlers.go:275-287` (remove `Breadcrumbs` from `handleDir` error case)
- Modify: `internal/server/handlers.go:289-308` (remove `Breadcrumbs` from `handleTags`)
- Modify: `internal/server/handlers.go:310-330` (remove `Breadcrumbs` from `handleTagNotes`)
- Modify: `internal/server/templates.go:84-89` (remove `breadcrumbs.html` from partials list)
- Modify: `internal/server/templates.go:117-119` (remove `breadcrumbs.html` from `parsePartial`)
- Remove: `web/templates/breadcrumbs.html`
- Modify: `web/templates/index_card.html:3-4` (remove breadcrumbs header)
- Test: `internal/server/chrome_test.go`
- Test: `internal/server/templates_test.go`

- [ ] **Step 1: Update tests — remove breadcrumb tests from chrome_test.go**

Delete the entire `TestBuildDirBreadcrumbs` function from `internal/server/chrome_test.go` (lines 9-94). Keep `TestReadDirEntries` unchanged.

- [ ] **Step 2: Update tests — remove breadcrumb references from templates_test.go**

In `internal/server/templates_test.go`:

In `TestLoadTemplates_DefinedTemplates` (line 35), remove `"breadcrumbs"` from the expected template names list:
```go
for _, name := range []string{"layout", "sidebar_body", "note_pane_body", "index_card"} {
```

In `TestParsePage` (line 49), same change:
```go
for _, name := range []string{"layout", "sidebar_body", "note_pane_body", "index_card"} {
```

In `TestRenderView` (line 98-107), remove the `Breadcrumbs` field from the `IndexCard` literal:
```go
IndexCard: &IndexCard{
    Mode: "dir",
    Entries: []IndexEntry{
        {Name: "test.md", IsDir: false, Href: "/view/notes/test.md"},
    },
},
```

In `TestRenderSidebarPartial` (line 219-229), remove the `Breadcrumbs` field:
```go
IndexCard: &IndexCard{
    Mode: "dir",
    Entries: []IndexEntry{
        {Name: "readme.md", IsDir: false, Href: "/view/docs/readme.md"},
        {Name: "subdir", IsDir: true, Href: "/view/docs/subdir/"},
    },
},
```

Remove the breadcrumb assertion (line 244-246):
```go
// Remove this check:
// if !strings.Contains(body, "docs") {
//     t.Error("renderSidebarPartial: expected breadcrumb 'docs'")
// }
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -run 'TestBuildDir|TestLoadTemplates_Defined|TestParsePage|TestRenderView|TestRenderSidebarPartial' -v`

Expected: FAIL — code still references `BreadcrumbsData`, `Crumb`, breadcrumb builders, and `breadcrumbs.html`.

- [ ] **Step 4: Remove breadcrumb types and builders**

In `internal/server/templates.go`, remove the `Crumb` and `BreadcrumbsData` structs (lines 12-21) and the `Breadcrumbs` field from `IndexCard` (line 33):

```go
// IndexCard is the sidebar's data shape.
type IndexCard struct {
	Mode    string
	Entries []IndexEntry
	Empty   string
}
```

In `internal/server/chrome.go`, remove `buildDirBreadcrumbs` (lines 26-55), `buildTagBreadcrumbs` (lines 57-66), and `buildTagsListBreadcrumbs` (lines 68-74).

- [ ] **Step 5: Remove breadcrumbs.html template**

Delete the file `web/templates/breadcrumbs.html`.

In `internal/server/templates.go`, remove `"templates/breadcrumbs.html"` from the `partials` slice (line 86) and from the `parsePartial` function call (line 118):

```go
var partials = []string{
	"templates/layout.html",
	"templates/index_card.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}
```

```go
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS, "templates/"+name+".html", "templates/index_card.html")
}
```

- [ ] **Step 6: Remove breadcrumb header from index_card.html**

Replace the content of `web/templates/index_card.html` to remove the header with breadcrumbs. The template should just render the entry list:

```html
{{ define "index_card" }}
<div class="w-full">
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
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/></svg> {{ .Name }}</a>
    </li>
    {{ else if .IsDir }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-get="{{ .Href }}"
        hx-target="#sidebar"
        hx-swap="innerHTML"
        onclick="setSidebarDir('{{ .Href }}')"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 font-medium no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg> {{ .Name }}</a>
    </li>
    {{ else }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-boost="true"
        hx-target="#note-pane"
        hx-swap="innerHTML"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 13H8"/><path d="M16 13h-2"/><path d="M10 17H8"/><path d="M16 17h-2"/></svg> {{ .Name }}</a>
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

- [ ] **Step 7: Update handlers to stop passing Breadcrumbs**

In `internal/server/handlers.go`:

`buildDirIndex` (line 189-204) — remove the `Breadcrumbs` line:
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
		Mode:    "dir",
		Entries: entries,
		Empty:   "No files here.",
	}, nil
}
```

`handleDir` error case (line 281) — remove `Breadcrumbs`:
```go
card = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
```

`handleTags` (line 298-303) — remove `Breadcrumbs`:
```go
card := &IndexCard{
    Mode:    "tags",
    Entries: entries,
    Empty:   "No tags found.",
}
```

`handleTagNotes` (line 320-325) — remove `Breadcrumbs`:
```go
card := &IndexCard{
    Mode:    "tag",
    Entries: entries,
    Empty:   "No notes with this tag.",
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -v`

Expected: All tests PASS.

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "Remove breadcrumbs system from sidebar

Drop BreadcrumbsData, Crumb types, breadcrumb builder functions,
and breadcrumbs.html template. The tree navigation design replaces
breadcrumb-based navigation."
```

---

### Task 2: Create Shared Entry List Template

**Files:**
- Create: `web/templates/entry_list.html` — shared entry list partial
- Modify: `web/templates/index_card.html` — delegate to `entry_list`
- Modify: `internal/server/templates.go` — add `entry_list.html` to partials
- Test: `internal/server/templates_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/server/templates_test.go`, add a test that the `entry_list` template is defined:

```go
func TestLoadTemplates_EntryListDefined(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}
	if ts.view.Lookup("entry_list") == nil {
		t.Error("view template set missing 'entry_list'")
	}
	if ts.sidebar.Lookup("entry_list") == nil {
		t.Error("sidebar template set missing 'entry_list'")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -run TestLoadTemplates_EntryListDefined -v`

Expected: FAIL — `entry_list` template not found.

- [ ] **Step 3: Create entry_list.html**

Create `web/templates/entry_list.html`. This is the shared entry markup used by both sidebar and main panel. The `EntryContext` controls onclick behavior — sidebar entries use JS functions, main panel entries use the same functions (so clicking a dir in the main listing also expands it in the sidebar).

```html
{{ define "entry_list" }}
{{ if .Entries }}
<ul class="list-none m-0 p-0">
  {{ range .Entries }}
  {{ if .IsTag }}
  <li class="border-b border-gray-100 last:border-b-0">
    <a
      href="{{ .Href }}"
      onclick="selectTag('{{ .Name }}'); return false;"
      data-entry-type="tag"
      data-entry-name="{{ .Name }}"
      class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/></svg> {{ .Name }}</a>
  </li>
  {{ else if .IsDir }}
  <li class="border-b border-gray-100 last:border-b-0">
    <a
      href="{{ .Href }}"
      onclick="selectDir('{{ .Href }}'); return false;"
      data-entry-type="dir"
      data-entry-href="{{ .Href }}"
      class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 font-medium no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg> {{ .Name }}</a>
  </li>
  {{ else }}
  <li class="border-b border-gray-100 last:border-b-0">
    <a
      href="{{ .Href }}"
      onclick="selectNote('{{ .Href }}'); return false;"
      data-entry-type="note"
      data-entry-href="{{ .Href }}"
      class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:bg-blue-100 hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 13H8"/><path d="M16 13h-2"/><path d="M10 17H8"/><path d="M16 17h-2"/></svg> {{ .Name }}</a>
  </li>
  {{ end }}
  {{ end }}
</ul>
{{ else }}
<p class="px-4 py-6 text-gray-500 text-center">{{ .Empty }}</p>
{{ end }}
{{ end }}
```

- [ ] **Step 4: Add IsTag to IndexEntry and register entry_list.html**

In `internal/server/templates.go`, add `IsTag` field to `IndexEntry`:

```go
type IndexEntry struct {
	Name  string
	IsDir bool
	IsTag bool
	Href  string
}
```

Add `"templates/entry_list.html"` to the `partials` slice:

```go
var partials = []string{
	"templates/layout.html",
	"templates/index_card.html",
	"templates/entry_list.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}
```

Update `parsePartial` to include `entry_list.html`:

```go
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS, "templates/"+name+".html", "templates/index_card.html", "templates/entry_list.html")
}
```

- [ ] **Step 5: Update index_card.html to use entry_list**

Replace the entry list markup in `web/templates/index_card.html` with a call to the shared template:

```html
{{ define "index_card" }}
<div class="w-full">
  {{ template "entry_list" . }}
</div>
{{ end }}
```

- [ ] **Step 6: Set IsTag on tag entries in handlers**

In `internal/server/handlers.go`, `handleTags` — set `IsTag: true` on each entry:

```go
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags := s.tagIndex.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{
			Name:  tag,
			IsTag: true,
			Href:  "/tags/" + tagPath(tag),
		}
	}
	card := &IndexCard{
		Mode:    "tags",
		Entries: entries,
		Empty:   "No tags found.",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 7: Run tests**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -v`

Expected: All tests PASS including the new `TestLoadTemplates_EntryListDefined`.

- [ ] **Step 8: Commit**

```bash
git add -A && git commit -m "Extract shared entry_list template

Create entry_list.html as a shared partial for rendering file/dir/tag
entries identically in both sidebar and main panel. Add IsTag field
to IndexEntry."
```

---

### Task 3: Create Sidebar Tree Template

**Files:**
- Create: `web/templates/sidebar_tree.html` — two-section sidebar with FILES/TAGS roots
- Modify: `web/templates/sidebar_body.html` — render the tree instead of index_card
- Modify: `internal/server/templates.go` — add `SidebarTreeData`, update `SidebarPartialData`, add template registration
- Modify: `internal/server/handlers.go` — update `handleView` and `handleRoot` to pass tree data
- Test: `internal/server/templates_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/server/templates_test.go`:

```go
func TestRenderSidebarPartial_Tree(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{
		FilesEntries: []IndexEntry{
			{Name: "notes", IsDir: true, Href: "/dir/notes"},
			{Name: "README.md", IsDir: false, Href: "/view/README.md"},
		},
		TagsEntries: []IndexEntry{
			{Name: "golang", IsTag: true, Href: "/tags/golang"},
			{Name: "til", IsTag: true, Href: "/tags/til"},
		},
	}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() error: %v", err)
	}

	body := buf.String()
	checks := []struct {
		label    string
		contains string
	}{
		{"files section", "FILES"},
		{"tags section", "TAGS"},
		{"dir entry", "notes"},
		{"file entry", "README.md"},
		{"tag entry", "golang"},
		{"files-content target", `id="files-content"`},
		{"tags-content target", `id="tags-content"`},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("expected %s (%q) in output, got: %s", c.label, c.contains, body)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -run TestRenderSidebarPartial_Tree -v`

Expected: FAIL — `SidebarPartialData` doesn't have the new fields.

- [ ] **Step 3: Update SidebarPartialData and templates.go**

In `internal/server/templates.go`, replace `SidebarPartialData`:

```go
// SidebarPartialData is the render context for the sidebar tree.
// On full-page loads it carries both sections' entries. On HTMX
// partial refreshes only one section is populated.
type SidebarPartialData struct {
	// Full-tree fields (used on full-page render).
	FilesEntries []IndexEntry
	TagsEntries  []IndexEntry
	FilesEmpty   string
	TagsEmpty    string

	// Legacy single-card field (kept temporarily for partial responses).
	IndexCard *IndexCard
}
```

Add `"templates/sidebar_tree.html"` to the `partials` slice:

```go
var partials = []string{
	"templates/layout.html",
	"templates/index_card.html",
	"templates/entry_list.html",
	"templates/sidebar_tree.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}
```

Update `parsePartial` to include the new templates:

```go
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS,
		"templates/"+name+".html",
		"templates/index_card.html",
		"templates/entry_list.html",
		"templates/sidebar_tree.html",
	)
}
```

- [ ] **Step 4: Create sidebar_tree.html**

Create `web/templates/sidebar_tree.html`:

```html
{{ define "sidebar_tree" }}
<section id="files-section">
  <button type="button" onclick="toggleSection('files')" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="files-disclosure" class="text-[10px] leading-none">&#9662;</span>
    FILES
  </button>
  <div id="files-content">
    {{ template "entry_list" (dict "Entries" .FilesEntries "Empty" .FilesEmpty) }}
  </div>
</section>

<hr class="border-gray-200 m-0" />

<section id="tags-section">
  <button type="button" onclick="toggleSection('tags')" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="tags-disclosure" class="text-[10px] leading-none">&#9662;</span>
    TAGS
  </button>
  <div id="tags-content">
    {{ template "entry_list" (dict "Entries" .TagsEntries "Empty" .TagsEmpty) }}
  </div>
</section>
{{ end }}
```

Wait — Go's `html/template` doesn't have a built-in `dict` function. We need to pass structured data. Instead, use two `IndexCard`-like structs or pass data through the existing shape. Let's use a simpler approach — the `entry_list` template expects `.Entries` and `.Empty`, so we need a wrapper.

Revised `sidebar_tree.html`:

```html
{{ define "sidebar_tree" }}
<section id="files-section">
  <button type="button" onclick="toggleSection('files')" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="files-disclosure" class="text-[10px] leading-none">&#9662;</span>
    FILES
  </button>
  <div id="files-content">
    {{ template "entry_list" .Files }}
  </div>
</section>

<hr class="border-gray-200 m-0" />

<section id="tags-section">
  <button type="button" onclick="toggleSection('tags')" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="tags-disclosure" class="text-[10px] leading-none">&#9662;</span>
    TAGS
  </button>
  <div id="tags-content">
    {{ template "entry_list" .Tags }}
  </div>
</section>
{{ end }}
```

And update `SidebarPartialData` to use `IndexCard` sub-structs:

```go
type SidebarPartialData struct {
	Files *IndexCard // FILES section
	Tags  *IndexCard // TAGS section
}
```

- [ ] **Step 5: Update SidebarPartialData (revised)**

In `internal/server/templates.go`, replace `SidebarPartialData` with the cleaner version:

```go
// SidebarPartialData is the render context for the sidebar tree.
type SidebarPartialData struct {
	Files *IndexCard // FILES section entries
	Tags  *IndexCard // TAGS section entries
}
```

- [ ] **Step 6: Update sidebar_body.html**

Replace the content of `web/templates/sidebar_body.html`:

```html
{{ define "sidebar_body" }}
{{ template "sidebar_tree" . }}
{{ end }}
```

- [ ] **Step 7: Update the test to match revised data shape**

Update the test from step 1 to use the revised `SidebarPartialData`:

```go
func TestRenderSidebarPartial_Tree(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{
		Files: &IndexCard{
			Mode: "dir",
			Entries: []IndexEntry{
				{Name: "notes", IsDir: true, Href: "/dir/notes"},
				{Name: "README.md", IsDir: false, Href: "/view/README.md"},
			},
			Empty: "No files here.",
		},
		Tags: &IndexCard{
			Mode: "tags",
			Entries: []IndexEntry{
				{Name: "golang", IsTag: true, Href: "/tags/golang"},
				{Name: "til", IsTag: true, Href: "/tags/til"},
			},
			Empty: "No tags found.",
		},
	}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() error: %v", err)
	}

	body := buf.String()
	checks := []struct {
		label    string
		contains string
	}{
		{"files heading", "FILES"},
		{"tags heading", "TAGS"},
		{"dir entry", "notes"},
		{"file entry", "README.md"},
		{"tag entry", "golang"},
		{"files-content target", `id="files-content"`},
		{"tags-content target", `id="tags-content"`},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("expected %s (%q) in output", c.label, c.contains)
		}
	}
}
```

- [ ] **Step 8: Update existing tests for new SidebarPartialData shape**

In `internal/server/templates_test.go`:

Update `TestRenderSidebarPartial` to use the new shape:

```go
func TestRenderSidebarPartial(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{
		Files: &IndexCard{
			Mode: "dir",
			Entries: []IndexEntry{
				{Name: "readme.md", IsDir: false, Href: "/view/docs/readme.md"},
				{Name: "subdir", IsDir: true, Href: "/dir/docs/subdir"},
			},
			Empty: "No files here.",
		},
		Tags: &IndexCard{
			Mode:  "tags",
			Empty: "No tags found.",
		},
	}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() error: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "readme.md") {
		t.Error("renderSidebarPartial: expected file entry 'readme.md'")
	}
	if !strings.Contains(body, "subdir") {
		t.Error("renderSidebarPartial: expected directory entry 'subdir'")
	}
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("renderSidebarPartial should not contain full HTML document")
	}
}
```

Update `TestRenderSidebarPartial_NilIndexCard` to the new shape:

```go
func TestRenderSidebarPartial_NilSections(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() with nil sections error: %v", err)
	}
}
```

Update `TestRenderView` — the `ViewData.IndexCard` field will be replaced. For now, update the `IndexCard` in the test to be wrapped properly. This depends on how `ViewData` changes. In this task we update `ViewData` to carry `SidebarPartialData` instead of `*IndexCard`:

In `internal/server/templates.go`, replace `IndexCard *IndexCard` in `ViewData` with sidebar data:

```go
type ViewData struct {
	layoutFields
	NotePath    string
	NoteTitle   string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
	Sidebar     SidebarPartialData
}
```

Update `TestRenderView` accordingly (the `IndexCard` field becomes `Sidebar`):

```go
Sidebar: SidebarPartialData{
    Files: &IndexCard{
        Mode: "dir",
        Entries: []IndexEntry{
            {Name: "test.md", IsDir: false, Href: "/view/notes/test.md"},
        },
        Empty: "No files here.",
    },
    Tags: &IndexCard{
        Mode:  "tags",
        Empty: "No tags found.",
    },
},
```

- [ ] **Step 9: Update layout.html to pass Sidebar data**

In `web/templates/layout.html`, change the sidebar template invocation from:

```html
{{ template "sidebar_body" . }}
```

to:

```html
{{ template "sidebar_body" .Sidebar }}
```

- [ ] **Step 10: Update handlers to build SidebarPartialData**

In `internal/server/handlers.go`:

Update `handleView` (full-page render section, around line 147-165). Replace the single `buildDirIndex` call with building both sections:

```go
// Full page: build sidebar tree with both sections.
sidebarDir := currentDir
lf := s.buildLayoutFields(title, editPath)

filesCard, err := s.buildDirIndex(sidebarDir)
if err != nil {
    s.logger.Warn("sidebar files build failed", "dir", sidebarDir, "err", err)
    filesCard = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
}

tagsCard := s.buildTagsIndex()

view := ViewData{
    layoutFields: lf,
    NotePath:     reqPath,
    NoteTitle:    noteTitle,
    Frontmatter:  fm,
    HTML:         template.HTML(html),
    SSEWatch:     viewSSEWatch(reqPath),
    ViewHref:     "/view/" + viewPath(reqPath),
    Sidebar: SidebarPartialData{
        Files: filesCard,
        Tags:  tagsCard,
    },
}
```

Update `handleRoot` (around line 64-81) similarly:

```go
lf := s.buildLayoutFields("", "")
filesCard, err := s.buildDirIndex("")
if err != nil {
    s.logger.Warn("sidebar build failed", "dir", "", "err", err)
    filesCard = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
}
s.index.Rebuild()
tagsCard := s.buildTagsIndex()

view := ViewData{
    layoutFields: lf,
    NotePath:     "",
    HTML:         template.HTML(`<p class="text-gray-500 text-center py-8">No note selected.</p>`),
    ViewHref:     "/view/",
    Sidebar: SidebarPartialData{
        Files: filesCard,
        Tags:  tagsCard,
    },
}
```

Add a `buildTagsIndex` helper:

```go
func (s *Server) buildTagsIndex() *IndexCard {
	tags := s.tagIndex.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{
			Name:  tag,
			IsTag: true,
			Href:  "/tags/" + tagPath(tag),
		}
	}
	return &IndexCard{
		Mode:    "tags",
		Entries: entries,
		Empty:   "No tags found.",
	}
}
```

- [ ] **Step 11: Update sidebar HTMX partial handlers**

The `handleDir`, `handleTags`, `handleTagNotes` handlers currently return full `SidebarPartialData`. They now need to return just the entry list for their specific section (targeted at `#files-content` or `#tags-content`). For now, keep them rendering `index_card` as a section-content partial. Update them to render the `entry_list` template directly.

Add a new render method to `templateSet` in `templates.go`:

```go
func (t *templateSet) renderEntryList(w io.Writer, data *IndexCard) error {
	return t.sidebar.ExecuteTemplate(w, "entry_list", data)
}
```

Update `handleDir`:

```go
func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dirPath := r.PathValue("path")

	card, err := s.buildDirIndex(dirPath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", dirPath, "err", err)
		card = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Update `handleTags`:

```go
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	card := s.buildTagsIndex()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Update `handleTagNotes`:

```go
func (s *Server) handleTagNotes(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	notes := s.tagIndex.NotesByTag(tag)
	entries := make([]IndexEntry, len(notes))
	for i, notePath := range notes {
		entries[i] = IndexEntry{
			Name: notePath,
			Href: "/view/" + viewPath(notePath),
		}
	}
	card := &IndexCard{
		Mode:    "tag",
		Entries: entries,
		Empty:   "No notes with this tag.",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 12: Update handler tests for new response shape**

The sidebar handler tests (`TestDirHandler`, `TestDirHandlerRoot`, `TestTagsHandler`, `TestTagNotesHandler`, `TestTagNotesHandlerUnknownTag`) check that responses contain entry names — these should still pass since the entry list markup still contains those names. Run them to verify.

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -v`

Expected: All tests PASS.

- [ ] **Step 13: Commit**

```bash
git add -A && git commit -m "Add sidebar tree with FILES and TAGS sections

Replace the single IndexCard sidebar with a two-section tree layout.
Full-page renders include both sections. HTMX partials return entry
lists targeted at individual section containers."
```

---

### Task 4: Add Directory/Tag Listing to Main Panel

**Files:**
- Create: `web/templates/dir_listing.html` — main panel template for directory/tag listings
- Modify: `internal/server/templates.go` — add `DirListingData`, add `renderDirListing` method, register template
- Modify: `internal/server/handlers.go` — `handleDir`/`handleTags`/`handleTagNotes` respond to `HX-Target: note-pane`
- Test: `internal/server/handlers_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/server/handlers_test.go`:

```go
func TestDirHandler_NotePanePartial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/dir/2026", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "03") {
		t.Errorf("expected subdirectory '03' in listing, got: %s", body)
	}
	if !strings.Contains(body, `id="dir-listing"`) {
		t.Errorf("expected dir-listing container, got: %s", body)
	}
}

func TestTagsHandler_NotePanePartial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/tags/todo", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "20260331_9201_todo.md") {
		t.Errorf("expected note in tag listing, got: %s", body)
	}
	if !strings.Contains(body, `id="dir-listing"`) {
		t.Errorf("expected dir-listing container, got: %s", body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -run 'TestDirHandler_NotePanePartial|TestTagsHandler_NotePanePartial' -v`

Expected: FAIL — handlers don't respond to `HX-Target: note-pane` with a listing template.

- [ ] **Step 3: Create dir_listing.html**

Create `web/templates/dir_listing.html`. This is the main panel wrapper that renders the same `entry_list` partial, styled identically to the sidebar entries:

```html
{{ define "dir_listing" }}
<div id="dir-listing" class="mx-auto max-w-[900px] border border-gray-200 rounded-md bg-white">
  {{ if .Title }}
  <header class="flex items-center px-6 py-3 border-b border-gray-200">
    <span class="text-sm text-gray-500 font-medium truncate">{{ .Title }}</span>
  </header>
  {{ end }}
  <div>
    {{ template "entry_list" .IndexCard }}
  </div>
</div>
{{ end }}
```

- [ ] **Step 4: Add DirListingData and register template**

In `internal/server/templates.go`, add:

```go
// DirListingData is the render context for a directory or tag listing
// shown in the main panel.
type DirListingData struct {
	Title     string
	IndexCard *IndexCard
}
```

Add `"templates/dir_listing.html"` to the `partials` slice:

```go
var partials = []string{
	"templates/layout.html",
	"templates/index_card.html",
	"templates/entry_list.html",
	"templates/sidebar_tree.html",
	"templates/dir_listing.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}
```

Add a `dirListing` template to the `templateSet` and update `loadTemplates`:

```go
type templateSet struct {
	view       *template.Template
	sidebar    *template.Template
	note       *template.Template
	dirListing *template.Template
}
```

```go
func loadTemplates() (*templateSet, error) {
	view, err := parsePage("templates/view.html")
	if err != nil {
		return nil, fmt.Errorf("parse view template: %w", err)
	}
	sidebar, err := parsePartial("sidebar_body")
	if err != nil {
		return nil, fmt.Errorf("parse sidebar partial: %w", err)
	}
	note, err := parsePartial("note_pane_body")
	if err != nil {
		return nil, fmt.Errorf("parse note-pane partial: %w", err)
	}
	dirListing, err := parsePartial("dir_listing")
	if err != nil {
		return nil, fmt.Errorf("parse dir-listing partial: %w", err)
	}
	return &templateSet{view: view, sidebar: sidebar, note: note, dirListing: dirListing}, nil
}
```

Update `parsePartial` to include all shared templates:

```go
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS,
		"templates/"+name+".html",
		"templates/index_card.html",
		"templates/entry_list.html",
		"templates/sidebar_tree.html",
		"templates/dir_listing.html",
	)
}
```

Add render method:

```go
func (t *templateSet) renderDirListing(w io.Writer, data DirListingData) error {
	return t.dirListing.ExecuteTemplate(w, "dir_listing", data)
}
```

- [ ] **Step 5: Update handlers to respond to note-pane target**

In `internal/server/handlers.go`, update `handleDir`:

```go
func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dirPath := r.PathValue("path")

	card, err := s.buildDirIndex(dirPath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", dirPath, "err", err)
		card = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		title := dirPath
		if title == "" {
			title = "/"
		}
		if err := s.templates.renderDirListing(w, DirListingData{Title: title, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Update `handleTagNotes`:

```go
func (s *Server) handleTagNotes(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	notes := s.tagIndex.NotesByTag(tag)
	entries := make([]IndexEntry, len(notes))
	for i, notePath := range notes {
		entries[i] = IndexEntry{
			Name: notePath,
			Href: "/view/" + viewPath(notePath),
		}
	}
	card := &IndexCard{
		Mode:    "tag",
		Entries: entries,
		Empty:   "No notes with this tag.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: tag, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

Update `handleTags` similarly (for when a tag list is shown in main panel):

```go
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	card := s.buildTagsIndex()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: "Tags", IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if err := s.templates.renderEntryList(w, card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 6: Run all tests**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -v`

Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "Add directory/tag listing in main panel

Handlers respond to HX-Target: note-pane with a dir_listing template
that reuses the shared entry_list partial, so listings look identical
in sidebar and main panel."
```

---

### Task 5: Frontend JavaScript — Tree Navigation

**Files:**
- Modify: `web/src/app.js` — replace sidebar mode logic with tree section/drill/select functions

- [ ] **Step 1: Rewrite app.js sidebar logic**

Replace the entire sidebar mode section of `web/src/app.js` (everything after `// --- Sidebar toggle ---` section, from line 51 onwards) with:

```js
// --- Sidebar tree state ---

function getLS(key, fallback) {
  try { return localStorage.getItem('notesview.' + key) || fallback; } catch (e) { return fallback; }
}

function setLS(key, value) {
  try { localStorage.setItem('notesview.' + key, value); } catch (e) {}
}

function encodePath(p) {
  if (!p) return '';
  return p.split('/').map(encodeURIComponent).join('/');
}

// --- Section collapse/expand ---

window.toggleSection = function(name) {
  var content = document.getElementById(name + '-content');
  var disclosure = document.getElementById(name + '-disclosure');
  if (!content) return;
  var isOpen = content.style.display !== 'none';
  content.style.display = isOpen ? 'none' : '';
  if (disclosure) disclosure.textContent = isOpen ? '\u25B8' : '\u25BE';
  setLS(name + 'Open', isOpen ? '0' : '1');
};

function restoreSectionState(name) {
  var open = getLS(name + 'Open', '1');
  var content = document.getElementById(name + '-content');
  var disclosure = document.getElementById(name + '-disclosure');
  if (!content) return;
  if (open === '0') {
    content.style.display = 'none';
    if (disclosure) disclosure.textContent = '\u25B8';
  }
}

// --- Selection highlight ---

function clearSelected() {
  document.querySelectorAll('.entry-link.selected').forEach(function(el) {
    el.classList.remove('selected', 'bg-blue-50', 'border-blue-200');
  });
}

function markSelected(selector) {
  clearSelected();
  var el = document.querySelector(selector);
  if (el) el.classList.add('selected', 'bg-blue-50', 'border-blue-200');
}

// --- Directory navigation ---

window.selectDir = function(href) {
  setLS('filesDir', href);
  setLS('selected', href);

  // Load listing in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Load entries in sidebar files section
  htmx.ajax('GET', href, {
    target: '#files-content',
    swap: 'innerHTML',
  });

  // Ensure files section is visible
  var content = document.getElementById('files-content');
  var disclosure = document.getElementById('files-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('filesOpen', '1');
};

// --- Tag navigation ---

window.selectTag = function(tag) {
  var href = '/tags/' + encodeURIComponent(tag);
  setLS('tagsTag', tag);
  setLS('selected', href);

  // Load listing in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Load entries in sidebar tags section
  htmx.ajax('GET', href, {
    target: '#tags-content',
    swap: 'innerHTML',
  });

  // Ensure tags section is visible
  var content = document.getElementById('tags-content');
  var disclosure = document.getElementById('tags-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('tagsOpen', '1');
};

// --- Note navigation ---

window.selectNote = function(href) {
  setLS('selected', href);

  // Load note in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });
};

// --- Restore state ---

function refreshSidebar() {
  var filesDir = getLS('filesDir', '');
  var filesUrl = '/dir/' + encodePath(filesDir);
  htmx.ajax('GET', filesUrl, {
    target: '#files-content',
    swap: 'innerHTML',
  });

  var tagsTag = getLS('tagsTag', '');
  var tagsUrl = tagsTag ? '/tags/' + encodeURIComponent(tagsTag) : '/tags';
  htmx.ajax('GET', tagsUrl, {
    target: '#tags-content',
    swap: 'innerHTML',
  });
}

function restoreSidebarState() {
  restoreSectionState('files');
  restoreSectionState('tags');
}

// --- Selection highlight after HTMX swaps ---

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target);

  // Re-apply selection highlight after any swap
  var selected = getLS('selected', '');
  if (selected) {
    setTimeout(function() {
      markSelected('[data-entry-href="' + selected + '"]');
    }, 0);
  }
});
```

Keep the existing DOMContentLoaded handler, highlight function, and sidebar toggle logic (lines 1-49) unchanged. Update the `DOMContentLoaded` handler to call the new `restoreSidebarState`:

```js
document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document);
  wireSidebarToggle();
  restoreSidebarState();
});
```

This is already the same, so no change needed to that line.

- [ ] **Step 2: Build the frontend to verify no syntax errors**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && npm run build --prefix web`

Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/app.js && git commit -m "Rewrite sidebar JS for tree navigation

Replace mode-toggle logic with section collapse/expand, directory
drill-down, tag drill-down, note selection, and selected-state
highlighting."
```

---

### Task 6: CSS for Selected State and Section Headings

**Files:**
- Modify: `web/src/style.css` — add selected-state styles

- [ ] **Step 1: Add selected-state CSS**

In `web/src/style.css`, add to `@source inline(...)` the new classes that appear in JS but not templates:

```css
@source inline('broken-link uid-link task-checked task-unchecked task-tag selected bg-blue-50 border-blue-200');
```

No additional CSS rules needed — the selection classes (`bg-blue-50`, `border-blue-200`) are Tailwind utilities applied via JS. The section heading styles are all inline Tailwind classes in `sidebar_tree.html`.

- [ ] **Step 2: Build to verify**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && npm run build --prefix web`

Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/style.css && git commit -m "Add selected-state Tailwind classes to safelist"
```

---

### Task 7: Update Tag Clicks in Note Pane

**Files:**
- Modify: `web/templates/note_pane_body.html:37` — update tag click handler

- [ ] **Step 1: Update tag onclick handler**

In `web/templates/note_pane_body.html`, line 37, change the tag links from:

```html
onclick="setSidebarTag('{{ . }}')"
```

to:

```html
onclick="selectTag('{{ . }}'); return false;"
```

Also remove the `hx-get`, `hx-target`, and `hx-swap` attributes since `selectTag` handles the HTMX calls itself:

Replace the full tag `<a>` element:

```html
{{ range .Tags }}<li><a href="/tags/{{ urlquery . }}" onclick="selectTag('{{ . }}'); return false;" class="inline-flex items-center bg-blue-100 text-blue-600 text-xs font-medium px-2 py-0.5 rounded-full leading-relaxed no-underline cursor-pointer hover:bg-blue-200">{{ . }}</a></li>{{ end }}
```

- [ ] **Step 2: Run all backend tests**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./internal/server/ -v`

Expected: PASS. The handler tests that check for tag names in output should still pass.

- [ ] **Step 3: Commit**

```bash
git add web/templates/note_pane_body.html && git commit -m "Update note pane tag clicks to use selectTag

Tags in rendered notes now trigger the tree navigation selectTag()
function instead of the removed setSidebarTag()."
```

---

### Task 8: Clean Up Dead Code

**Files:**
- Modify: `internal/server/templates.go` — remove `IndexCard.Mode` if unused, remove old `index_card.html` if fully replaced
- Modify: `internal/server/templates_test.go` — clean up any remaining references to old shapes

- [ ] **Step 1: Check for dead code**

`IndexCard.Mode` is still used by `entry_list.html` — check if the template references it. Looking at the `entry_list.html` we created: it uses `IsTag`, `IsDir` on each entry, not `$.Mode`. So `Mode` is no longer referenced in templates. However, it's still set in handler code. Remove it from the struct if nothing reads it.

Check: does any template or test reference `.Mode`? If not, remove it from `IndexCard`:

```go
type IndexCard struct {
	Entries []IndexEntry
	Empty   string
}
```

Update all places that set `Mode` in handlers and tests.

- [ ] **Step 2: Remove index_card.html if fully replaced**

`index_card.html` now just delegates to `entry_list`. Check if anything references the `index_card` template name. If `sidebar_tree.html` and `dir_listing.html` both use `entry_list` directly, and nothing calls `index_card`, remove the template file and its registration.

If anything still references it, keep it as the thin wrapper.

- [ ] **Step 3: Run all tests**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./... -v`

Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "Remove dead code from sidebar refactor

Clean up unused Mode field, unreferenced templates, and any
remaining artifacts from the old sidebar design."
```

---

### Task 9: Integration Test and Manual Verification

**Files:**
- Test: `internal/server/handlers_test.go`

- [ ] **Step 1: Run the full test suite**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go test ./... -v`

Expected: All tests PASS.

- [ ] **Step 2: Build frontend**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && npm run build --prefix web`

Expected: Build succeeds.

- [ ] **Step 3: Start the dev server and test manually**

Run: `cd /Users/alex/src/notes-view/.claude/worktrees/declarative-gliding-kernighan && go run ./cmd/notesview`

Verify in browser:
1. Sidebar shows FILES and TAGS sections with labeled headings.
2. Clicking a directory expands it in the sidebar AND shows listing in main panel.
3. Clicking a note opens it in main panel, highlights it in sidebar.
4. Clicking a tag expands it in sidebar AND shows tag's notes in main panel.
5. Tag pills in note content trigger `selectTag` and navigate correctly.
6. Section collapse/expand works and persists across reload.
7. Selected state persists across reload.
8. Entry lists look identical in sidebar and main panel.

- [ ] **Step 4: Final commit if any fixes needed**

Only if manual testing reveals issues. Fix and commit each fix separately.
