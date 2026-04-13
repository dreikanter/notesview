//go:build integration

package main_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreikanter/notes-view/internal/server"
)

func TestIntegrationSmoke(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Welcome\n\nHello world.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"),
		[]byte("---\ntitle: Daily Todo\ntags: [todo]\n---\n# Daily Todo\n\n- [+] Done task\n- [ ] Pending task\n- [daily] Routine\n\nSee [readme](../../README.md) and note://20260331_9201.\n"), 0o644)

	srv, err := server.NewServer(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Test: root redirects to README
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	rootResp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	rootResp.Body.Close()
	if rootResp.StatusCode != http.StatusFound {
		t.Errorf("root: status = %d, want 302", rootResp.StatusCode)
	}

	// Test: view a file renders HTML
	viewResp, err := http.Get(ts.URL + "/view/2026/03/20260331_9201_todo.md")
	if err != nil {
		t.Fatal(err)
	}
	defer viewResp.Body.Close()
	if viewResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(viewResp.Body)
		t.Fatalf("view: status = %d, body: %s", viewResp.StatusCode, body)
	}
	if ct := viewResp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("view: content-type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(viewResp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Daily Todo") {
		t.Errorf("view: expected title 'Daily Todo' in body")
	}
	if !strings.Contains(bodyStr, `class="markdown-body`) {
		t.Errorf("view: expected markdown-body wrapper in HTML")
	}

	// Test: /view/README.md renders a two-pane view with the sidebar
	// showing the note's parent directory (root). Dir entries link to
	// /dir/{path}, file entries link to /view/{path}.
	dirResp, err := http.Get(ts.URL + "/view/README.md")
	if err != nil {
		t.Fatal(err)
	}
	viewBody, err := io.ReadAll(dirResp.Body)
	dirResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dirResp.StatusCode != http.StatusOK {
		t.Errorf("/view/README.md: status = %d, body: %s", dirResp.StatusCode, viewBody)
	}
	// Sidebar region must be present.
	if !strings.Contains(string(viewBody), `id="sidebar"`) {
		t.Errorf("expected #sidebar in response")
	}
	// The root dir contains 2026/. Its entry must link to /dir/2026.
	if !strings.Contains(string(viewBody), `href="/dir/2026"`) {
		t.Errorf("expected 2026/ dir entry linking to /dir/2026, got: %s", viewBody)
	}

	// Test: raw endpoint
	rawResp, err := http.Get(ts.URL + "/api/raw/README.md")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rawResp.Body)
	rawResp.Body.Close()
	if string(raw) != "# Welcome\n\nHello world.\n" {
		t.Errorf("raw = %q", raw)
	}

	// Test: 404
	notFoundResp, err := http.Get(ts.URL + "/view/nonexistent.md")
	if err != nil {
		t.Fatal(err)
	}
	notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Errorf("404: status = %d", notFoundResp.StatusCode)
	}
}
