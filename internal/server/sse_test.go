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
	os.WriteFile(testFile, []byte("# Test"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
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
	os.WriteFile(testFile, []byte("# Updated"), 0o644)

	<-done
	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Logf("SSE body: %s", body)
	}
}

func TestServerShutdownStopsHub(t *testing.T) {
	dir := t.TempDir()
	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()

	srv := &Server{root: dir, sseHub: hub}
	srv.Shutdown()

	// After Shutdown, the done channel should be closed.
	select {
	case <-hub.done:
		// expected
	default:
		t.Fatal("expected hub.done to be closed after Shutdown")
	}
}

func TestSSEHubClientCleanup(t *testing.T) {
	dir := t.TempDir()
	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients, got %d", count)
	}
}

func TestSSEMultiClientBroadcast(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "shared.md"), []byte("# Shared"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	c1 := &sseClient{watchPath: "shared.md", events: make(chan string, 1)}
	c2 := &sseClient{watchPath: "shared.md", events: make(chan string, 1)}
	hub.addClient(c1)
	hub.addClient(c2)
	defer hub.removeClient(c1)
	defer hub.removeClient(c2)

	absPath, _ := SafePath(dir, "shared.md")
	hub.broadcast(absPath)

	select {
	case p := <-c1.events:
		if p != "shared.md" {
			t.Errorf("client 1: expected shared.md, got %s", p)
		}
	case <-time.After(time.Second):
		t.Error("client 1: timed out waiting for event")
	}

	select {
	case p := <-c2.events:
		if p != "shared.md" {
			t.Errorf("client 2: expected shared.md, got %s", p)
		}
	case <-time.After(time.Second):
		t.Error("client 2: timed out waiting for event")
	}
}

func TestSSESelectiveBroadcast(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	clientA := &sseClient{watchPath: "a.md", events: make(chan string, 1)}
	clientB := &sseClient{watchPath: "b.md", events: make(chan string, 1)}
	hub.addClient(clientA)
	hub.addClient(clientB)
	defer hub.removeClient(clientA)
	defer hub.removeClient(clientB)

	absA, _ := SafePath(dir, "a.md")
	hub.broadcast(absA)

	select {
	case <-clientA.events:
		// expected
	case <-time.After(time.Second):
		t.Error("client A should have received the event")
	}

	select {
	case <-clientB.events:
		t.Error("client B should NOT have received the event for file A")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSSEPerPathDebounce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	clientA := &sseClient{watchPath: "a.md", events: make(chan string, 1)}
	clientB := &sseClient{watchPath: "b.md", events: make(chan string, 1)}
	hub.addClient(clientA)
	hub.addClient(clientB)
	defer hub.removeClient(clientA)
	defer hub.removeClient(clientB)

	// Write to both files within the debounce window (100ms).
	// Each file's debounce timer is independent, so both clients
	// should receive their respective events.
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A updated"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B updated"), 0o644)

	timeout := time.After(3 * time.Second)

	select {
	case <-clientA.events:
		// expected
	case <-timeout:
		t.Error("client A: timed out waiting for debounced event")
	}

	select {
	case <-clientB.events:
		// expected
	case <-timeout:
		t.Error("client B: timed out waiting for debounced event")
	}
}

func TestSSEClientCleanupOnDisconnect(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("# Test"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	srv := &Server{root: dir, sseHub: hub}

	req := httptest.NewRequest("GET", "/events?watch=test.md", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleSSE(w, req)
		close(done)
	}()

	// Wait for the client to register.
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	before := len(hub.clients)
	hub.mu.RUnlock()
	if before != 1 {
		t.Fatalf("expected 1 client before disconnect, got %d", before)
	}

	// Simulate client disconnect.
	cancel()
	<-done

	hub.mu.RLock()
	after := len(hub.clients)
	hub.mu.RUnlock()
	if after != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", after)
	}

	// The watcher should have removed the directory.
	watched := hub.watcher.WatchList()
	parentDir := filepath.Dir(filepath.Join(dir, "test.md"))
	for _, w := range watched {
		if w == parentDir {
			t.Errorf("watcher still watching %s after last client disconnected", parentDir)
		}
	}
}

func TestSSENonBlockingSend(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("data"), 0o644)

	hub := NewSSEHub(dir, nil, nil, nil)
	hub.Start()
	defer hub.Stop()

	// Slow client: buffer is already full.
	slow := &sseClient{watchPath: "test.md", events: make(chan string, 1)}
	slow.events <- "stale"

	// Fast client: buffer is empty.
	fast := &sseClient{watchPath: "test.md", events: make(chan string, 1)}

	hub.addClient(slow)
	hub.addClient(fast)
	defer hub.removeClient(slow)
	defer hub.removeClient(fast)

	absPath, _ := SafePath(dir, "test.md")

	// broadcast must not block even though slow's channel is full.
	broadcastDone := make(chan struct{})
	go func() {
		hub.broadcast(absPath)
		close(broadcastDone)
	}()

	select {
	case <-broadcastDone:
		// expected: broadcast returned without blocking
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on slow client")
	}

	// Fast client should have received the event.
	select {
	case p := <-fast.events:
		if p != "test.md" {
			t.Errorf("fast client: expected test.md, got %s", p)
		}
	default:
		t.Error("fast client should have received the event")
	}

	// Slow client's channel should still contain the stale value (not replaced).
	select {
	case p := <-slow.events:
		if p != "stale" {
			t.Errorf("slow client: expected stale value, got %s", p)
		}
	default:
		t.Error("slow client channel should still have the stale value")
	}
}
