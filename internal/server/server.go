package server

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/internal/logging"
	"github.com/dreikanter/notes-view/internal/renderer"
	"github.com/dreikanter/notes-view/web"
)

type Server struct {
	root      string
	editor    string
	logger    *slog.Logger
	renderer  *renderer.Renderer
	index     *index.Index
	tagIndex  *index.TagIndex
	sseHub    *SSEHub
	templates *templateSet
}

// NewServer builds a Server rooted at the given notes directory. The logger
// is optional: a nil logger is replaced with a discard logger so handlers
// can always call s.logger.* without a nil check. Callers that want output
// should pass a logger built via internal/logging.
func NewServer(root, editor string, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = logging.Discard()
	}
	idx := index.NewLegacy(root, logger)
	if err := idx.Build(); err != nil {
		return nil, fmt.Errorf("initial index build: %w", err)
	}
	tagIdx := index.NewTagIndex(root, logger)
	if err := tagIdx.Build(); err != nil {
		return nil, fmt.Errorf("initial tag index build: %w", err)
	}
	tpls, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return &Server{
		root:      root,
		editor:    editor,
		logger:    logger,
		renderer:  renderer.NewRenderer(idx),
		index:     idx,
		tagIndex:  tagIdx,
		sseHub:    NewSSEHub(root, logger, idx, tagIdx),
		templates: tpls,
	}, nil
}

// Logger returns the server's structured logger. Exposed so cmd wiring can
// share one logger for both request logging and startup messages.
func (s *Server) Logger() *slog.Logger {
	return s.logger
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /view/{filepath...}", s.handleView)
	mux.HandleFunc("GET /dir/{path...}", s.handleDir)
	mux.HandleFunc("GET /tags", s.handleTags)
	mux.HandleFunc("GET /tags/{tag}", s.handleTagNotes)
	mux.HandleFunc("POST /api/edit/{filepath...}", s.handleEdit)
	mux.HandleFunc("GET /api/raw/{filepath...}", s.handleRaw)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.HandleFunc("GET /", s.handleRoot)

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		s.logger.Error("failed to open embedded static FS", "err", err)
	} else {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}

	return logRequests(s.logger)(rejectDirtyPaths(mux))
}

// rejectDirtyPaths returns 400 for any request whose raw URL path is not
// already clean (i.e. contains ".." segments), rather than letting the mux
// silently redirect them.
func rejectDirtyPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath := r.URL.Path
		if strings.Contains(rawPath, "..") {
			http.Error(w, "bad request: unclean path", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) StartWatcher() error {
	return s.sseHub.Start()
}

// Shutdown stops the SSE hub, closing the fsnotify watcher and draining
// connected clients.
func (s *Server) Shutdown() {
	s.sseHub.Stop()
}
