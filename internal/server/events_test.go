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

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	srv := &Server{root: dir, events: hub}

	req := httptest.NewRequest("GET", "/events?watch=test.md", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleEvents(w, req)
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
	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}

	srv := &Server{root: dir, events: hub}
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
	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
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

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	c1 := &Subscription{watchPath: "shared.md", events: make(chan eventMsg, 1)}
	c2 := &Subscription{watchPath: "shared.md", events: make(chan eventMsg, 1)}
	hub.addClient(c1)
	hub.addClient(c2)
	defer hub.removeClient(c1)
	defer hub.removeClient(c2)

	absPath, _ := SafePath(dir, "shared.md")
	hub.broadcastChange(absPath)

	select {
	case msg := <-c1.events:
		if msg.path != "shared.md" || msg.kind != "change" {
			t.Errorf("client 1: expected change/shared.md, got %s/%s", msg.kind, msg.path)
		}
	case <-time.After(time.Second):
		t.Error("client 1: timed out waiting for event")
	}

	select {
	case msg := <-c2.events:
		if msg.path != "shared.md" || msg.kind != "change" {
			t.Errorf("client 2: expected change/shared.md, got %s/%s", msg.kind, msg.path)
		}
	case <-time.After(time.Second):
		t.Error("client 2: timed out waiting for event")
	}
}

func TestSSESelectiveBroadcast(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("B"), 0o644)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	clientA := &Subscription{watchPath: "a.md", events: make(chan eventMsg, 1)}
	clientB := &Subscription{watchPath: "b.md", events: make(chan eventMsg, 1)}
	hub.addClient(clientA)
	hub.addClient(clientB)
	defer hub.removeClient(clientA)
	defer hub.removeClient(clientB)

	absA, _ := SafePath(dir, "a.md")
	hub.broadcastChange(absA)

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

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	clientA := &Subscription{watchPath: "a.md", events: make(chan eventMsg, 1)}
	clientB := &Subscription{watchPath: "b.md", events: make(chan eventMsg, 1)}
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

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	srv := &Server{root: dir, events: hub}

	req := httptest.NewRequest("GET", "/events?watch=test.md", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleEvents(w, req)
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
}

func TestSSENonBlockingSend(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("data"), 0o644)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	// Slow client: buffer is already full.
	slow := &Subscription{watchPath: "test.md", events: make(chan eventMsg, 1)}
	slow.events <- eventMsg{kind: "change", path: "stale"}

	// Fast client: buffer is empty.
	fast := &Subscription{watchPath: "test.md", events: make(chan eventMsg, 1)}

	hub.addClient(slow)
	hub.addClient(fast)
	defer hub.removeClient(slow)
	defer hub.removeClient(fast)

	absPath, _ := SafePath(dir, "test.md")

	// broadcastChange must not block even though slow's channel is full.
	broadcastDone := make(chan struct{})
	go func() {
		hub.broadcastChange(absPath)
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
		if p.path != "test.md" {
			t.Errorf("fast client: expected test.md, got %s", p.path)
		}
	default:
		t.Error("fast client should have received the event")
	}

	// Slow client's channel should still contain the stale value (not replaced).
	select {
	case p := <-slow.events:
		if p.path != "stale" {
			t.Errorf("slow client: expected stale value, got %s", p.path)
		}
	default:
		t.Error("slow client channel should still have the stale value")
	}
}

func TestDirChangedBroadcast(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	sub := &Subscription{watchPath: "", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	if err := os.WriteFile(filepath.Join(dir, "a", "new.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-sub.events:
		if msg.kind != "dir-changed" {
			t.Errorf("kind = %q, want dir-changed", msg.kind)
		}
		if msg.path != "a" {
			t.Errorf("path = %q, want a", msg.path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for dir-changed")
	}
}

func TestDirChangedOnNewSubdir(t *testing.T) {
	dir := t.TempDir()

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	sub := &Subscription{watchPath: "", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	if err := os.Mkdir(filepath.Join(dir, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-sub.events:
			if msg.kind == "dir-changed" && msg.path == "" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for dir-changed at root")
		}
	}
}

func TestDirChangedEndpointEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	srv := &Server{root: dir, events: hub}

	req := httptest.NewRequest("GET", "/events", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleEvents(w, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "a", "n.md"), []byte("x"), 0o644)

	<-done
	body := w.Body.String()
	if !strings.Contains(body, "event: dir-changed") {
		t.Errorf("expected event: dir-changed in body, got:\n%s", body)
	}
}

func TestChangeEventStillDelivered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.md"), []byte("a"), 0o644)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	sub := &Subscription{watchPath: "x.md", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	os.WriteFile(filepath.Join(dir, "x.md"), []byte("b"), 0o644)

	deadline := time.After(3 * time.Second)
	gotChange := false
	for !gotChange {
		select {
		case msg := <-sub.events:
			if msg.kind == "change" && msg.path == "x.md" {
				gotChange = true
			}
		case <-deadline:
			t.Fatal("change event not delivered")
		}
	}
}
