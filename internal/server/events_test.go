package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dreikanter/notesctl/note"

	"github.com/dreikanter/nview/internal/index"
)

// memWatchStore wraps note.MemStore with a stubbed Watch method backed by
// a writable channel the test controls. Reconcile is delegated to MemStore.
type memWatchStore struct {
	*note.MemStore
	ch chan note.Event
}

func newMemWatchStore() *memWatchStore {
	return &memWatchStore{MemStore: note.NewMemStore(), ch: make(chan note.Event, 32)}
}

type stubWatcher struct {
	ch chan note.Event
}

func (s *stubWatcher) Events() <-chan note.Event { return s.ch }
func (s *stubWatcher) Close() error              { return nil }

func (s *memWatchStore) Watch(ctx context.Context, opts ...note.WatchOpt) (note.Watcher, error) {
	return &stubWatcher{ch: s.ch}, nil
}

// emit pushes a watcher event from a test, blocking briefly if the buffer is full.
func (s *memWatchStore) emit(t *testing.T, ev note.Event) {
	t.Helper()
	select {
	case s.ch <- ev:
	case <-time.After(time.Second):
		t.Fatal("emit blocked: watcher channel full")
	}
}

func newTestHub(t *testing.T) (*memWatchStore, *index.NoteIndex, *EventHub) {
	t.Helper()
	store := newMemWatchStore()
	idx := index.New(store, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	hub := NewEventHub(store, nil, idx)
	if err := hub.Start(); err != nil {
		t.Fatalf("hub.Start: %v", err)
	}
	t.Cleanup(hub.Stop)
	return store, idx, hub
}

func TestNoteScopeReceivesMatchingEvent(t *testing.T) {
	store, _, hub := newTestHub(t)

	stored, err := store.Put(note.Entry{ID: 7, Meta: note.Meta{
		CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Slug: "fresh", Title: "Fresh",
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	sub := &Subscription{scope: scopeNote, id: stored.ID, events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	store.emit(t, note.Event{Type: note.EventCreated, ID: stored.ID})

	select {
	case msg := <-sub.events:
		if msg.scope != scopeNote || msg.id != stored.ID {
			t.Errorf("got %+v, want scope=note id=%d", msg, stored.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("note-scope client did not receive event")
	}
}

func TestNoteScopeIgnoresOtherIDs(t *testing.T) {
	store, _, hub := newTestHub(t)
	if _, err := store.Put(note.Entry{ID: 1, Meta: note.Meta{CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Slug: "a"}}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	sub := &Subscription{scope: scopeNote, id: 1, events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	store.emit(t, note.Event{Type: note.EventCreated, ID: 999})

	select {
	case msg := <-sub.events:
		t.Errorf("scope=note id=1 should not receive event for ID 999, got %+v", msg)
	case <-time.After(150 * time.Millisecond):
		// expected
	}
}

func TestListScopeReceivesDebouncedTick(t *testing.T) {
	store, _, hub := newTestHub(t)
	for _, id := range []int{1, 2, 3} {
		if _, err := store.Put(note.Entry{ID: id, Meta: note.Meta{CreatedAt: time.Date(2026, 4, id, 0, 0, 0, 0, time.UTC)}}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	sub := &Subscription{scope: scopeList, events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	// Emit a burst of events; the list-scope subscriber should receive
	// exactly one debounced tick.
	store.emit(t, note.Event{Type: note.EventCreated, ID: 1})
	store.emit(t, note.Event{Type: note.EventCreated, ID: 2})
	store.emit(t, note.Event{Type: note.EventCreated, ID: 3})

	deadline := time.After(listDebounce + time.Second)
	select {
	case msg := <-sub.events:
		if msg.scope != scopeList {
			t.Errorf("got %+v, want scope=list", msg)
		}
	case <-deadline:
		t.Fatal("list-scope client did not receive debounced tick")
	}

	// No second tick should arrive within another short window.
	select {
	case msg := <-sub.events:
		t.Errorf("unexpected second tick: %+v", msg)
	case <-time.After(150 * time.Millisecond):
		// expected: debounce coalesces the burst
	}
}

func TestApplyHappensBeforeBroadcast(t *testing.T) {
	// The eventLoop must apply the index update before forwarding the
	// SSE event, so a client refetching against the index always sees
	// fresh state.
	store, idx, hub := newTestHub(t)
	stored, err := store.Put(note.Entry{ID: 42, Meta: note.Meta{
		CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Slug: "x", Title: "X",
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	sub := &Subscription{scope: scopeNote, id: stored.ID, events: make(chan eventMsg, 1)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	store.emit(t, note.Event{Type: note.EventCreated, ID: stored.ID})

	select {
	case <-sub.events:
		// At this point the index must already reflect the new entry.
		if _, ok := idx.NoteByID(stored.ID); !ok {
			t.Error("index does not contain entry when broadcast is delivered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no broadcast received")
	}
}

func TestServerShutdownStopsHub(t *testing.T) {
	_, _, hub := newTestHub(t)

	srv := &Server{events: hub}
	srv.Shutdown()

	select {
	case <-hub.done:
		// expected
	default:
		t.Fatal("expected hub.done to be closed after Shutdown")
	}
}

func TestSSEClientCleanupOnDisconnect(t *testing.T) {
	_, _, hub := newTestHub(t)

	srv := &Server{events: hub}

	req := httptest.NewRequest("GET", "/events?scope=note&id=1", nil)
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

	cancel()
	<-done

	hub.mu.RLock()
	after := len(hub.clients)
	hub.mu.RUnlock()
	if after != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", after)
	}
}

func TestEventsRequiresScope(t *testing.T) {
	_, _, hub := newTestHub(t)
	srv := &Server{events: hub}

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()
	srv.handleEvents(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEventsRejectsInvalidNoteID(t *testing.T) {
	_, _, hub := newTestHub(t)
	srv := &Server{events: hub}

	req := httptest.NewRequest("GET", "/events?scope=note&id=0", nil)
	w := httptest.NewRecorder()
	srv.handleEvents(w, req)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestEventsEmitsEventOverHTTP(t *testing.T) {
	store, _, hub := newTestHub(t)
	stored, err := store.Put(note.Entry{ID: 5, Meta: note.Meta{
		CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Slug: "x",
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	srv := &Server{events: hub}

	req := httptest.NewRequest("GET", "/events?scope=note&id=5", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleEvents(w, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	store.emit(t, note.Event{Type: note.EventUpdated, ID: stored.ID})

	<-done
	body := w.Body.String()
	if !strings.Contains(body, "event: note") {
		t.Errorf("expected event: note in body, got:\n%s", body)
	}
}

func TestNonBlockingSend(t *testing.T) {
	store, _, hub := newTestHub(t)
	if _, err := store.Put(note.Entry{ID: 1, Meta: note.Meta{CreatedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Slow client: buffer is already full.
	slow := &Subscription{scope: scopeNote, id: 1, events: make(chan eventMsg, 1)}
	slow.events <- eventMsg{scope: scopeNote, id: 0}

	fast := &Subscription{scope: scopeNote, id: 1, events: make(chan eventMsg, 1)}

	hub.addClient(slow)
	hub.addClient(fast)
	defer hub.removeClient(slow)
	defer hub.removeClient(fast)

	store.emit(t, note.Event{Type: note.EventUpdated, ID: 1})

	// Fast client should see the broadcast.
	select {
	case msg := <-fast.events:
		if msg.id != 1 {
			t.Errorf("fast client got id %d, want 1", msg.id)
		}
	case <-time.After(time.Second):
		t.Error("fast client did not receive event (broadcast may have blocked on slow client)")
	}

	// Slow client's stale value must remain intact (broadcast skipped, not replaced).
	select {
	case msg := <-slow.events:
		if msg.id != 0 {
			t.Errorf("slow client lost stale value: got %+v", msg)
		}
	default:
		t.Error("slow client buffer is empty; broadcast must have replaced the value")
	}
}
