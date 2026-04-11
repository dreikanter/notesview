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
	srv, err := NewServer(dir, "")
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
	// Sidebar tree should include the file.
	if !strings.Contains(body, `data-file-path="2026/03/20260331_9201_todo.md"`) {
		t.Errorf("expected sidebar link for file")
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
