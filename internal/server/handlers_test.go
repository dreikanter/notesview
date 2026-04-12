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

// TestViewHandlerNotePanePartial verifies that an HX-Request with
// HX-Target: note-pane returns just the note-pane fragment, not a
// full page. The response must contain the note body and must NOT
// contain the sidebar or the topbar.
func TestViewHandlerNotePanePartial(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/2026/03/20260331_9201_todo.md", nil)
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

// TestViewHandler404Partial verifies that a missing note returned with
// HX-Target: note-pane yields HTTP 200 and an empty-state body.
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
// exists at the notes root.
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

func TestTagNotesHandlerUnknownTag(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	// The note-pane response for an unknown tag shows an empty state.
	req := httptest.NewRequest("GET", "/tags/nonexistent", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "note-pane")
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

