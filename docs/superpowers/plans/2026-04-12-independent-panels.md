# Independent Sidebar and Note Panels — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the view into two independently-scrolling HTMX regions (`#sidebar`, `#note-pane`) with surgical swaps, replace the renderer's regex-over-HTML link rewriter with a goldmark extension, and move sidebar visibility to client-side state.

**Architecture:** Three fixed viewport regions (topbar, sidebar, note-pane). Internal links carry per-link `hx-boost="true"` + `hx-target="#sidebar"` or `hx-target="#note-pane"`; external links are plain HTML. The renderer uses a goldmark extension (AST transformer for `note://` and relative `.md`, inline parser for bare UIDs, custom link `NodeRenderer` for HTMX attributes). The server dispatches full-page vs partial responses on the `HX-Target` request header.

**Tech Stack:** Go 1.25, goldmark v1.8.2, HTMX + SSE extension, Tailwind v4, `html/template`.

**Design spec:** `docs/superpowers/specs/2026-04-12-independent-panels-design.md`

---

## File Map

**Created:**
- `internal/renderer/noteext.go` — goldmark extension (transformer, UID inline parser, link renderer)
- `internal/renderer/noteext_test.go` — extension unit + conversion tests (replaces `notelinks_test.go`)
- `web/templates/sidebar_body.html` — inner HTML of `#sidebar`, the sidebar-partial response
- `web/templates/note_pane_body.html` — inner HTML of `#note-pane`, the note-partial response

**Deleted:**
- `internal/renderer/notelinks.go` — regex pipeline replaced by extension
- `internal/renderer/notelinks_test.go` — replaced by `noteext_test.go`
- `web/templates/browse.html` — standalone-panel page collapses into the two-pane layout
- `web/templates/view.html` — replaced by the layout composition

**Modified:**
- `internal/renderer/renderer.go` — register extension, pass per-request state via parser.Context, remove post-process pipeline
- `internal/server/handlers.go` — HX-Target dispatch in `handleView`, new `handleSidebar`, empty-state for `handleRoot`
- `internal/server/chrome.go` — rename `indexQuery`→`dirQuery`, simplify `dirLinkHref`/`fileLinkHref`, delete `toggleHref`/`indexState`
- `internal/server/templates.go` — drop `IndexOpen`/`ShowToggle`/`ToggleHref`, rename `IndexQuery`→`DirQuery`, add partial render methods
- `internal/server/handlers_test.go` — rewrite tests for the new model
- `web/templates/layout.html` — three fixed regions, no body-level hx-boost, hamburger button, pre-paint script
- `web/templates/index_card.html` — add `hx-boost="true"` + explicit `hx-target` per link
- `web/templates/breadcrumbs.html` — add `hx-boost="true"` + `hx-target="#sidebar"` per link
- `web/src/app.js` — sidebar toggle handler, localStorage, aria updates, htmx.ajax refresh
- `web/src/style.css` — independent scroll containers, `.sidebar-open` class behavior
- `integration_test.go` — update to the new URL shape (`?dir=` not `?index=dir`)

---

## Task 1: Renderer — update tests for goldmark extension (TDD red)

**Files:**
- Create: `internal/renderer/noteext_test.go`
- Delete: `internal/renderer/notelinks_test.go`

- [ ] **Step 1: Delete the old test file**

```bash
git rm internal/renderer/notelinks_test.go
```

- [ ] **Step 2: Write the new test file**

```go
// internal/renderer/noteext_test.go
package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreikanter/notesview/internal/index"
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
	html, _, err := r.Render([]byte(`See [my todo](note://20260331_9201) for details.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("note:// link not resolved:\n%s", html)
	}
	if !strings.Contains(html, `hx-boost="true"`) || !strings.Contains(html, `hx-target="#note-pane"`) {
		t.Errorf("note:// link missing HTMX attrs:\n%s", html)
	}
}

func TestBrokenNoteLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [missing](note://99999999_0000) link.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="broken-link"`) {
		t.Errorf("broken note:// link not marked:\n%s", html)
	}
	if !strings.Contains(html, `href="#"`) {
		t.Errorf("broken link should href=\"#\":\n%s", html)
	}
	if strings.Contains(html, `hx-boost="true"`) {
		t.Errorf("broken link should not have hx-boost (href is #):\n%s", html)
	}
}

func TestAutoLinkUID(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`Refer to 20260331_9201 for the todo list.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("UID not auto-linked:\n%s", html)
	}
	if !strings.Contains(html, `class="uid-link"`) {
		t.Errorf("UID auto-link missing uid-link class:\n%s", html)
	}
	if !strings.Contains(html, `hx-boost="true"`) {
		t.Errorf("UID auto-link missing hx-boost:\n%s", html)
	}
}

func TestAutoLinkUIDNoMatch(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`Reference 99999999_0000 does not exist.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `<a href="/view/`) {
		t.Errorf("non-matching UID should not be linked:\n%s", html)
	}
}

func TestRelativeMdLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [other](../01/foo.md) for details.`), "2026/03", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/01/foo.md"`) {
		t.Errorf("relative .md link not rewritten:\n%s", html)
	}
	if !strings.Contains(html, `hx-boost="true"`) || !strings.Contains(html, `hx-target="#note-pane"`) {
		t.Errorf("relative .md link missing HTMX attrs:\n%s", html)
	}
}

