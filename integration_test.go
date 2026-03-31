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

	"github.com/alex/notesview/internal/server"
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

	// Test: view a file (JSON API)
	req, _ := http.NewRequest("GET", ts.URL+"/view/2026/03/20260331_9201_todo.md", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("view: status = %d, body: %s", resp.StatusCode, body)
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
