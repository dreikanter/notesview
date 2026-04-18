package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTreeListRoot(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var nodes []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("invalid JSON: %v, body: %s", err, w.Body.String())
	}
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(nodes))
	}
	if !nodes[0].IsDir {
		t.Errorf("first entry should be a directory, got %+v", nodes[0])
	}
	if nodes[len(nodes)-1].IsDir {
		t.Errorf("last entry should be a file, got %+v", nodes[len(nodes)-1])
	}
	if nodes[0].Path != nodes[0].Name {
		t.Errorf("root-level path should equal name, got path=%q name=%q", nodes[0].Path, nodes[0].Name)
	}
}

func TestTreeListSubdir(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var nodes []struct {
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) == 0 {
		t.Fatal("expected entries under 2026")
	}
	for _, n := range nodes {
		if !strings.HasPrefix(n.Path, "2026/") {
			t.Errorf("child path should be prefixed with parent: %q", n.Path)
		}
	}
}

func TestTreeListPathEscaped(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=2026%2F03", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var nodes []struct{ Path string `json:"path"` }
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) == 0 {
		t.Fatal("expected entries under 2026/03")
	}
	for _, n := range nodes {
		if !strings.HasPrefix(n.Path, "2026/03/") {
			t.Errorf("child path should be 2026/03/..., got %q", n.Path)
		}
	}
}

func TestTreeListNonexistent(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=nope", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTreeListPathTraversal(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=../secrets", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTreeListNotADirectory(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	// README.md is a file, not a directory.
	req := httptest.NewRequest("GET", "/api/tree/list?path=README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTreeListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
	os.WriteFile(filepath.Join(dir, "empty", ".gitkeep"), nil, 0o644)
	srv, _ := NewServer(dir, "", nil)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=empty", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("empty dir should return [], got %q", w.Body.String())
	}
}