// TestExternalLinksStayPlain pins the "external links are plain HTML"
// rule. No hx-boost, no hx-target, no HTMX attributes of any kind on
// links whose destination points outside the notes system.
func TestExternalLinksStayPlain(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `[web](https://example.com) [mail](mailto:a@b.com) [asset](/static/foo.png)`
	html, _, err := r.Render([]byte(input), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, href := range []string{`href="https://example.com"`, `href="mailto:a@b.com"`, `href="/static/foo.png"`} {
		if !strings.Contains(html, href) {
			t.Errorf("expected %s in output:\n%s", href, html)
		}
	}
	// Sanity-check: none of the external-link anchors carry hx-* attrs.
	// Strip all internal-link anchors first so we can assert over what's left.
	if strings.Contains(html, `href="https://example.com" hx-boost`) ||
		strings.Contains(html, `href="mailto:a@b.com" hx-boost`) ||
		strings.Contains(html, `href="/static/foo.png" hx-boost`) {
		t.Errorf("external link picked up hx-boost:\n%s", html)
	}
}

// TestDirQueryThreading pins the per-request state contract: when the
// renderer is given a dirQuery suffix, every internal /view/... href
// it emits must carry that suffix.
func TestDirQueryThreading(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [todo](note://20260331_9201), [rel](../03/20260330_9198.md), and bare 20260331_9201.`
	html, _, err := r.Render([]byte(input), "2026/03", "?dir=2026%2F03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03"`) {
		t.Errorf("note:// link dropped dirQuery:\n%s", html)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260330_9198.md?dir=2026%2F03"`) {
		t.Errorf("relative .md link dropped dirQuery:\n%s", html)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03" class="uid-link"`) &&
		!strings.Contains(html, `class="uid-link" href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03"`) {
		t.Errorf("bare UID auto-link dropped dirQuery (attr order may vary):\n%s", html)
	}
}
```

- [ ] **Step 3: Run the tests — they should fail to compile or fail**

```bash
go test ./internal/renderer/ -run 'TestNoteProtocolLink|TestBrokenNoteLink|TestAutoLinkUID|TestRelativeMdLink|TestExternalLinksStayPlain|TestDirQueryThreading' -v
```

Expected: FAIL (the old `processNoteLinks` may still pass some of them, but `TestExternalLinksStayPlain` and the new HTMX-attr assertions in `TestNoteProtocolLink`/etc. will fail). If the old code is still in place, this is the "red" state that Task 2–5 turn green.

- [ ] **Step 4: Commit the red state**

```bash
git add internal/renderer/noteext_test.go
git commit -m "Add failing tests for goldmark-extension renderer

Replaces the regex pipeline tests with assertions against the
new extension behavior: internal links carry hx-boost/hx-target,
external links stay plain, dirQuery threads through every pass."
```

---

## Task 2: Renderer — create goldmark extension skeleton with ASTTransformer

**Files:**
- Create: `internal/renderer/noteext.go`

- [ ] **Step 1: Create the new extension file**

```go
// internal/renderer/noteext.go
package renderer

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/dreikanter/notesview/internal/index"
)

// noteLinkStateKey identifies per-request state stored in parser.Context.
// The state travels from the Renderer.Render call into the AST transformer
// and UID inline parser during goldmark's parse phase, and is discarded
// when the request is done.
var noteLinkStateKey = parser.NewContextKey()

// noteLinkState is the per-request context the extension reads during
// parsing. currentDir is the note's parent directory (used to resolve
// relative .md links); dirQuery is the URL suffix ("?dir=..." or "")
// threaded into every rewritten internal href for panel state coherence.
type noteLinkState struct {
	idx        *index.Index
	currentDir string
	dirQuery   string
}

// noteLinkExtension wires the AST transformer, UID inline parser, and
// custom link renderer into a goldmark.Markdown instance. The Renderer
// constructor registers this as an extension, once, at startup.
type noteLinkExtension struct{}

// NoteLinkExtension is the registerable goldmark extension. There is no
// per-request configuration here — everything that varies per request
// lives in parser.Context (see noteLinkState).
var NoteLinkExtension goldmark.Extender = &noteLinkExtension{}

func (e *noteLinkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&noteLinkTransformer{}, 100),
		),
		parser.WithInlineParsers(
			util.Prioritized(&uidInlineParser{}, 100),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&noteLinkRenderer{}, 100),
		),
	)
}

// noteLinkTransformer walks the AST once after parsing, rewriting
// *ast.Link destinations for note:// and relative .md references.
// Bare-UID auto-linking is handled separately by uidInlineParser
// because it needs to inject new nodes during parsing.
type noteLinkTransformer struct{}

func (t *noteLinkTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	stateAny := pc.Get(noteLinkStateKey)
	if stateAny == nil {
		return
	}
	state := stateAny.(*noteLinkState)

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if link, ok := n.(*ast.Link); ok {
			rewriteLinkDestination(link, state)
		}
		return ast.WalkContinue, nil
	})
}

// rewriteLinkDestination mutates a single *ast.Link in place. It handles
// two internal-link shapes: note://UID and relative .md paths. Anything
// else — absolute URLs with a scheme, protocol-relative, root-absolute
// paths, non-.md — is left alone and will render as a plain <a href>
// because the custom renderer's internal-link check won't match.
func rewriteLinkDestination(n *ast.Link, s *noteLinkState) {
	dest := string(n.Destination)

	if strings.HasPrefix(dest, "note://") {
		uid := strings.TrimPrefix(dest, "note://")
		if relPath, ok := s.idx.Lookup(uid); ok {
			n.Destination = []byte("/view/" + relPath + s.dirQuery)
		} else {
			n.Destination = []byte("#")
			n.SetAttributeString("class", []byte("broken-link"))
			n.SetAttributeString("title", []byte(fmt.Sprintf("Note %s not found", uid)))
		}
		return
	}

	// Relative .md: must end in .md, not be absolute, not have a scheme.
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") {
		return
	}
	if !strings.HasSuffix(dest, ".md") {
		return
	}
	resolved := path.Clean(path.Join(s.currentDir, dest))
	resolved = strings.TrimPrefix(resolved, "/")
	n.Destination = []byte("/view/" + resolved + s.dirQuery)
}

// uidPattern matches a UID token. The inline parser uses it to decide
// whether the bytes at its trigger position actually form a UID.
var uidPattern = regexp.MustCompile(`^\d{8}_\d{4,}`)

// uidInlineParser is a goldmark InlineParser that recognizes bare UID
// tokens like "20260331_0001" in note text and, if the UID resolves in
// the index, emits a Link node pointing at the resolved note URL. It
// is triggered on every digit byte; the Parse method double-checks the
// full pattern and the word-boundary before consuming.
type uidInlineParser struct{}

func (p *uidInlineParser) Trigger() []byte {
	return []byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
}

func (p *uidInlineParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	stateAny := pc.Get(noteLinkStateKey)
	if stateAny == nil {
		return nil
	}
	state := stateAny.(*noteLinkState)
	if state.idx == nil {
		return nil
	}

	// Reject matches that aren't at a word boundary: if the byte
	// immediately before the current position is a word char (digit,
	// letter, or underscore), this UID-looking run is part of a larger
	// identifier and shouldn't be auto-linked.
	source := block.Source()
	pos := block.Position()
	if pos.Start > 0 {
		prev := source[pos.Start-1]
		if isWordByte(prev) {
			return nil
		}
	}

	line, _ := block.PeekLine()
	m := uidPattern.FindIndex(line)
	if m == nil {
		return nil
	}
	uid := string(line[m[0]:m[1]])

	// Reject matches followed by a word char (e.g. "20260101_12345foo").
	if m[1] < len(line) && isWordByte(line[m[1]]) {
		return nil
	}

	relPath, ok := state.idx.Lookup(uid)
	if !ok {
		return nil
	}

	block.Advance(m[1])
	link := ast.NewLink()
	link.Destination = []byte("/view/" + relPath + state.dirQuery)
	link.SetAttributeString("class", []byte("uid-link"))
	link.AppendChild(link, ast.NewString([]byte(uid)))
	return link
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
```

Note: the `noteLinkRenderer` type is added in Task 3. The file won't compile until then — that's OK because the next task is the renderer, and we commit both together only after Task 3.

- [ ] **Step 2: Do NOT commit yet**

The file won't build on its own. Move straight to Task 3.

---

## Task 3: Renderer — add custom link NodeRenderer

**Files:**
- Modify: `internal/renderer/noteext.go` (append the renderer type)

- [ ] **Step 1: Append the NodeRenderer to `noteext.go`**

Add this block at the end of `internal/renderer/noteext.go`:

```go
// noteLinkRenderer overrides goldmark's default *ast.Link renderer so
// we can emit HTMX attributes on internal links. External links (and
// anything else that didn't get rewritten to a /view/... destination)
// fall through as plain <a href>.
//
// This is a goldmark renderer.NodeRenderer; its RegisterFuncs
// method tells goldmark to call renderLink instead of the default
// when encountering *ast.Link nodes.
type noteLinkRenderer struct{}

func (r *noteLinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.renderLink)
}

