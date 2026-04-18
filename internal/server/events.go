package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/internal/logging"
	"github.com/fsnotify/fsnotify"
)

// eventMsg is the internal envelope passed to subscribers.
// kind is "change" (file content) or "dir-changed" (tree mutation).
type eventMsg struct {
	kind string
	path string
}

type EventHub struct {
	root     string
	logger   *slog.Logger
	index    *index.NoteIndex
	mu       sync.RWMutex
	clients  map[*Subscription]struct{}
	watcher  *fsnotify.Watcher
	done     chan struct{}
	timerMu  sync.Mutex
	stopOnce sync.Once
}

type Subscription struct {
	watchPath string // "" means no file-change subscription
	events    chan eventMsg
}

func NewEventHub(root string, logger *slog.Logger, idx *index.NoteIndex) *EventHub {
	if logger == nil {
		logger = logging.Discard()
	}
	return &EventHub{
		root:    root,
		logger:  logger,
		index:   idx,
		clients: make(map[*Subscription]struct{}),
		done:    make(chan struct{}),
	}
}

func (h *EventHub) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	h.watcher = watcher
	if err := h.watchRecursive(h.root); err != nil {
		h.logger.Warn("initial recursive watch failed", "err", err)
	}
	go h.eventLoop()
	return nil
}

func (h *EventHub) watchRecursive(absDir string) error {
	return filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); strings.HasPrefix(name, ".") && p != absDir {
				return filepath.SkipDir
			}
			if err := h.watcher.Add(p); err != nil {
				h.logger.Warn("watcher add failed", "dir", p, "err", err)
			}
		}
		return nil
	})
}

func (h *EventHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.done)
		if h.watcher != nil {
			if err := h.watcher.Close(); err != nil {
				h.logger.Warn("file watcher close failed", "err", err)
			}
		}
	})
}

func (h *EventHub) eventLoop() {
	changeTimers := make(map[string]*time.Timer)
	dirTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-h.done:
			h.timerMu.Lock()
			for _, t := range changeTimers {
				t.Stop()
			}
			for _, t := range dirTimers {
				t.Stop()
			}
			h.timerMu.Unlock()
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			h.handleFSEvent(event, changeTimers, dirTimers)
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			h.logger.Warn("file watcher error", "err", err)
		}
	}
}

func (h *EventHub) handleFSEvent(event fsnotify.Event, changeTimers, dirTimers map[string]*time.Timer) {
	// A new subdir means we must watch it too.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if name := filepath.Base(event.Name); !strings.HasPrefix(name, ".") {
				if err := h.watchRecursive(event.Name); err != nil {
					h.logger.Warn("watcher add (new dir tree) failed", "dir", event.Name, "err", err)
				}
			}
		}
	}

	// File-content change: Write/Create on a .md file → 'change' broadcast.
	if event.Op&(fsnotify.Write|fsnotify.Create) != 0 &&
		strings.HasSuffix(strings.ToLower(event.Name), ".md") {
		p := event.Name
		h.timerMu.Lock()
		if t, ok := changeTimers[p]; ok {
			t.Stop()
		}
		changeTimers[p] = time.AfterFunc(100*time.Millisecond, func() {
			h.timerMu.Lock()
			delete(changeTimers, p)
			h.timerMu.Unlock()
			if h.index != nil {
				<-h.index.Rebuild()
			}
			h.broadcastChange(p)
		})
		h.timerMu.Unlock()
	}

	// Dir mutation: Create/Remove/Rename on a visible entry → 'dir-changed'
	// broadcast for the parent dir.
	if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		base := filepath.Base(event.Name)
		visible := !strings.HasPrefix(base, ".")
		if visible {
			// On Remove/Rename the path is gone — can't Stat. Treat it as
			// potentially visible; if the name was a non-.md file it'll be
			// dropped by the dir-listing anyway. For Create, check Stat to
			// filter out non-.md files.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && !info.IsDir() {
					visible = strings.HasSuffix(strings.ToLower(base), ".md")
				}
			}
		}
		if visible {
			parentAbs := filepath.Dir(event.Name)
			parentRel, err := filepath.Rel(h.root, parentAbs)
			if err == nil {
				if parentRel == "." {
					parentRel = ""
				}
				pr := parentRel
				h.timerMu.Lock()
				if t, ok := dirTimers[pr]; ok {
					t.Stop()
				}
				dirTimers[pr] = time.AfterFunc(200*time.Millisecond, func() {
					h.timerMu.Lock()
					delete(dirTimers, pr)
					h.timerMu.Unlock()
					h.broadcastDirChanged(pr)
				})
				h.timerMu.Unlock()
			}
		}
	}
}

func (h *EventHub) broadcastChange(absPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.clients {
		if sub.watchPath == "" {
			continue
		}
		safePath, err := SafePath(h.root, sub.watchPath)
		if err != nil || safePath != absPath {
			continue
		}
		select {
		case sub.events <- eventMsg{kind: "change", path: sub.watchPath}:
		default:
		}
	}
}

func (h *EventHub) broadcastDirChanged(relPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.clients {
		select {
		case sub.events <- eventMsg{kind: "dir-changed", path: relPath}:
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

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	watchPath := r.URL.Query().Get("watch")
	if watchPath != "" {
		if _, err := SafePath(s.root, watchPath); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := &Subscription{
		watchPath: watchPath,
		events:    make(chan eventMsg, 8),
	}
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
			switch msg.kind {
			case "change":
				fmt.Fprintf(w, "event: change\ndata: %s\n\n", toJSON(map[string]string{
					"type": "change",
					"path": msg.path,
				}))
			case "dir-changed":
				fmt.Fprintf(w, "event: dir-changed\ndata: %s\n\n", toJSON(map[string]string{
					"path": msg.path,
				}))
			}
			flusher.Flush()
		}
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
