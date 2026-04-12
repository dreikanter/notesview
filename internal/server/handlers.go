package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// parseDirParam normalizes the ?dir=... query parameter. An empty
// string means "no sticky directory" (reopen defaults to the note's
// parent). A slash-trimmed non-empty value is the directory the sidebar
// should show.
func parseDirParam(r *http.Request) (dir string, hasDir bool) {
	raw, ok := r.URL.Query()["dir"]
	if !ok {
		return "", false
	}
	return strings.Trim(raw[0], "/"), true
}

// buildLayoutFields assembles the common chrome every full-page render
// needs. effectiveDir is the directory the sidebar is showing — already
// resolved from either ?dir= or a handler-specific default (the note's
// parent).
func (s *Server) buildLayoutFields(title, editPath, effectiveDir string) layoutFields {
	lf := layoutFields{
		Title:    title,
		EditPath: editPath,
		DirQuery: dirQuery(effectiveDir),
	}
	if editPath != "" {
		lf.EditHref = "/api/edit/" + editPath
	}
	return lf
}

// viewSSEWatch is the value for the sse-connect attribute on note_pane_body.
// The SSE URL needs the note path percent-encoded because file names may
// contain spaces, slashes, question marks, etc.
func viewSSEWatch(filePath string) string {
	return "/events?watch=" + url.QueryEscape(filePath)
}

// hxTargetedAt returns true if this is an HTMX request whose target is
// the named element id (without the leading "#"). HTMX sends
// HX-Target as the raw id value.
func hxTargetedAt(r *http.Request, id string) bool {
	if r.Header.Get("HX-Request") != "true" {
		return false
	}
	return r.Header.Get("HX-Target") == id
}