func (r *noteLinkRenderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		w.WriteString(`<a href="`)
		w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
		w.WriteByte('"')

		if isInternalLink(n.Destination) {
			w.WriteString(` hx-boost="true" hx-target="#note-pane"`)
		}

		if class, ok := n.AttributeString("class"); ok {
			w.WriteString(` class="`)
			w.Write(util.EscapeHTML(class.([]byte)))
			w.WriteByte('"')
		}
		if title, ok := n.AttributeString("title"); ok {
			w.WriteString(` title="`)
			w.Write(util.EscapeHTML(title.([]byte)))
			w.WriteByte('"')
		} else if len(n.Title) > 0 {
			w.WriteString(` title="`)
			w.Write(util.EscapeHTML(n.Title))
			w.WriteByte('"')
		}
		w.WriteByte('>')
	} else {
		w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

// isInternalLink returns true if the destination points at a note the
// server knows how to serve (via /view/...). Broken note:// links have
// been rewritten to "#" and do NOT count as internal — they should
// render without hx-boost so clicking them is inert.
func isInternalLink(dest []byte) bool {
	return len(dest) >= len("/view/") && string(dest[:len("/view/")]) == "/view/"
}
```

- [ ] **Step 2: Verify the file builds**

```bash
go build ./internal/renderer/
```

Expected: builds cleanly. If there's a signature mismatch against goldmark v1.8.2 (e.g. `Position()` returning a different type, or `AttributeString` returning `any` vs `interface{}`), the compiler will tell you exactly where; adjust the type assertion. Goldmark's public API is stable at v1.x but the engineer should trust the compiler over this plan if they diverge.

- [ ] **Step 3: Do NOT commit yet — Renderer still calls the old processNoteLinks**

Move to Task 4.

---

## Task 4: Renderer — wire extension into Renderer, delete notelinks.go

**Files:**
- Modify: `internal/renderer/renderer.go`
- Delete: `internal/renderer/notelinks.go`

- [ ] **Step 1: Replace `internal/renderer/renderer.go` with the extension-aware version**

```go
package renderer

import (
	"bytes"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/dreikanter/notesview/internal/index"
)

var leadingH1 = regexp.MustCompile(`(?s)^\s*<h1[^>]*>\s*(.*?)\s*</h1>`)
var tagStripper = regexp.MustCompile(`<[^>]+>`)

type Frontmatter struct {
	Title       string   `yaml:"title"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
	Slug        string   `yaml:"slug"`
}

type Renderer struct {
	md    goldmark.Markdown
	index *index.Index
}

func NewRenderer(idx *index.Index) *Renderer {
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

// Render converts markdown to HTML. currentDir is the note's parent
// directory relative to the notes root (used to resolve `[text](./rel.md)`).
// dirQuery is appended verbatim to every internal /view/... href emitted
// by the goldmark extension, so the caller can thread the current
// sidebar directory through wiki-link navigation. Pass "" when there
// is no panel state to preserve.
func (r *Renderer) Render(source []byte, currentDir, dirQuery string) (string, *Frontmatter, error) {
	ctx := parser.NewContext()
	ctx.Set(noteLinkStateKey, &noteLinkState{
		idx:        r.index,
		currentDir: currentDir,
		dirQuery:   dirQuery,
	})

	var buf bytes.Buffer
	if err := r.md.Convert(source, &buf, parser.WithContext(ctx)); err != nil {
		return "", nil, err
	}

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
	html = processTaskSyntax(html)
	if fm != nil && fm.Title != "" {
		html = stripRedundantTitle(html, fm.Title)
	}
	return html, fm, nil
}

func stripRedundantTitle(html, title string) string {
	m := leadingH1.FindStringSubmatchIndex(html)
	if m == nil {
		return html
	}
	innerStart, innerEnd := m[2], m[3]
	plain := stdhtml.UnescapeString(tagStripper.ReplaceAllString(html[innerStart:innerEnd], ""))
	if strings.TrimSpace(plain) != strings.TrimSpace(title) {
		return html
	}
	return html[m[1]:]
}
```

- [ ] **Step 2: Delete `internal/renderer/notelinks.go`**

```bash
git rm internal/renderer/notelinks.go
```

- [ ] **Step 3: Run the renderer tests**

```bash
go test ./internal/renderer/ -v
```

Expected: all tests pass. If `TestAutoLinkUID` fails with attr-order issues in the uid-link class, adjust the assertion to match the actual output — the goldmark attribute API may emit `class` before `href` in some versions. Leave the *intent* of the test (href + class + hx-boost must all appear) intact.

- [ ] **Step 4: Run `go vet` and the full Go test suite**

```bash
go vet ./...
go test ./...
```

Expected: clean. The `server` package tests may still reference `?index=` URL shapes and will be updated in later tasks; for now they should still pass because we haven't touched `handlers.go` yet.

- [ ] **Step 5: Commit**

```bash
git add internal/renderer/
git commit -m "Replace regex link rewriter with goldmark extension

Introduces NoteLinkExtension: an AST transformer rewrites
note:// and relative .md destinations, a digit-triggered inline
parser auto-links bare UIDs, and a custom NodeRenderer for
ast.KindLink emits hx-boost/hx-target on internal links while
leaving external links plain. Per-request state flows via
parser.Context.

Deletes internal/renderer/notelinks.go and the regex pipeline."
```

---

## Task 5: Server — rename and simplify chrome.go helpers

**Files:**
- Modify: `internal/server/chrome.go`
- Modify: `internal/server/templates.go`

- [ ] **Step 1: Rewrite `internal/server/chrome.go`**

```go
package server

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dirQuery formats the canonical query suffix that carries the sidebar's
// sticky directory across links. Empty string means "no sticky directory"
// (the URL has no ?dir= at all). When non-empty the path is always
// explicit — callers resolve any default (note's parent directory)
// before constructing the query.
func dirQuery(path string) string {
	if path == "" {
		return "?dir="
	}
	return "?dir=" + url.QueryEscape(path)
}

// dirLinkHref builds an href that repositions the sidebar to a new
// directory while keeping the current note in view (sticky model).
// notePath is the note that should stay visible, or "" for the
// empty-state page where there's no note to keep.
func dirLinkHref(notePath, newDir string) string {
	q := dirQuery(newDir)
	if notePath == "" {
		return "/" + q
	}
	return "/view/" + notePath + q
}

// fileLinkHref builds an href that changes the note while keeping the
// sidebar on the same directory. The other half of the sticky model.
func fileLinkHref(filePath, sidebarDir string) string {
	return "/view/" + filePath + dirQuery(sidebarDir)
}

// buildBreadcrumbs constructs the sidebar header trail. Intermediate
// segments link back up the directory chain via dirLinkHref so a click
// only repositions the sidebar; the note is untouched. The final
// segment is marked Current (no link).
func buildBreadcrumbs(sidebarDir, notePath string) BreadcrumbsData {
	data := BreadcrumbsData{
		HomeHref: dirLinkHref(notePath, ""),
	}
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
			Href:  dirLinkHref(notePath, accumulated),
		})
	}
	return data
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. Directory entries link through dirLinkHref so the
// note stays put on click; file entries link through fileLinkHref so
// the sidebar stays put on click.
func readDirEntries(absPath, relPath, notePath string) ([]IndexEntry, error) {
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
			href = dirLinkHref(notePath, entryRel)
		} else {
			href = fileLinkHref(entryRel, relPath)
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

- [ ] **Step 2: Rewrite `internal/server/templates.go` — drop visibility fields, rename IndexQuery→DirQuery, add partial render methods**

Read `internal/server/templates.go` first if not already in context, then replace its contents with:

```go
package server

import (
	"fmt"
	"html/template"
	"io"

	"github.com/dreikanter/notesview/internal/renderer"
	"github.com/dreikanter/notesview/web"
)

type Crumb struct {
	Label   string
	Href    string
	Current bool
}

type BreadcrumbsData struct {
	HomeHref string
	Crumbs   []Crumb
}

type IndexEntry struct {
	Name  string
	IsDir bool
	Href  string
}

// IndexCard is the sidebar's data shape. Mode is kept as an extensibility
// hook for future non-directory sources (search, tag); today only "dir"
// is populated.
type IndexCard struct {
	Mode        string
	Breadcrumbs BreadcrumbsData
	Entries     []IndexEntry
	Empty       string
}

// layoutFields is the common chrome passed to every full-page render.
// DirQuery is the canonical "?dir=..." suffix appended to hrefs that
// need to preserve the sidebar's sticky directory (currently just the
// SSE live-reload hx-get URL).
type layoutFields struct {
	Title    string
	EditPath string
	EditHref string
	DirQuery string
}

// ViewData is the full-page render context for a note view.
type ViewData struct {
	layoutFields
	NotePath    string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
	IndexCard   *IndexCard
}

// NotePartialData is the render context for an HX-Target: note-pane
// partial response. Only the fields the note-pane template needs;
// no sidebar, no topbar.
type NotePartialData struct {
	NotePath    string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
	DirQuery    string
}

// SidebarPartialData is the render context for an HX-Target: sidebar
// partial response. Only the fields the sidebar-body template needs.
type SidebarPartialData struct {
	IndexCard *IndexCard
}

type templateSet struct {
	view    *template.Template
	sidebar *template.Template
	note    *template.Template
}

var partials = []string{
	"templates/layout.html",
	"templates/breadcrumbs.html",
	"templates/index_card.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}

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
	return &templateSet{view: view, sidebar: sidebar, note: note}, nil
}

func parsePage(page string) (*template.Template, error) {
	files := append([]string{}, partials...)
	files = append(files, page)
	return template.ParseFS(web.TemplatesFS, files...)
}

// parsePartial loads only the files needed to render one partial
// template, so a partial response doesn't accidentally include the
// full layout.
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS, "templates/"+name+".html", "templates/breadcrumbs.html", "templates/index_card.html")
}

