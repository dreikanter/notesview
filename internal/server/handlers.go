package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dreikanter/nview/internal/index"
	"github.com/dreikanter/nview/internal/renderer"
)

// buildInitialJSON returns a pre-encoded JSON object retained for the
// TreeView component tests/future reuse. Empty string means no selection.
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

// buildSidebar assembles the metadata-driven sidebar shown on every full page.
func (s *Server) buildSidebar(selectedPath string) SidebarData {
	return SidebarData{
		RecentNotes: s.buildRecentNotes(12),
		Tags:        s.buildTagsIndex(),
		Types:       s.buildTypesIndex(),
		InitialJSON: buildInitialJSON(selectedPath),
	}
}

// editHref builds the API href for triggering the editor for a given note ID.
func editHref(noteID int) string {
	return fmt.Sprintf("/api/edit/%d", noteID)
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

	view := ViewData{
		layoutFields: layoutFields{Title: ""},
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 dark:text-gray-400 text-center py-8">No note selected.</p>`),
		ViewHref:     "/",
		Sidebar:      s.buildSidebar(""),
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
	if noteEntry != nil && noteEntry.ID > 0 {
		viewHref = fmt.Sprintf("/n/%d", noteEntry.ID)
	}

	if hxTargetedAt(r, "note-pane") {
		partial := NotePartialData{
			NotePath:  relPath,
			NoteTitle: noteTitle,
			Note:      noteEntry,
			HTML:      template.HTML(html),
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

	view := ViewData{
		layoutFields: layoutFields{Title: title, EditPath: editPath, EditHref: eHref},
		NotePath:     relPath,
		NoteTitle:    noteTitle,
		Note:         noteEntry,
		HTML:         template.HTML(html),
		ViewHref:     viewHref,
		Sidebar:      s.buildSidebar(relPath),
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

func (s *Server) buildRecentNotes(limit int) *IndexCard {
	notes := s.index.AllNotes()
	if limit > 0 && len(notes) > limit {
		notes = notes[:limit]
	}
	return notesCard(notes, "No notes found.")
}

// buildTagsIndex assembles an IndexCard in tags mode from the tag index.
func (s *Server) buildTagsIndex() *IndexCard {
	tags := s.index.Tags()
	entries := make([]IndexEntry, len(tags))
	for i, tag := range tags {
		entries[i] = IndexEntry{Name: tag, IsTag: true, Href: "/tags/" + tagPath(tag)}
	}
	return &IndexCard{Entries: entries, Empty: "No tags found."}
}

func (s *Server) buildTypesIndex() *IndexCard {
	types := s.index.Types()
	entries := make([]IndexEntry, len(types))
	for i, typ := range types {
		entries[i] = IndexEntry{Name: typ, IsType: true, Href: "/types/" + tagPath(typ)}
	}
	return &IndexCard{Entries: entries, Empty: "No types found."}
}

func (s *Server) notesByRelCard(rels []string, empty string) *IndexCard {
	notes := make([]index.NoteEntry, 0, len(rels))
	for _, rel := range rels {
		if noteEntry, ok := s.index.NoteEntryByRel(rel); ok {
			notes = append(notes, noteEntry)
		}
	}
	return notesCard(notes, empty)
}

func notesCard(notes []index.NoteEntry, empty string) *IndexCard {
	entries := make([]IndexEntry, len(notes))
	for i, noteEntry := range notes {
		name := noteEntry.Title
		if name == "" {
			name = fmt.Sprintf("#%d", noteEntry.ID)
		}
		date := ""
		if !noteEntry.Date.IsZero() {
			date = noteEntry.Date.Format("2006-01-02")
		}
		href := ""
		if noteEntry.ID > 0 {
			href = fmt.Sprintf("/n/%d", noteEntry.ID)
		}
		entries[i] = IndexEntry{
			Name:        name,
			Description: noteEntry.Description,
			Date:        date,
			ID:          noteEntry.ID,
			Slug:        noteEntry.Slug,
			Type:        noteEntry.Type,
			Href:        href,
		}
	}
	return &IndexCard{Entries: entries, Empty: empty}
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

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderSidebar(w, s.buildSidebar(r.URL.Query().Get("selected"))); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

// handleRefresh runs a Reconcile against the store and applies the diff to
// the index. The browser hits this on visibilitychange or via a manual
// refresh button to recover from missed events (sleep, lost SSE, etc.).
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	diff, err := s.index.Reconcile()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]int{
		"added":   len(diff.Added),
		"updated": len(diff.Updated),
		"deleted": len(diff.Removed),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Warn("write response failed", "err", err)
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
	s.renderIndexPage(w, r, "Tags", "/tags", card)
}

func (s *Server) handleTagNotes(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	card := s.notesByRelCard(s.index.NotesByTag(tag), "No notes with this tag.")
	s.renderIndexPage(w, r, tag, "/tags/"+tagPath(tag), card)
}

func (s *Server) handleTypes(w http.ResponseWriter, r *http.Request) {
	card := s.buildTypesIndex()
	s.renderIndexPage(w, r, "Types", "/types", card)
}

func (s *Server) handleTypeNotes(w http.ResponseWriter, r *http.Request) {
	noteType := r.PathValue("type")
	card := s.notesByRelCard(s.index.NotesByType(noteType), "No notes with this type.")
	s.renderIndexPage(w, r, noteType, "/types/"+tagPath(noteType), card)
}

func (s *Server) handleDates(w http.ResponseWriter, r *http.Request) {
	card := dateIndexCard(s.index.AllNotes(), 0, 0)
	s.renderIndexPage(w, r, "Dates", "/dates", card)
}

func (s *Server) handleDateYear(w http.ResponseWriter, r *http.Request) {
	year, ok := parseDatePart(w, r.PathValue("year"), 1, 9999)
	if !ok {
		return
	}
	card := dateIndexCard(s.index.AllNotes(), year, 0)
	s.renderIndexPage(w, r, r.PathValue("year"), "/dates/"+r.PathValue("year"), card)
}

func (s *Server) handleDateMonth(w http.ResponseWriter, r *http.Request) {
	year, ok := parseDatePart(w, r.PathValue("year"), 1, 9999)
	if !ok {
		return
	}
	month, ok := parseDatePart(w, r.PathValue("month"), 1, 12)
	if !ok {
		return
	}
	card := dateIndexCard(s.index.AllNotes(), year, month)
	title := fmt.Sprintf("%04d-%02d", year, month)
	s.renderIndexPage(w, r, title, "/dates/"+r.PathValue("year")+"/"+r.PathValue("month"), card)
}

func (s *Server) handleDateDay(w http.ResponseWriter, r *http.Request) {
	year, ok := parseDatePart(w, r.PathValue("year"), 1, 9999)
	if !ok {
		return
	}
	month, ok := parseDatePart(w, r.PathValue("month"), 1, 12)
	if !ok {
		return
	}
	day, ok := parseDatePart(w, r.PathValue("day"), 1, 31)
	if !ok {
		return
	}
	notes := filterNotesByDate(s.index.AllNotes(), year, month, day)
	title := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	s.renderIndexPage(w, r, title, "/dates/"+r.PathValue("year")+"/"+r.PathValue("month")+"/"+r.PathValue("day"), notesCard(notes, "No notes on this date."))
}

func (s *Server) renderIndexPage(w http.ResponseWriter, r *http.Request, title, href string, card *IndexCard) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	listing := DirListingData{Title: title, IndexCard: card}
	if hxTargetedAt(r, "note-pane") {
		if err := s.templates.renderDirListing(w, listing); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	view := ViewData{
		layoutFields: layoutFields{Title: title},
		ViewHref:     href,
		Sidebar:      s.buildSidebar(""),
		DirListing:   &listing,
	}
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func parseDatePart(w http.ResponseWriter, raw string, min, max int) (int, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		http.Error(w, "bad date path", http.StatusBadRequest)
		return 0, false
	}
	return n, true
}

func dateIndexCard(notes []index.NoteEntry, year, month int) *IndexCard {
	seen := map[string]IndexEntry{}
	for _, noteEntry := range notes {
		if noteEntry.Date.IsZero() {
			continue
		}
		y, m, d := noteEntry.Date.Date()
		if year == 0 {
			name := fmt.Sprintf("%04d", y)
			seen[name] = IndexEntry{Name: name, IsDate: true, Href: "/dates/" + name}
		} else if y == year && month == 0 {
			name := fmt.Sprintf("%02d", m)
			seen[name] = IndexEntry{Name: name, IsDate: true, Href: fmt.Sprintf("/dates/%04d/%02d", year, m)}
		} else if y == year && int(m) == month {
			name := fmt.Sprintf("%02d", d)
			seen[name] = IndexEntry{Name: name, IsDate: true, Href: fmt.Sprintf("/dates/%04d/%02d/%02d", year, month, d)}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	entries := make([]IndexEntry, len(keys))
	for i, k := range keys {
		entries[i] = seen[k]
	}
	return &IndexCard{Entries: entries, Empty: "No dated notes found."}
}

func filterNotesByDate(notes []index.NoteEntry, year, month, day int) []index.NoteEntry {
	out := make([]index.NoteEntry, 0)
	for _, noteEntry := range notes {
		if noteEntry.Date.IsZero() {
			continue
		}
		y, m, d := noteEntry.Date.Date()
		if y == year && int(m) == month && d == day {
			out = append(out, noteEntry)
		}
	}
	return out
}
