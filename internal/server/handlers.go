package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/internal/renderer"
)

// buildInitialJSON returns a pre-encoded JSON object that the TreeView
// component reads from the <script id="tv-initial"> on first render. The
// selectedPath drives ancestor pre-expansion and selection. Empty string
// means no selection (e.g., empty root, tags page).
func buildInitialJSON(selectedPath string) template.JS {
	payload := struct {
		SelectedPath *string `json:"selectedPath"`
	}{}
	if selectedPath != "" {
		payload.SelectedPath = &selectedPath
	}
	b, _ := json.Marshal(payload)
	return template.JS(b)
}

// buildLayoutFields assembles the common chrome every full-page render needs.
func (s *Server) buildLayoutFields(title, editPath string) layoutFields {
	lf := layoutFields{
		Title:    title,
		EditPath: editPath,
	}
	if editPath != "" {
		lf.EditHref = editHref(editPath)
	}
	return lf
}

// editHref builds the API href for opening a note in the editor.
func editHref(editPath string) string {
	return "/api/edit/" + viewPath(editPath)
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

	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}

	// Empty state: render the two-pane layout with no note.
	lf := s.buildLayoutFields("", "")
	tree, err := buildDirTree(s.root, "")
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", "", "err", err)
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
	tagsCard := s.buildTagsIndex()
	s.index.Rebuild()

	view := ViewData{
		layoutFields: lf,
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 text-center py-8">No note selected.</p>`),
		ViewHref:     "/",
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
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

	html, err := s.renderer.Render(data, currentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var note *index.NoteEntry
	if entry, ok := s.index.NoteEntryByRel(reqPath); ok {
		note = &entry
	}

	title := filepath.Base(reqPath)
	if note != nil && note.Title != "" {
		title = note.Title
		html = renderer.StripRedundantTitle(html, note.Title)
	}
	noteTitle := title

	editPath := ""
	eHref := ""
	if s.editor != "" {
		editPath = reqPath
		eHref = editHref(reqPath)
	}

	// Note-pane partial response: return only the note body, no chrome.
	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:  reqPath,
			NoteTitle: noteTitle,
			Note:      note,
			HTML:      template.HTML(html),
			SSEWatch:  viewSSEWatch(reqPath),
			ViewHref:  "/view/" + viewPath(reqPath),
			EditPath:  editPath,
			EditHref:  eHref,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.templates.renderNotePartial(w, partial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full page: build sidebar tree with both sections.
	sidebarDir := currentDir
	lf := s.buildLayoutFields(title, editPath)

	tree, err := buildDirTree(s.root, sidebarDir)
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", sidebarDir, "err", err)
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
	tagsCard := s.buildTagsIndex()

	view := ViewData{
		layoutFields: lf,
		NotePath:     reqPath,
		NoteTitle:    noteTitle,
		Note:         note,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(reqPath),
		ViewHref:     "/view/" + viewPath(reqPath),
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(reqPath),
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
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
// relative to the notes root.
func (s *Server) buildDirIndex(sidebarDir string) (*IndexCard, error) {
	absPath, err := SafePath(s.root, sidebarDir)
	if err != nil {
		return nil, err
	}
	entries, err := readDirEntries(absPath, sidebarDir)
	if err != nil {
		return nil, err
	}
	return &IndexCard{
		Entries: entries,
		Empty:   "No files here.",
	}, nil
}

// buildTagsIndex assembles an IndexCard in tags mode from the tag index.
func (s *Server) buildTagsIndex() *IndexCard {
	tags := s.index.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{
			Name:  tag,
			IsTag: true,
			Href:  "/tags/" + tagPath(tag),
		}
	}
	return &IndexCard{
		Entries: entries,
		Empty:   "No tags found.",
	}
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

func (s *Server) handleDir(w http.ResponseWriter, r *http.Request) {
	dirPath := r.PathValue("path")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Main panel partial: flat listing of this directory.
	if hxTargetedAt(r, "note-pane") {
		card, err := s.buildDirIndex(dirPath)
		if err != nil {
			s.logger.Warn("dir listing build failed", "dir", dirPath, "err", err)
			card = &IndexCard{Empty: "Failed to read directory."}
		}
		card.Flat = true
		title := dirPath
		if title == "" {
			title = "/"
		}
		if err := s.templates.renderDirListing(w, DirListingData{Title: title, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Children-only partial: flat <li> rows for this dir's immediate
	// children, tagged at the requested depth. Used by the sidebar
	// chevron to expand a dir in place without re-rendering the whole
	// tree (so the clicked row stays under the cursor).
	if r.URL.Query().Get("children") == "1" {
		depth, err := strconv.Atoi(r.URL.Query().Get("depth"))
		if err != nil || depth < 0 {
			http.Error(w, "invalid depth", http.StatusBadRequest)
			return
		}
		absPath, err := SafePath(s.root, dirPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		entries, err := readDirEntries(absPath, dirPath)
		if err != nil {
			s.logger.Warn("children listing build failed", "dir", dirPath, "err", err)
			entries = nil
		}
		for i := range entries {
			entries[i].Depth = depth
		}
		if err := s.templates.renderEntryListRows(w, &IndexCard{Entries: entries}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Sidebar partial: tree with ancestor chain.
	if r.Header.Get("HX-Request") == "true" {
		tree, err := buildDirTree(s.root, dirPath)
		if err != nil {
			s.logger.Warn("sidebar tree build failed", "dir", dirPath, "err", err)
			tree = nil
		}
		card := &IndexCard{Entries: tree, Empty: "No files here."}
		if err := s.templates.renderEntryList(w, card); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full-page load (direct URL visit / reload): render two-pane layout
	// with directory listing in the main panel.
	card, err := s.buildDirIndex(dirPath)
	if err != nil {
		s.logger.Warn("dir listing build failed", "dir", dirPath, "err", err)
		card = &IndexCard{Empty: "Failed to read directory."}
	}
	card.Flat = true
	tree, err := buildDirTree(s.root, dirPath)
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", dirPath, "err", err)
	}
	title := dirPath
	if title == "" {
		title = "/"
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
	tagsCard := s.buildTagsIndex()

	view := ViewData{
		layoutFields: s.buildLayoutFields(title, ""),
		HTML:         template.HTML(""),
		ViewHref:     "/dir/" + viewPath(dirPath),
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(dirPath),
		},
		DirListing: &DirListingData{Title: title, IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	card := s.buildTagsIndex()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: "Tags", IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		if err := s.templates.renderEntryList(w, card); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	// Full-page load
	filesCard, _ := s.buildDirIndex("")
	if filesCard == nil {
		filesCard = &IndexCard{Empty: "No files here."}
	}
	view := ViewData{
		layoutFields: s.buildLayoutFields("Tags", ""),
		ViewHref:     "/tags",
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        card,
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: "Tags", IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTagNotes(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	notes := s.index.NotesByTag(tag)
	entries := make([]IndexEntry, len(notes))
	for i, notePath := range notes {
		entries[i] = IndexEntry{
			Name: notePath,
			Href: "/view/" + viewPath(notePath),
		}
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No notes with this tag.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// HTMX partial: note-pane listing of tagged notes.
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: tag, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full-page load (direct URL visit / reload).
	filesCard, _ := s.buildDirIndex("")
	if filesCard == nil {
		filesCard = &IndexCard{Empty: "No files here."}
	}
	tagsCard := s.buildTagsIndex()
	view := ViewData{
		layoutFields: s.buildLayoutFields(tag, ""),
		ViewHref:     "/tags/" + tagPath(tag),
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: tag, IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