func (t *templateSet) renderView(w io.Writer, data ViewData) error {
	return t.view.ExecuteTemplate(w, "layout", data)
}

func (t *templateSet) renderNotePartial(w io.Writer, data NotePartialData) error {
	return t.note.ExecuteTemplate(w, "note_pane_body", data)
}

func (t *templateSet) renderSidebarPartial(w io.Writer, data SidebarPartialData) error {
	return t.sidebar.ExecuteTemplate(w, "sidebar_body", data)
}
```

- [ ] **Step 3: Verify nothing compiles yet**

```bash
go build ./internal/server/
```

Expected: FAIL — `handlers.go` still references deleted types/functions. That's expected; the next task fixes it.

- [ ] **Step 4: Do NOT commit yet — handlers.go is broken**

Move to Task 6.

---

## Task 6: Server — rewrite handlers.go for HX-Target dispatch

**Files:**
- Modify: `internal/server/handlers.go` (replace handlers, keep editor-launch code)

- [ ] **Step 1: Replace the top half of `internal/server/handlers.go`**

Open `internal/server/handlers.go`. Replace everything from the top of the file through the end of `dirTitle` (end of the old `handleRoot`/`handleStandaloneIndex`/`handleView`/`buildDirIndex`/`dirTitle` block, i.e. up to the `handleRaw` function) with:

```go
package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// parseDirParam normalizes the ?dir=... query parameter. An empty
// string means "no sticky directory" (reopen defaults to the note's
// parent). A slash-trimmed non-empty value is the directory the sidebar
// should show.
func parseDirParam(r *http.Request) (dir string, hasDir bool) {
	raw, ok := r.URL.Query()["dir"]
	if !ok {
		return "", false
	}
	return strings.Trim(raw[0], "/"), true
}

// buildLayoutFields assembles the common chrome every full-page render
// needs. effectiveDir is the directory the sidebar is showing — already
// resolved from either ?dir= or a handler-specific default (the note's
// parent).
func (s *Server) buildLayoutFields(title, editPath, effectiveDir string) layoutFields {
	lf := layoutFields{
		Title:    title,
		EditPath: editPath,
		DirQuery: dirQuery(effectiveDir),
	}
	if editPath != "" {
		lf.EditHref = "/api/edit/" + editPath
	}
	return lf
}

// viewSSEWatch is the value for the sse-connect attribute on note_pane_body.
// The SSE URL needs the note path percent-encoded because file names may
// contain spaces, slashes, question marks, etc.
func viewSSEWatch(filePath string) string {
	return "/events?watch=" + url.QueryEscape(filePath)
}

// hxTargetedAt returns true if this is an HTMX request whose target is
// the named element id (without the leading "#"). HTMX sends
// HX-Target as the raw id value.
func hxTargetedAt(r *http.Request, id string) bool {
	if r.Header.Get("HX-Request") != "true" {
		return false
	}
	return r.Header.Get("HX-Target") == id
}

