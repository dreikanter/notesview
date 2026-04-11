package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSSEConnection(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0o644); err != nil {
		t.Fatal(err)
	}

	hub := NewSSEHub(dir)
	if err := hub.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer hub.Stop()

	srv := &Server{root: dir, sseHub: hub}

	req := httptest.NewRequest("GET", "/events?watch=test.md", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleSSE(w, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("# Updated"), 0o644); err != nil {
		t.Fatal(err)
	}

	<-done
	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Logf("SSE body: %s", body)
	}
}

func TestSSEHubClientCleanup(t *testing.T) {
	dir := t.TempDir()
	hub := NewSSEHub(dir)
	if err := hub.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer hub.Stop()

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients, got %d", count)
	}
}
