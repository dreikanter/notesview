package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type SSEHub struct {
	root    string
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	watcher *fsnotify.Watcher
	done    chan struct{}
}

type sseClient struct {
	watchPath string
	events    chan string
}

func NewSSEHub(root string) *SSEHub {
	return &SSEHub{
		root:    root,
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
	var debounce *time.Timer
	var lastPath string

	for {
		select {
		case <-h.done:
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			lastPath = event.Name
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(100*time.Millisecond, func() {
				h.broadcast(lastPath)
			})
		case _, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
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
	h.mu.Unlock()
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

	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", mustJSON(map[string]string{"type": "connected"}))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case path := <-client.events:
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", mustJSON(map[string]string{
				"type": "change",
				"path": path,
			}))
			flusher.Flush()
		}
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