// handleRoot is the entry point for /. It redirects to README.md if
// one exists at the notes root. Otherwise it renders the two-pane
// layout with an empty-state placeholder where the note would be.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Sidebar partial response via HX-Target: sidebar on /
	if hxTargetedAt(r, "sidebar") {
		s.writeSidebarPartial(w, r, "", "")
		return
	}

	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}

	// Empty state: render the two-pane layout with no note.
	sidebarDir, _ := parseDirParam(r)
	lf := s.buildLayoutFields("", "", sidebarDir)
	card, err := s.buildDirIndex(sidebarDir, "")
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
	}
	go s.index.Build()

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

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Sidebar partial: don't read the file at all.
	if hxTargetedAt(r, "sidebar") {
		explicitDir, _ := parseDirParam(r)
		if explicitDir == "" {
			explicitDir = noteParentDir(reqPath)
		}
		s.writeSidebarPartial(w, r, explicitDir, reqPath)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if hxTargetedAt(r, "note-pane") {
				// Empty-state partial with HTTP 200 so HTMX swaps it in.
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

	// Resolve the sidebar's sticky directory. ?dir= wins when present;
	// otherwise default to the note's parent.
	explicitDir, hasDir := parseDirParam(r)
	sidebarDir := currentDir
	if hasDir {
		sidebarDir = explicitDir
	}
	dq := dirQuery(sidebarDir)

	html, fm, err := s.renderer.Render(data, currentDir, dq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	title := filepath.Base(reqPath)
	if fm != nil && fm.Title != "" {
		title = fm.Title
	}

	// Note-pane partial response: return only the note body, no chrome.
	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:    reqPath,
			Frontmatter: fm,
			HTML:        template.HTML(html),
			SSEWatch:    viewSSEWatch(reqPath),
			ViewHref:    "/view/" + reqPath + dq,
			DirQuery:    dq,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.templates.renderNotePartial(w, partial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full page: build the sidebar too.
	lf := s.buildLayoutFields(title, reqPath, sidebarDir)
	card, err := s.buildDirIndex(sidebarDir, reqPath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
	}

	view := ViewData{
		layoutFields: lf,
		NotePath:     reqPath,
		Frontmatter:  fm,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(reqPath),
		ViewHref:     "/view/" + reqPath + dq,
		IndexCard:    card,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeSidebarPartial renders just the sidebar fragment for a given
// directory and optional in-view note (for sticky links). Used both
// by /view/... and / when HX-Target: sidebar is set.
func (s *Server) writeSidebarPartial(w http.ResponseWriter, r *http.Request, sidebarDir, notePath string) {
	if sidebarDir == "" && notePath != "" {
		sidebarDir = noteParentDir(notePath)
	}
	if sidebarDir == "" {
		sidebarDir, _ = parseDirParam(r)
	}
	card, err := s.buildDirIndex(sidebarDir, notePath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
		card = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeNoteNotFoundPartial serves the "note not found" fragment for an
// HX-Target: note-pane request, using HTTP 200 so HTMX swaps it in
// rather than skipping the swap on a 4xx status.
func (s *Server) writeNoteNotFoundPartial(w http.ResponseWriter, reqPath string) {
	partial := NotePartialData{
		NotePath: reqPath,
		HTML:     template.HTML(`<p class="text-gray-500 text-center py-8">Note not found.</p>`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderNotePartial(w, partial); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// buildDirIndex assembles an IndexCard in directory mode for a path
// relative to the notes root. notePath is the note currently in view
// (if any) — directory links in the resulting card will target that
// note with an updated ?dir= so the note stays visible when the user
// navigates the panel. Pass "" for the empty-state page.
func (s *Server) buildDirIndex(sidebarDir, notePath string) (*IndexCard, error) {
	absPath, err := SafePath(s.root, sidebarDir)
	if err != nil {
		return nil, err
	}
	entries, err := readDirEntries(absPath, sidebarDir, notePath)
	if err != nil {
		return nil, err
	}
	return &IndexCard{
		Mode:        "dir",
		Breadcrumbs: buildBreadcrumbs(sidebarDir, notePath),
		Entries:     entries,
		Empty:       "No files here.",
	}, nil
}

// noteParentDir returns the relative directory of a note path, or "" for
// notes at the root.
func noteParentDir(notePath string) string {
	d := filepath.Dir(notePath)
	if d == "." {
		return ""
	}
	return d
}
```

- [ ] **Step 2: Verify the package builds**

```bash
go build ./internal/server/
```

Expected: builds cleanly. If not, the most likely culprits are template field references in the templates themselves (fixed in later tasks) or unused imports — but since we're keeping the editor-launch code below this replacement, most imports stay valid.

- [ ] **Step 3: Do NOT commit yet — templates still broken**

---

## Task 7: Templates — rewrite `layout.html`, `view.html`, create partials, delete `browse.html`

**Files:**
- Modify: `web/templates/layout.html`
- Modify: `web/templates/view.html`
- Create: `web/templates/sidebar_body.html`
- Create: `web/templates/note_pane_body.html`
- Delete: `web/templates/browse.html`

- [ ] **Step 1: Rewrite `web/templates/layout.html`**

```html
{{ define "layout" }}<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ if .Title }}{{ .Title }} — {{ end }}notesview</title>
  <link rel="stylesheet" href="/static/style.css">
  <script>
    // Pre-paint: apply sidebar-open class from localStorage before first
    // render so the user never sees a flash of wrong sidebar state.
    try {
      if (localStorage.getItem('notesview.sidebarOpen') === '1') {
        document.documentElement.classList.add('sidebar-open');
      }
    } catch (e) {}
  </script>
</head>
<body data-note-path="{{ .NotePath }}">
  <header id="topbar" class="fixed top-0 left-0 right-0 h-12 bg-white border-b border-gray-200 flex items-center px-3 gap-2 z-[100]">
    <button
      id="sidebar-toggle"
      type="button"
      aria-label="Toggle files index"
      aria-controls="sidebar"
      aria-expanded="false"
      class="bg-transparent border-0 cursor-pointer px-2 py-1.5 text-lg text-gray-500 rounded-md leading-none flex-shrink-0 hover:bg-gray-50 hover:text-gray-900">&#9776;</button>
    <div class="flex-1 flex items-center justify-end gap-2 overflow-hidden">
      {{ if .EditPath }}
      <button
        id="edit-btn"
        type="button"
        hx-post="{{ .EditHref }}"
        hx-swap="none"
        class="bg-transparent border border-gray-300 rounded-md cursor-pointer px-3 py-1 text-[13px] text-gray-900 font-sans flex-shrink-0 hover:bg-gray-50 hover:border-gray-200">Edit</button>
      {{ end }}
    </div>
  </header>

  <aside id="sidebar" class="fixed top-12 left-0 bottom-0 w-[320px] bg-white border-r border-gray-200 overflow-y-auto hidden">
    {{ template "sidebar_body" (dict "IndexCard" .IndexCard) }}
  </aside>

  <main id="note-pane" class="fixed top-12 left-0 right-0 bottom-0 overflow-y-auto px-6 py-8 max-md:px-4 max-md:py-5 max-[480px]:px-3 max-[480px]:py-4">
    {{ template "note_pane_body" . }}
  </main>

  <script type="module" src="/static/app.js"></script>
</body>
</html>
{{ end }}
```

Note: the `dict` function is *not* built into Go's html/template. The simplest way to pass just the `IndexCard` into `sidebar_body` is to make the partial read from the top-level context directly:

```html
    {{ template "sidebar_body" . }}
```

and have `sidebar_body.html` reference `.IndexCard`. Use that form. Delete the `(dict ...)` call. (I left it in the first draft to show intent, but the dict helper doesn't exist.)

- [ ] **Step 2: Create `web/templates/sidebar_body.html`**

```html
{{ define "sidebar_body" }}
{{ with .IndexCard }}
{{ template "index_card" . }}
{{ end }}
{{ end }}
```

- [ ] **Step 3: Create `web/templates/note_pane_body.html`**

```html
{{ define "note_pane_body" }}
<article
  id="note-card"
  class="note-card mx-auto max-w-[900px] border border-gray-200 rounded-md bg-white px-6 py-6 max-md:px-4 max-md:py-5 max-[480px]:px-3 max-[480px]:py-4"
  data-note-path="{{ .NotePath }}"
  hx-ext="sse"
  sse-connect="{{ .SSEWatch }}"
  hx-trigger="sse:change"
  hx-get="{{ .ViewHref }}"
  hx-target="#note-pane"
  hx-swap="innerHTML">
  {{ with .Frontmatter }}
  {{ if or .Title .Description .Tags }}
  <div class="pb-4 mb-6 border-b border-gray-200">
    {{ if .Title }}<h1 class="text-[28px] font-semibold leading-tight text-gray-900 mt-0 mb-2">{{ .Title }}</h1>{{ end }}
    {{ if .Description }}<p class="text-[15px] text-gray-500 mt-0 mb-3">{{ .Description }}</p>{{ end }}
    {{ if .Tags }}
    <ul class="flex flex-wrap gap-1.5 m-0 p-0 list-none">
      {{ range .Tags }}<li class="inline-flex items-center bg-blue-100 text-blue-600 text-xs font-medium px-2 py-0.5 rounded-full leading-relaxed hover:bg-blue-200">{{ . }}</li>{{ end }}
    </ul>
    {{ end }}
  </div>
  {{ end }}
  {{ end }}
  <div class="markdown-body prose max-w-none">{{ .HTML }}</div>
</article>
{{ end }}
```

Note: The SSE swap uses `innerHTML` targeting `#note-pane` because the note-pane partial response returns an `<article id="note-card">` element. Swapping `innerHTML` of `#note-pane` replaces the article in place, keeping `#note-pane`'s scroll container intact.

- [ ] **Step 4: Overwrite `web/templates/view.html` (now a stub that delegates to layout)**

```html
{{ template "layout" . }}
```

- [ ] **Step 5: Delete `web/templates/browse.html`**

```bash
git rm web/templates/browse.html
```

- [ ] **Step 6: Verify the Go package builds**

```bash
go build ./...
```

Expected: builds cleanly. The server package no longer references `renderBrowse` or `BrowseData` (both are gone from `templates.go` in Task 5) and the handler code from Task 6 is consistent with the new template set.

- [ ] **Step 7: Do NOT commit yet — server tests still reference `?index=` shapes**

---

## Task 8: Templates — add HTMX attrs to `index_card.html` and `breadcrumbs.html`

**Files:**
- Modify: `web/templates/index_card.html`
- Modify: `web/templates/breadcrumbs.html`

- [ ] **Step 1: Rewrite `web/templates/index_card.html`**

```html
{{ define "index_card" }}
<div class="w-full">
  <header class="sticky top-0 bg-gray-50 px-4 py-2 border-b border-gray-200 flex items-center">
    {{ if eq .Mode "dir" }}
    {{ template "breadcrumbs" .Breadcrumbs }}
    {{ end }}
  </header>
  {{ if .Entries }}
  <ul class="list-none m-0 p-0">
    {{ range .Entries }}
    {{ if .IsDir }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-boost="true"
        hx-target="#sidebar"
        hx-swap="innerHTML"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 font-medium no-underline transition-colors duration-100 hover:bg-gray-50">&#128193; {{ .Name }}</a>
    </li>
    {{ else }}
    <li class="border-b border-gray-100 last:border-b-0">
      <a
        href="{{ .Href }}"
        hx-boost="true"
        hx-target="#note-pane"
        hx-swap="innerHTML"
        class="flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 hover:bg-gray-50">&#128196; {{ .Name }}</a>
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

- [ ] **Step 2: Rewrite `web/templates/breadcrumbs.html`**

```html
{{ define "breadcrumbs" }}<nav class="flex-1 flex items-center gap-1 text-sm text-gray-500 overflow-hidden whitespace-nowrap">
  <a class="text-blue-600 no-underline hover:underline" href="{{ .HomeHref }}" hx-boost="true" hx-target="#sidebar" hx-swap="innerHTML" aria-label="Home">&#8962;</a>
  {{ range .Crumbs }}
  <span class="text-gray-400 select-none">/</span>
  {{ if .Current }}
  <span aria-current="page" class="text-gray-900 overflow-hidden text-ellipsis">{{ .Label }}</span>
  {{ else }}
  <a class="text-blue-600 no-underline hover:underline" href="{{ .Href }}" hx-boost="true" hx-target="#sidebar" hx-swap="innerHTML">{{ .Label }}</a>
  {{ end }}
  {{ end }}
</nav>{{ end }}
```

- [ ] **Step 3: Verify the Go package still builds**

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 4: Do NOT commit yet — the server tests are still reading old URL shapes. Move to Task 9.**

---

## Task 9: Server tests — rewrite handlers_test.go for new behavior

**Files:**
- Modify: `internal/server/handlers_test.go`

- [ ] **Step 1: Replace the URL-shape-dependent tests**

Open `internal/server/handlers_test.go`. Make these changes:

**Delete** the following tests entirely (they test behavior that no longer exists):
- `TestViewHandlerWithIndex`
- `TestViewHandlerToggleClosed`
- `TestStandaloneIndex`
- `TestStandaloneIndexSubdir`
- `TestRootRedirect` (keep the idea, but it's replaced with a new test below)

**Update** `TestViewHandler`:

Replace the body of `TestViewHandler` with:

```go
func TestViewHandler(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(body, "<h1") || !strings.Contains(body, ">Todo<") {
		t.Errorf("expected frontmatter title <h1> in body, got: %s", body)
	}
	if !strings.Contains(body, ">todo<") || !strings.Contains(body, ">daily<") {
		t.Errorf("expected frontmatter tags in body")
	}
	if !strings.Contains(body, `sse-connect="/events?watch=2026%2F03%2F20260331_9201_todo.md"`) {
		t.Errorf("expected sse-connect for file, got: %s", body)
	}
	if !strings.Contains(body, `id="sidebar"`) {
		t.Errorf("expected #sidebar element in layout, got: %s", body)
	}
	if !strings.Contains(body, `id="note-pane"`) {
		t.Errorf("expected #note-pane element in layout, got: %s", body)
	}
}
```

**Update** `TestViewHandlerLiveReloadPreservesIndex` → rename to `TestViewHandlerLiveReloadPreservesDir`:

```go
// TestViewHandlerLiveReloadPreservesDir guards against the SSE
// live-reload fetch dropping the sidebar's sticky ?dir=. The note card
// carries hx-get pointing at its own URL; that URL must include the
// current ?dir= so file saves re-render with the sticky directory
// preserved.
func TestViewHandlerLiveReloadPreservesDir(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md?dir=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `hx-get="/view/README.md?dir=2026"`) {
		t.Errorf("expected hx-get to preserve ?dir=2026, got: %s", body)
	}
}
```

**Update** `TestViewHandlerStickyPath`:

```go
// TestViewHandlerStickyPath covers the core sticky-model promise:
// passing ?dir=2026 while viewing README.md means the sidebar shows
// 2026/, and dir entries inside it link to /view/README.md?dir=2026/<sub>.
func TestViewHandlerStickyPath(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md?dir=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The sidebar shows 2026/, whose entries include 03/. A click on
	// 03/ must keep README.md in view and only move ?dir=.
	if !strings.Contains(body, `href="/view/README.md?dir=2026%2F03"`) {
		t.Errorf("expected dir entry to target current note with ?dir=2026%%2F03, got: %s", body)
	}
}
```

**Update** `TestViewHandlerPathSurvivesSelfLinks` → rename to `TestViewHandlerDirSurvivesFileClicks`:

```go
// TestViewHandlerDirSurvivesFileClicks: file entries in the sidebar
// link to those files with the current ?dir= preserved, so clicking
// them changes the note without resetting the sidebar directory.
func TestViewHandlerDirSurvivesFileClicks(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md?dir=2026%2F03", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03"`) {
		t.Errorf("expected file entry to preserve ?dir=2026%%2F03, got: %s", body)
	}
}
```

**Add** new tests for partial responses:

```go
// TestViewHandlerNotePanePartial verifies that an HX-Request with
// HX-Target: note-pane returns just the note-pane fragment, not a
// full page. The response must contain the note body and must NOT
// contain the sidebar or the topbar.
func TestViewHandlerNotePanePartial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md?dir=2026", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `id="sidebar"`) {
		t.Errorf("note-pane partial should not contain #sidebar, got: %s", body)
	}
	if strings.Contains(body, `id="topbar"`) {
		t.Errorf("note-pane partial should not contain #topbar, got: %s", body)
	}
	if !strings.Contains(body, `id="note-card"`) {
		t.Errorf("note-pane partial should contain the note card, got: %s", body)
	}
}

// TestViewHandlerSidebarPartial verifies that an HX-Request with
// HX-Target: sidebar returns just the sidebar fragment.
func TestViewHandlerSidebarPartial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md?dir=2026", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "sidebar")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `id="note-card"`) {
		t.Errorf("sidebar partial should not contain the note card, got: %s", body)
	}
	if strings.Contains(body, `id="topbar"`) {
		t.Errorf("sidebar partial should not contain the topbar, got: %s", body)
	}
	// The sidebar partial must carry breadcrumb + entry HTML.
	if !strings.Contains(body, `aria-label="Home"`) {
		t.Errorf("sidebar partial should contain breadcrumbs home link, got: %s", body)
	}
}

// TestViewHandler404Partial verifies that a missing note returned with
// HX-Target: note-pane yields HTTP 200 and an empty-state body, so
// HTMX will swap the "not found" message into the note pane.
// (Direct browser requests still get a real 404; see
// TestViewHandler404.)
func TestViewHandler404Partial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/nonexistent.md", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for partial 404, got: %d", w.Code, w.Code)
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Errorf("expected 'not found' message in body, got: %s", w.Body.String())
	}
}

// TestRootRedirectToReadme pins the / redirect behavior when README.md
// exists at the notes root. This replaces the old TestRootRedirect.
func TestRootRedirectToReadme(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/view/README.md" {
		t.Errorf("redirect location = %q, want /view/README.md", loc)
	}
}

// TestRootEmptyState pins the no-README case: / renders the two-pane
// layout with an empty note-pane and the sidebar at root.
func TestRootEmptyState(t *testing.T) {
	dir := t.TempDir()
	// No README at the root.
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes", "hello.md"), []byte("# Hi"), 0o644)
	srv, err := NewServer(dir, "", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no readme means two-pane empty state)", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="sidebar"`) {
		t.Errorf("empty state should still render #sidebar, got: %s", body)
	}
	if !strings.Contains(body, "No note selected") {
		t.Errorf("empty state should show 'No note selected', got: %s", body)
	}
}
```

- [ ] **Step 2: Run the server tests**

```bash
go test ./internal/server/ -v
```

Expected: PASS (all of the above, plus the untouched edit-handler tests, path-traversal, etc.).

- [ ] **Step 3: Run `go vet` and the full Go test suite**

```bash
go vet ./...
go test ./...
```

Expected: clean. The integration test in the repo root may still reference `?index=` shapes; that's addressed in Task 10.

- [ ] **Step 4: Commit the backend refactor as one unit**

```bash
git add internal/server/ web/templates/
git commit -m "Split view into #sidebar and #note-pane with HX-Target dispatch

Server now returns full-page, sidebar-only, or note-pane-only
responses based on the HX-Target header. Sidebar visibility moves
to client-side state; ?index=dir goes away; ?dir= is the only
sidebar-state URL param. layout.html rewires to three fixed regions
(topbar, sidebar, note-pane), each an independent scroll container.
browse.html is deleted — the standalone index view collapses into
the two-pane layout's empty state."
```

---

## Task 10: Integration test — update to new URL shape

**Files:**
- Modify: `integration_test.go`

- [ ] **Step 1: Read the current integration test**

```bash
go test -tags integration ./... -run TestIntegrationSmoke -v 2>&1 | head -50
```

Any references to `?index=dir` or `?path=` in the test will fail. Grep for them:

```bash
grep -n 'index=dir\|?path=\|renderBrowse\|BrowseData' integration_test.go
```

- [ ] **Step 2: Apply the targeted fixes**

Replace any `?index=dir&path=<x>` URL in the integration test with `?dir=<x>`. Replace any assertion that looks for `id="index-toggle"` with `id="sidebar-toggle"`, and any assertion about `class="index-card"` with `id="sidebar"` (or drop it — the new layout always renders a sidebar).

If the existing integration test doesn't reference any of these, do nothing and move on.

- [ ] **Step 3: Run the integration test**

```bash
go test -tags integration ./...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add integration_test.go
git commit -m "Update integration test to new URL shape"
```

If there were no changes needed, skip the commit.

---

## Task 11: Frontend — sidebar toggle JS

**Files:**
- Modify: `web/src/app.js`

- [ ] **Step 1: Replace `web/src/app.js`**

```js
// notesview front-end bootstrap.
//
// Loads HTMX + SSE, runs syntax highlighting on every swap, and owns
// the sidebar toggle (client-side visibility with localStorage +
// on-open sidebar refresh).

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
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
});

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
    // Refresh the sidebar for the current note: while hidden, the
    // sidebar's DOM froze at its last render, but the user may have
    // clicked wiki-links and moved to a different note.
    window.htmx && window.htmx.ajax('GET', currentSidebarUrl(), {
      target: '#sidebar',
      swap: 'innerHTML',
    });
  } else {
    // Closing strips ?dir= from the URL (intentional, per spec). No
    // pushState — this is a UI preference, not a navigation event.
    const url = new URL(window.location.href);
    url.searchParams.delete('dir');
    history.replaceState(null, '', url.toString());
  }
}

// currentSidebarUrl builds the URL for refreshing the sidebar for the
// current note. The note path is stashed on <body> by the layout
// template (data-note-path) and re-stashed on #note-card for resilience
// across note-pane swaps.
function currentSidebarUrl() {
  const notePath = (document.body.dataset.notePath || '').replace(/^\/+/, '');
  const parent = notePath ? notePath.replace(/[^/]*$/, '').replace(/\/$/, '') : '';
  const base = notePath ? `/view/${notePath}` : '/';
  return `${base}?dir=${encodeURIComponent(parent)}`;
}
```

- [ ] **Step 2: Build the frontend**

```bash
cd web && yarn build && cd ..
```

Expected: builds cleanly. The bundled output lands in `web/static/`.

- [ ] **Step 3: Do NOT commit yet — CSS still needs updating**

---

## Task 12: Frontend — CSS for independent scroll containers

**Files:**
- Modify: `web/src/style.css`

- [ ] **Step 1: Add the layout rules to `web/src/style.css`**

Append (or insert before the existing `@layer components` block) the following:

```css
/*
 * Layout: three fixed viewport regions.
 *
 * The topbar is fixed across the top (h-12). The sidebar and note-pane
 * are siblings, both fixed to the viewport and starting below the topbar,
 * each with its own overflow-y: auto so their scroll positions are
 * independent. The sidebar is hidden by default (the document root lacks
 * the .sidebar-open class); when the user toggles the hamburger, the
 * class is added, the sidebar becomes visible, and the note-pane's left
 * edge shifts right by the sidebar width.
 */
:root {
  --sidebar-width: 320px;
}

#sidebar {
  display: none;
}

html.sidebar-open #sidebar {
  display: block;
}

