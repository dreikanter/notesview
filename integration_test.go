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

	"github.com/dreikanter/notesview/internal/server"
)

func TestIntegrationSmoke(t *testing.T) {
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

	// Test: view a file renders HTML
	resp, err = http.Get(ts.URL + "/view/2026/03/20260331_9201_todo.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("view: status = %d, body: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("view: content-type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Daily Todo") {
		t.Errorf("view: expected title 'Daily Todo' in body")
	}
	if !strings.Contains(bodyStr, `class="markdown-body`) {
		t.Errorf("view: expected markdown-body wrapper in HTML")
	}

	// Test: browse root
	resp, err = http.Get(ts.URL + "/browse/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("browse: status = %d", resp.StatusCode)
	}
	browseBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(browseBody), `class="dir-listing"`) {
		t.Errorf("browse: expected dir-listing in body")
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
	resp, _ = http.Get(ts.URL + "/view/nonexistent.md")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("404: status = %d", resp.StatusCode)
	}
}