// handleRoot is the entry point for /. It redirects to README.md if
// one exists at the notes root. Otherwise it renders the two-pane
// layout with an empty-state placeholder where the note would be.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Sidebar partial response via HX-Target: sidebar on /
	if hxTargetedAt(r, "sidebar") {
		sidebarDir, _ := parseDirParam(r)
		s.writeSidebarPartial(w, sidebarDir, "")
		return
	}

	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}

	// Empty state: render the two-pane layout with no note.
	sidebarDir, _ := parseDirParam(r)
	lf := s.buildLayoutFields("", "", sidebarDir)
	card, err := s.buildDirIndex(sidebarDir, "")
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
	}
	s.index.Rebuild()

	view := ViewData{
		layoutFields: lf,
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 text-center py-8">No note selected.</p>`),
		IndexCard:    card,
		ViewHref:     "/",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Sidebar partial: don't read the file at all.
	if hxTargetedAt(r, "sidebar") {
		explicitDir, _ := parseDirParam(r)
		if explicitDir == "" {
			explicitDir = noteParentDir(reqPath)
		}
		s.writeSidebarPartial(w, explicitDir, reqPath)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if hxTargetedAt(r, "note-pane") {
				// Empty-state partial with HTTP 200 so HTMX swaps it in.
				s.writeNoteNotFoundPartial(w, reqPath)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentDir := noteParentDir(reqPath)

	// Resolve the sidebar's sticky directory. ?dir= wins when present;
	// otherwise default to the note's parent.
	explicitDir, hasDir := parseDirParam(r)
	sidebarDir := currentDir
	if hasDir {
		sidebarDir = explicitDir
	}
	dq := dirQuery(sidebarDir)

	html, fm, err := s.renderer.Render(data, currentDir, dq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	title := filepath.Base(reqPath)
	if fm != nil && fm.Title != "" {
		title = fm.Title
	}
	noteTitle := title

	editPath := ""
	editHref := ""
	if s.editor != "" {
		editPath = reqPath
		editHref = "/api/edit/" + reqPath
	}

	// Note-pane partial response: return only the note body, no chrome.
	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:    reqPath,
			NoteTitle:   noteTitle,
			Frontmatter: fm,
			HTML:        template.HTML(html),
			SSEWatch:    viewSSEWatch(reqPath),
			ViewHref:    "/view/" + reqPath + dq,
			DirQuery:    dq,
			EditPath:    editPath,
			EditHref:    editHref,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.templates.renderNotePartial(w, partial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full page: build the sidebar too.
	lf := s.buildLayoutFields(title, editPath, sidebarDir)
	card, err := s.buildDirIndex(sidebarDir, reqPath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
	}

	view := ViewData{
		layoutFields: lf,
		NotePath:     reqPath,
		NoteTitle:    noteTitle,
		Frontmatter:  fm,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(reqPath),
		ViewHref:     "/view/" + reqPath + dq,
		IndexCard:    card,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeSidebarPartial renders just the sidebar fragment for a given
// directory and optional in-view note (for sticky links). The
// sidebarDir must be fully resolved before calling — this function
// takes no http.Request and has no fallback logic.
func (s *Server) writeSidebarPartial(w http.ResponseWriter, sidebarDir, notePath string) {
	card, err := s.buildDirIndex(sidebarDir, notePath)
	if err != nil {
		s.logger.Warn("sidebar build failed", "dir", sidebarDir, "err", err)
		card = &IndexCard{Mode: "dir", Empty: "Failed to read directory."}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebarPartial(w, SidebarPartialData{IndexCard: card}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeNoteNotFoundPartial serves the "note not found" fragment for an
// HX-Target: note-pane request, using HTTP 200 so HTMX swaps it in
// rather than skipping the swap on a 4xx status.
func (s *Server) writeNoteNotFoundPartial(w http.ResponseWriter, reqPath string) {
	partial := NotePartialData{
		NotePath:  reqPath,
		NoteTitle: filepath.Base(reqPath),
		HTML:      template.HTML(`<p class="text-gray-500 text-center py-8">Note not found.</p>`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderNotePartial(w, partial); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// buildDirIndex assembles an IndexCard in directory mode for a path
// relative to the notes root. notePath is the note currently in view
// (if any) — directory links in the resulting card will target that
// note with an updated ?dir= so the note stays visible when the user
// navigates the panel. Pass "" for the empty-state page.
func (s *Server) buildDirIndex(sidebarDir, notePath string) (*IndexCard, error) {
	absPath, err := SafePath(s.root, sidebarDir)
	if err != nil {
		return nil, err
	}
	entries, err := readDirEntries(absPath, sidebarDir, notePath)
	if err != nil {
		return nil, err
	}
	return &IndexCard{
		Mode:        "dir",
		Breadcrumbs: buildBreadcrumbs(sidebarDir, notePath),
		Entries:     entries,
		Empty:       "No files here.",
	}, nil
}

// noteParentDir returns the relative directory of a note path, or "" for
// notes at the root.
func noteParentDir(notePath string) string {
	d := filepath.Dir(notePath)
	if d == "." {
		return ""
	}
	return d
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(data); err != nil {
		s.logger.Warn("write response failed", "path", reqPath, "err", err)
	}
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.editor == "" {
		http.Error(w, "no editor configured (set NOTESVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}

	// Parse the editor env var the way shells treat $EDITOR: the first
	// token is the binary, the rest are leading arguments (e.g.
	// `code --wait`, `subl -w`, `nvim -R`). Without this split, exec
	// looks for a literal binary named `"code --wait"` and 500s. A
	// whitespace-only value slips past the `== ""` guard above but
	// yields zero fields, so recheck after Fields to avoid indexing a
	// nil slice and panicking the handler.
	fields := strings.Fields(s.editor)
	if len(fields) == 0 {
		http.Error(w, "no editor configured (set NOTESVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}
	editorBin, editorArgs := fields[0], fields[1:]
	args := append(append([]string{}, editorArgs...), absPath)

	if err := openEditor(editorBin, args); err != nil {
		http.Error(w, fmt.Sprintf("failed to start editor: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		s.logger.Warn("write response failed", "path", reqPath, "err", err)
	}
}
