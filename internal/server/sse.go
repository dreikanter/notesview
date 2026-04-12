package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type SSEHub struct {
	root    string
	logger  *slog.Logger
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	watcher *fsnotify.Watcher
	done    chan struct{}
}

type sseClient struct {
	watchPath string
	events    chan string
}

func NewSSEHub(root string, logger *slog.Logger) *SSEHub {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SSEHub{
		root:    root,
		logger:  logger,
		clients: make(map[*sseClient]struct{}),
		done:    make(chan struct{}),
	}
}

func (h *SSEHub) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	h.watcher = watcher
	go h.eventLoop()
	return nil
}

func (h *SSEHub) Stop() {
	close(h.done)
	if h.watcher != nil {
		h.watcher.Close()
	}
}

func (h *SSEHub) eventLoop() {
	timers := make(map[string]*time.Timer)

	for {
		select {
		case <-h.done:
			for _, t := range timers {
				t.Stop()
			}
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			p := event.Name
			if t, ok := timers[p]; ok {
				t.Stop()
			}
			timers[p] = time.AfterFunc(100*time.Millisecond, func() {
				h.broadcast(p)
			})
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			h.logger.Warn("file watcher error", "err", err)
		}
	}
}

func (h *SSEHub) broadcast(absPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		safePath, err := SafePath(h.root, client.watchPath)
		if err != nil {
			continue
		}
		if safePath == absPath {
			select {
			case client.events <- client.watchPath:
			default:
			}
		}
	}
}

func (h *SSEHub) addClient(c *sseClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	if absPath, err := SafePath(h.root, c.watchPath); err == nil {
		h.watcher.Add(absPath)
	}
}

func (h *SSEHub) removeClient(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)

	// Remove the watched path if no remaining client needs it.
	stillWatched := false
	for other := range h.clients {
		if other.watchPath == c.watchPath {
			stillWatched = true
			break
		}
	}
	h.mu.Unlock()

	if !stillWatched {
		if absPath, err := SafePath(h.root, c.watchPath); err == nil {
			h.watcher.Remove(absPath)
		}
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	watchPath := r.URL.Query().Get("watch")
	if watchPath == "" {
		http.Error(w, "watch parameter required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{
		watchPath: watchPath,
		events:    make(chan string, 1),
	}
	s.sseHub.addClient(client)
	defer s.sseHub.removeClient(client)

	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", toJSON(map[string]string{"type": "connected"}))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case path := <-client.events:
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", toJSON(map[string]string{
				"type": "change",
				"path": path,
			}))
			flusher.Flush()
		}
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
