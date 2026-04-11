package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/dreikanter/notesview/internal/index"
	"github.com/dreikanter/notesview/internal/renderer"
	"github.com/dreikanter/notesview/web"
)

type Server struct {
	root      string
	editor    string
	renderer  *renderer.Renderer
	index     *index.Index
	sseHub    *SSEHub
	templates *templateSet
}

func NewServer(root, editor string) (*Server, error) {
	idx := index.New(root)
	idx.Build()
	tpls, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return &Server{
		root:      root,
		editor:    editor,
		renderer:  renderer.NewRenderer(idx),
		index:     idx,
		sseHub:    NewSSEHub(root),
		templates: tpls,
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /view/{filepath...}", s.handleView)
	mux.HandleFunc("GET /browse/{dirpath...}", s.handleBrowse)
	mux.HandleFunc("POST /api/edit/{filepath...}", s.handleEdit)
	mux.HandleFunc("GET /api/raw/{filepath...}", s.handleRaw)
	mux.HandleFunc("GET /events", s.handleSSE)
	mux.HandleFunc("GET /", s.handleRoot)

	staticFS, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	return rejectDirtyPaths(mux)
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