html.sidebar-open #note-pane {
  left: var(--sidebar-width);
}
```

Also remove the existing `.index-card` width/height rules if any conflict — the new layout controls sidebar sizing at the container level, not on the card itself. Grep for them:

```bash
grep -n 'index-card\|content-wrapper' web/src/style.css
```

If anything matches, delete those rules; the new templates don't use either class.

- [ ] **Step 2: Rebuild frontend and verify**

```bash
cd web && yarn build && cd ..
go vet ./... && go test ./...
```

Expected: clean.

- [ ] **Step 3: Smoke-test in a browser**

```bash
go run ./cmd/notesview /path/to/a/real/notes/dir
```

Open `http://localhost:<port>/`. Verify:

1. The page loads without a sidebar visible.
2. Clicking the hamburger opens the sidebar; it becomes a 320px-wide column on the left; the note-pane shifts right.
3. Scrolling inside the note-pane does not move the sidebar; scrolling inside the sidebar does not move the note-pane.
4. The topbar stays fixed during scroll of either pane.
5. Clicking a directory in the sidebar updates only the sidebar (note stays put, scroll positions preserved).
6. Clicking a file in the sidebar updates only the note-pane (sidebar stays put, scroll preserved).
7. Clicking a wiki-link inside a note updates only the note-pane.
8. Clicking an external `http://` link in a note navigates the browser normally (new history entry, full page load to the external site).
9. Closing the sidebar restores the note-pane to full width; reopening re-renders the sidebar at the current note's parent directory.
10. Refresh on an open-sidebar URL (`/view/foo?dir=a/b`) restores the same state; closing the sidebar strips the `?dir=` from the URL bar.

