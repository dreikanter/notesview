package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("---\ntitle: Todo\ntags: [todo, daily]\n---\n# Todo\n- [+] Done\n- [ ] Pending\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Welcome\nHello"), 0o644)
	srv, err := NewServer(dir, "", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, dir
}

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
	// The frontmatter title should appear as an <h1> in a bordered bar.
	if !strings.Contains(body, "<h1") || !strings.Contains(body, ">Todo<") {
		t.Errorf("expected frontmatter title <h1> in body, got: %s", body)
	}
	// Each tag becomes a <li> inside the tag list.
	if !strings.Contains(body, ">todo<") || !strings.Contains(body, ">daily<") {
		t.Errorf("expected frontmatter tags in body")
	}
	// The SSE wrapper should reference the file path (percent-encoded).
	if !strings.Contains(body, `sse-connect="/events?watch=2026%2F03%2F20260331_9201_todo.md"`) {
		t.Errorf("expected sse-connect for file, got: %s", body)
	}
	// The #content wrapper carries the file path for client-side code
	// (SSE live reload, highlight.js scoping).
	if !strings.Contains(body, `data-file-path="2026/03/20260331_9201_todo.md"`) {
		t.Errorf("expected data-file-path on content wrapper")
	}
	// With no ?index query the index card must not be rendered, so the
	// note card is centered alone.
	if strings.Contains(body, `class="index-card`) {
		t.Errorf("expected no index card when ?index is absent, got: %s", body)
	}
	// The note body is always wrapped in a card.
	if !strings.Contains(body, `class="note-card`) {
		t.Errorf("expected note-card wrapper, got: %s", body)
	}
}

// TestViewHandlerWithIndex covers the 2-panel mode: when `?index=dir`
// is set, the index card for the note's parent directory is rendered
// alongside the note card, and every link in the card preserves the
// index query string so navigating between siblings keeps the panel
// open.
func TestViewHandlerWithIndex(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md?index=dir", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `class="index-card`) {
		t.Errorf("expected index card with ?index=dir, got: %s", body)
	}
	// Sibling links should carry the index query so toggling persists.
	if !strings.Contains(body, `href="/view/2026/03/20260331_9201_todo.md?index=dir"`) {
		t.Errorf("expected sibling file link to preserve ?index=dir")
	}
	// Breadcrumb intermediate segments should also preserve the query.
	if !strings.Contains(body, `href="/browse/2026?index=dir"`) {
		t.Errorf("expected breadcrumb link to preserve ?index=dir, got: %s", body)
	}
	// Hamburger toggles index off by linking to the path without query.
	if !strings.Contains(body, `id="index-toggle"`) ||
		!strings.Contains(body, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("expected toggle href to strip ?index when open, got: %s", body)
	}
}

// TestViewHandlerToggleClosed pins the inverse: when the index is closed
// the hamburger link must point at the same path with ?index=dir so a
// click opens the panel.
func TestViewHandlerToggleClosed(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `id="index-toggle"`) ||
		!strings.Contains(body, `href="/view/README.md?index=dir"`) {
		t.Errorf("expected toggle href to add ?index=dir when closed, got: %s", body)
	}
}

// TestViewHandlerLiveReloadPreservesIndex guards against the SSE
// live-reload fetch collapsing the index panel. The note card carries
// hx-get pointing at its own URL; if that URL omits the index query,
// every file save would re-render the page with the panel closed.
func TestViewHandlerLiveReloadPreservesIndex(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md?index=dir", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `hx-get="/view/README.md?index=dir"`) {
		t.Errorf("expected hx-get to preserve ?index=dir for live reload, got: %s", body)
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
	body := w.Body.String()
	if !strings.Contains(body, `data-dir-path=""`) {
		t.Errorf("expected #content data-dir-path in body")
	}
	if !strings.Contains(body, `href="/browse/2026"`) {
		t.Errorf("expected browse link for 2026/, got: %s", body)
	}
	if !strings.Contains(body, `href="/view/README.md"`) {
		t.Errorf("expected view link for README.md")
	}
	// Browse page IS the index card so it's always rendered.
	if !strings.Contains(body, `class="index-card`) {
		t.Errorf("expected index card on browse page")
	}
	// The hamburger has no meaning on browse pages (nothing to reveal)
	// and must not be rendered.
	if strings.Contains(body, `id="index-toggle"`) {
		t.Errorf("expected no index toggle on browse page")
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
		t.Errorf("raw = %q", w.Body.String())
	}
}

func TestPathTraversal(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRootRedirect(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/view/README.md" {
		t.Errorf("redirect location = %q, want /view/README.md", loc)
	}
}

func TestEditHandlerNoEditor(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("POST", "/api/edit/README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no editor)", w.Code)
	}
}

// TestEditHandlerWhitespaceEditor guards against a nil-slice panic when
// the editor env var is non-empty but contains only whitespace: the
// `s.editor == ""` guard passes, strings.Fields returns an empty slice,
// and a naive fields[0] indexing crashes the handler. The handler must
// treat whitespace-only config as "not configured" and return 400.
func TestEditHandlerWhitespaceEditor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	srv, err := NewServer(dir, "   \t  ", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.Routes()

	req := httptest.NewRequest("POST", "/api/edit/note.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", w.Code, w.Body.String())
	}
}

func TestEditHandlerBadPath(t *testing.T) {
	dir := t.TempDir()
	srv, err := NewServer(dir, "true", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.Routes()

	req := httptest.NewRequest("POST", "/api/edit/../etc/passwd", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestEditHandlerSimpleEditor pins the success path for a plain editor
// binary. Uses `true` so the test does not depend on any real editor.
func TestEditHandlerSimpleEditor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	srv, err := NewServer(dir, "true", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.Routes()

	req := httptest.NewRequest("POST", "/api/edit/note.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q, want status:ok", w.Body.String())
	}
}

// TestEditHandlerEditorWithArgs is the regression guard for #7: an $EDITOR
// value with embedded arguments (e.g. `subl -w`, `code --wait`,
// `nvim -R`) must be parsed into binary + args rather than treated as a
// single binary name, otherwise exec() 500s with "file not found".
func TestEditHandlerEditorWithArgs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	// `true` ignores all of these extra flags, so they're harmless, but a
	// naive exec.Command would look for a literal binary named
	// `"true --wait"` and fail.
	srv, err := NewServer(dir, "true --wait -n", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	handler := srv.Routes()

	req := httptest.NewRequest("POST", "/api/edit/note.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":                `'simple'`,
		"with space":            `'with space'`,
		"with'quote":            `'with'\''quote'`,
		"/path/to/note's me.md": `'/path/to/note'\''s me.md'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestViewStripsRedundantH1(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	// The markdown has `# Todo` which matches frontmatter title "Todo".
	// The rendered markdown body should not contain a duplicate <h1>Todo</h1>;
	// unrelated later <h1> tags are fine.
	idx := strings.Index(body, `class="markdown-body`)
	if idx == -1 {
		t.Fatalf("expected markdown-body wrapper in body, got: %s", body)
	}
	md := body[idx:]
	if strings.Contains(md, "<h1>Todo</h1>") {
		t.Errorf("expected duplicate <h1>Todo</h1> to be stripped, got: %s", md)
	}
}
