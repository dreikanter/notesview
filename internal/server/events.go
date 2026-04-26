package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dreikanter/notesctl/note"

	"github.com/dreikanter/nview/internal/index"
	"github.com/dreikanter/nview/internal/logging"
)

// scope describes which events a Subscription wants to receive.
type scope int

const (
	scopeNote scope = iota + 1 // single-note view: only events with matching ID
	scopeList                  // list view: debounced "index changed" tick
)

// listDebounce is the maximum rate at which list-scope clients receive
// "index changed" ticks. Coalesces bursts during multi-file operations
// (e.g. a sync) so the browser doesn't redraw the sidebar per file.
const listDebounce = 1 * time.Second

type eventMsg struct {
	scope scope
	id    int // populated for scopeNote
}

// watchableStore is the subset of *note.OSStore the EventHub needs:
// the standard Store interface plus Watch.
type watchableStore interface {
	note.Store
	Watch(ctx context.Context, opts ...note.WatchOpt) (note.Watcher, error)
}

type EventHub struct {
	logger *slog.Logger
	index  *index.NoteIndex
	store  watchableStore

	ctx     context.Context
	cancel  context.CancelFunc
	watcher note.Watcher

	mu      sync.RWMutex
	clients map[*Subscription]struct{}

	timerMu   sync.Mutex
	listTimer *time.Timer

	stopOnce sync.Once
	done     chan struct{}
}

type Subscription struct {
	scope  scope
	id     int
	events chan eventMsg
}

// NewEventHub builds a hub backed by store. The hub forwards watcher events
// to subscribed SSE clients filtered by scope, and applies each event to
// the index before forwarding so re-fetches always see fresh state.
func NewEventHub(store watchableStore, logger *slog.Logger, idx *index.NoteIndex) *EventHub {
	if logger == nil {
		logger = logging.Discard()
	}
	return &EventHub{
		logger:  logger,
		index:   idx,
		store:   store,
		clients: make(map[*Subscription]struct{}),
		done:    make(chan struct{}),
	}
}

// Start opens the store's watcher and begins consuming events.
func (h *EventHub) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	h.ctx = ctx
	h.cancel = cancel
	w, err := h.store.Watch(ctx)
	if err != nil {
		cancel()
		return err
	}
	h.watcher = w
	go h.eventLoop()
	return nil
}

func (h *EventHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.done)
		if h.cancel != nil {
			h.cancel()
		}
		if h.watcher != nil {
			if err := h.watcher.Close(); err != nil {
				h.logger.Warn("watcher close failed", "err", err)
			}
		}
		h.timerMu.Lock()
		if h.listTimer != nil {
			h.listTimer.Stop()
			h.listTimer = nil
		}
		h.timerMu.Unlock()
	})
}

func (h *EventHub) eventLoop() {
	for {
		select {
		case <-h.done:
			return
		case ev, ok := <-h.watcher.Events():
			if !ok {
				return
			}
			// Apply to the index BEFORE forwarding to clients so an SSE
			// client refetching against the index never sees stale state.
			if h.index != nil {
				h.index.Apply(ev)
			}
			h.dispatch(ev)
		}
	}
}

// dispatch sends ev to clients whose scope matches: note-scope subscribers
// receive an event when ev.ID matches; list-scope subscribers receive a
// debounced tick.
func (h *EventHub) dispatch(ev note.Event) {
	h.mu.RLock()
	var noteSubs []*Subscription
	hasListSub := false
	for sub := range h.clients {
		switch sub.scope {
		case scopeNote:
			if sub.id == ev.ID {
				noteSubs = append(noteSubs, sub)
			}
		case scopeList:
			hasListSub = true
		}
	}
	h.mu.RUnlock()

	for _, sub := range noteSubs {
		select {
		case sub.events <- eventMsg{scope: scopeNote, id: ev.ID}:
		default:
		}
	}
	if hasListSub {
		h.scheduleListTick()
	}
}

// scheduleListTick coalesces list-scope broadcasts to one per listDebounce
// window. The timer fires at most once per debounce window regardless of
// how many events arrive in the meantime.
func (h *EventHub) scheduleListTick() {
	h.timerMu.Lock()
	defer h.timerMu.Unlock()
	if h.listTimer != nil {
		return
	}
	h.listTimer = time.AfterFunc(listDebounce, func() {
		h.timerMu.Lock()
		h.listTimer = nil
		h.timerMu.Unlock()
		h.broadcastList()
	})
}

func (h *EventHub) broadcastList() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.clients {
		if sub.scope != scopeList {
			continue
		}
		select {
		case sub.events <- eventMsg{scope: scopeList}:
		default:
		}
	}
}

func (h *EventHub) addClient(c *Subscription) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *EventHub) removeClient(c *Subscription) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// handleEvents serves /events. Required query params:
//
//	scope=note&id=<int>  → single-note view, filtered to that ID
//	scope=list           → list view, debounced index-changed tick
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sub, err := parseSubscription(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.events.addClient(sub)
	defer s.events.removeClient(sub)

	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", toJSON(map[string]string{"type": "connected"}))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sub.events:
			switch msg.scope {
			case scopeNote:
				fmt.Fprintf(w, "event: note\ndata: %s\n\n", toJSON(map[string]int{"id": msg.id}))
			case scopeList:
				fmt.Fprintf(w, "event: list\ndata: %s\n\n", toJSON(map[string]string{"type": "list"}))
			}
			flusher.Flush()
		}
	}
}

func parseSubscription(r *http.Request) (*Subscription, error) {
	q := r.URL.Query()
	switch q.Get("scope") {
	case "note":
		idStr := q.Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid id %q for scope=note", idStr)
		}
		return &Subscription{scope: scopeNote, id: id, events: make(chan eventMsg, 8)}, nil
	case "list":
		return &Subscription{scope: scopeList, events: make(chan eventMsg, 8)}, nil
	default:
		return nil, fmt.Errorf("missing or invalid scope (expected 'note' or 'list')")
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
