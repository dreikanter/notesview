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

	"github.com/dreikanter/nview/internal/index"
	"github.com/dreikanter/nview/internal/renderer"
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

// editHref builds the API href for triggering the editor for a given note ID.
func editHref(noteID int) string {
	return fmt.Sprintf("/api/edit/%d", noteID)
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

// handleRoot is the entry point for /. It redirects to /n/readme when a
// note with slug "readme" exists. Otherwise it renders the two-pane layout
// with an empty-state placeholder.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if _, ok := s.index.Resolve("readme"); ok {
		http.Redirect(w, r, "/n/readme", http.StatusFound)
		return
	}

	lf := layoutFields{Title: ""}
	tagsCard := s.buildTagsIndex()
	s.index.Rebuild()

	view := ViewData{
		layoutFields: lf,
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 dark:text-gray-400 text-center py-8">No note selected.</p>`),
		ViewHref:     "/",
		Sidebar: SidebarData{
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleNote serves GET /n/{x} where x is a slug or numeric ID.
func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	x := r.PathValue("x")

	relPath, ok := s.index.Resolve(x)
	if !ok {
		if hxTargetedAt(r, "note-pane") {
			s.writeNoteNotFoundPartial(w, x)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(s.root, filepath.FromSlash(relPath))
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			if hxTargetedAt(r, "note-pane") {
				s.writeNoteNotFoundPartial(w, x)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentDir := noteParentDir(relPath)

	html, err := s.renderer.Render(data, currentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var noteEntry *index.NoteEntry
	if entry, ok := s.index.NoteEntryByRel(relPath); ok {
		noteEntry = &entry
	}

	title := filepath.Base(relPath)
	if noteEntry != nil && noteEntry.Title != "" {
		title = noteEntry.Title
		html = renderer.StripRedundantTitle(html, noteEntry.Title)
	}
	noteTitle := title

	editPath := ""
	eHref := ""
	if s.editor != "" && noteEntry != nil && noteEntry.ID > 0 {
		editPath = relPath
		eHref = editHref(noteEntry.ID)
	}

	viewHref := "/n/" + url.PathEscape(x)

	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:  relPath,
			NoteTitle: noteTitle,
			Note:      noteEntry,
			HTML:      template.HTML(html),
			SSEWatch:  viewSSEWatch(relPath),
			ViewHref:  viewHref,
			EditPath:  editPath,
			EditHref:  eHref,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.templates.renderNotePartial(w, partial); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	lf := layoutFields{Title: title, EditPath: editPath, EditHref: eHref}
	tagsCard := s.buildTagsIndex()

	view := ViewData{
		layoutFields: lf,
		NotePath:     relPath,
		NoteTitle:    noteTitle,
		Note:         noteEntry,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(relPath),
		ViewHref:     viewHref,
		Sidebar: SidebarData{
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(relPath),
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
func (s *Server) writeNoteNotFoundPartial(w http.ResponseWriter, x string) {
	partial := NotePartialData{
		NoteTitle: x,
		HTML:      template.HTML(`<p class="text-gray-500 text-center py-8">Note not found.</p>`),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderNotePartial(w, partial); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	relPath, ok := s.index.NoteByID(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	absPath := filepath.Join(s.root, filepath.FromSlash(relPath))
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
		s.logger.Warn("write response failed", "id", id, "err", err)
	}
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	relPath, ok := s.index.NoteByID(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if s.editor == "" {
		http.Error(w, "no editor configured (set NVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}

	// Split the $EDITOR-style editor value on Unicode whitespace via
	// strings.Fields so values like `code --wait` or `subl -w` run as
	// binary + args. This is not a shell parser — quoting and escaping
	// are not honored. The recheck handles whitespace-only values that
	// pass the `== ""` guard but yield zero fields.
	fields := strings.Fields(s.editor)
	if len(fields) == 0 {
		http.Error(w, "no editor configured (set NVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}
	editorBin, editorArgs := fields[0], fields[1:]
	absPath := filepath.Join(s.root, filepath.FromSlash(relPath))
	args := append(append([]string{}, editorArgs...), absPath)

	if err := openEditor(editorBin, args); err != nil {
		http.Error(w, fmt.Sprintf("failed to start editor: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		s.logger.Warn("write response failed", "id", id, "err", err)
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
	view := ViewData{
		layoutFields: layoutFields{Title: "Tags"},
		ViewHref:     "/tags",
		Sidebar: SidebarData{
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
	for i, relPath := range notes {
		entry := IndexEntry{Name: relPath}
		if ne, ok := s.index.NoteEntryByRel(relPath); ok {
			entry.Type = ne.Type
			if ne.ID > 0 {
				entry.Href = fmt.Sprintf("/n/%d", ne.ID)
			}
		}
		entries[i] = entry
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No notes with this tag.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: tag, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	tagsCard := s.buildTagsIndex()
	view := ViewData{
		layoutFields: layoutFields{Title: tag},
		ViewHref:     "/tags/" + tagPath(tag),
		Sidebar: SidebarData{
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: tag, IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTypes(w http.ResponseWriter, r *http.Request) {
	types := s.index.Types()
	entries := make([]IndexEntry, len(types))
	for i, typ := range types {
		entries[i] = IndexEntry{
			Name: typ,
			Href: "/types/" + tagPath(typ),
		}
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No types found.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: "Types", IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	view := ViewData{
		layoutFields: layoutFields{Title: "Types"},
		ViewHref:     "/types",
		Sidebar: SidebarData{
			Tags:        s.buildTagsIndex(),
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: "Types", IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleTypeNotes(w http.ResponseWriter, r *http.Request) {
	typ := r.PathValue("type")
	notes := s.index.NotesByType(typ)
	entries := make([]IndexEntry, len(notes))
	for i, relPath := range notes {
		entry := IndexEntry{Name: relPath, Type: typ}
		if ne, ok := s.index.NoteEntryByRel(relPath); ok {
			if ne.ID > 0 {
				entry.Href = fmt.Sprintf("/n/%d", ne.ID)
			}
		}
		entries[i] = entry
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No notes with this type.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: typ, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	view := ViewData{
		layoutFields: layoutFields{Title: typ},
		ViewHref:     "/types/" + tagPath(typ),
		Sidebar: SidebarData{
			Tags:        s.buildTagsIndex(),
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: typ, IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDates(w http.ResponseWriter, r *http.Request) {
	dates := s.index.Dates()
	entries := make([]IndexEntry, len(dates))
	for i, d := range dates {
		entries[i] = IndexEntry{
			Name: d,
			Href: "/dates/" + d,
		}
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No dates found.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: "Dates", IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	view := ViewData{
		layoutFields: layoutFields{Title: "Dates"},
		ViewHref:     "/dates",
		Sidebar: SidebarData{
			Tags:        s.buildTagsIndex(),
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: "Dates", IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDateNotes serves GET /dates/{date} where date is YYYY, YYYY-MM, or
// YYYY-MM-DD. Dashes are stripped to form the YYYYMMDD prefix used by the
// index.
func (s *Server) handleDateNotes(w http.ResponseWriter, r *http.Request) {
	dateParam := r.PathValue("date")
	// Normalise "2026-03-31" → "20260331", "2026-03" → "202603", "2026" → "2026"
	prefix := strings.ReplaceAll(dateParam, "-", "")
	notes := s.index.NotesByDatePrefix(prefix)
	entries := make([]IndexEntry, len(notes))
	for i, relPath := range notes {
		entry := IndexEntry{Name: relPath}
		if ne, ok := s.index.NoteEntryByRel(relPath); ok {
			entry.Type = ne.Type
			if ne.ID > 0 {
				entry.Href = fmt.Sprintf("/n/%d", ne.ID)
			}
		}
		entries[i] = entry
	}
	card := &IndexCard{
		Entries: entries,
		Empty:   "No notes for this date.",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, DirListingData{Title: dateParam, IndexCard: card}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	view := ViewData{
		layoutFields: layoutFields{Title: dateParam},
		ViewHref:     "/dates/" + dateParam,
		Sidebar: SidebarData{
			Tags:        s.buildTagsIndex(),
			InitialJSON: buildInitialJSON(""),
		},
		DirListing: &DirListingData{Title: dateParam, IndexCard: card},
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