- [ ] **Step 4: Commit the frontend changes**

```bash
git add web/src/app.js web/src/style.css web/static/
git commit -m "Client-side sidebar toggle with independent scroll layout

Hamburger button is now a plain <button> that toggles a
.sidebar-open class on <html> (persisted in localStorage).
Opening fires an htmx.ajax to refresh the sidebar for the
current note (handling the staleness case where the user
navigated wiki-links while the sidebar was hidden). Closing
strips ?dir= from the URL. Sidebar and note-pane each have
their own overflow-y scroll container, 100vh minus the fixed
topbar."
```

---

## Task 13: Final verification

- [ ] **Step 1: Full test suite**

```bash
go vet ./...
go test ./...
go test -tags integration ./...
```

Expected: all green.

- [ ] **Step 2: Frontend build**

```bash
cd web && yarn build && cd ..
```

Expected: clean.

- [ ] **Step 3: Browser sanity run**

Start the dev server and walk through the full smoke-test list from Task 12 Step 3 one more time, including:

- Open a note with a `# Heading` duplicated in frontmatter title — the leading H1 still gets stripped (regression guard for `stripRedundantTitle`).
- Open a note containing a `[+] done` task and a `[daily]` tag — the task/daily rewriters still produce their classes (regression guard for `processTaskSyntax`).
- Trigger live-reload by editing a note in `$EDITOR` while the page is open — only the note-pane swaps, sidebar scroll position is preserved.

- [ ] **Step 4: Commit any stragglers, push branch**

```bash
git status
# If clean, no action. Otherwise inspect and commit.
git log --oneline -15
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task(s) |
| --- | --- |
| Layout: three fixed regions | Task 7 (layout.html) + Task 12 (CSS) |
| HTMX wiring: per-link boost, nothing on body | Task 7 (layout.html body), Task 8 (index_card, breadcrumbs), Task 3 (renderer) |
| External links plain | Task 1 (test), Task 3 (renderer) |
| Server response shapes (HX-Target dispatch) | Task 5 (types), Task 6 (handlers), Task 9 (tests) |
| 404 partial = HTTP 200 + body | Task 6 (writeNoteNotFoundPartial), Task 9 (TestViewHandler404Partial) |
| dirQuery on partial note responses | Task 6 (handleView partial branch), Task 9 (TestViewHandlerLiveReloadPreservesDir) |
| Sidebar toggle: client-side + opt-in refresh | Task 11 (app.js) |
| currentSidebarUrl() | Task 11 |
| Close strips ?dir= via replaceState | Task 11 |
| Browser back/forward semantics | Task 11 (pushState via hx-boost, replaceState on close), covered implicitly |
| URL model (?dir= only, no ?index=) | Task 5 (chrome.go), Task 6 (parseDirParam), Task 9 (tests) |
| Renderer: goldmark extension, AST transformer | Task 2 |
| Renderer: InlineParser for UIDs | Task 2 |
| Renderer: custom NodeRenderer | Task 3 |
| Renderer: per-request state via parser.Context | Task 4 (Renderer.Render) |
| dirQuery rename | Task 4 (renderer), Task 5 (server), throughout |
| SSE live-reload moves to #note-pane | Task 7 (note_pane_body.html) |
| Tests: TestInternalLinkBoostAttributes | Task 1 (via TestNoteProtocolLink / TestAutoLinkUID / TestRelativeMdLink) |
| Tests: TestExternalLinksStayPlain | Task 1 |
| Tests: TestASTTransformerRewritesLinks | Task 1 (integrated into the conversion tests; separate AST-level test not needed since the extension IS the transformer) |
| Tests: TestBrokenNoteLink | Task 1 |
| a11y: aria-expanded, aria-controls | Task 7 (layout.html), Task 11 (app.js updates aria-expanded) |
| Non-goals: mobile, sidebar SSE, scroll restoration | Not implemented, as intended |

No gaps. The plan covers every spec requirement.

**Placeholder scan:** No TBDs, no "implement later," no unfilled code blocks.

**Type consistency:**
- `dirQuery` function (server) returns a string starting with `?dir=` (possibly empty path). `DirQuery` field (template) holds the same shape. `dirQuery` parameter (renderer) same shape.
- `noteLinkState` struct fields: `idx`, `currentDir`, `dirQuery` — used consistently across Task 2 and Task 4.
- `NotePartialData` / `SidebarPartialData` / `ViewData` defined in Task 5, consumed in Task 6 and the templates from Task 7.
- `noteParentDir` helper added in Task 6, not referenced elsewhere (local helper).
- `parseDirParam`, `hxTargetedAt` added in Task 6, used within Task 6.
- Hamburger button id: `sidebar-toggle` in layout.html (Task 7), `getElementById('sidebar-toggle')` in app.js (Task 11). Consistent.
- Body data attribute: `data-note-path` in layout.html (Task 7), `document.body.dataset.notePath` in app.js (Task 11). Consistent.
- Sidebar partial template name: `sidebar_body` (file `web/templates/sidebar_body.html`, define block `{{ define "sidebar_body" }}`). Referenced from Task 5's `parsePartial("sidebar_body")` and Task 7's layout.
- Note-pane partial template name: `note_pane_body`. Same consistency check.

No mismatches found.
