package server

import (
	"fmt"
	"html/template"
	"io"

	"github.com/dreikanter/notesview/internal/renderer"
	"github.com/dreikanter/notesview/web"
)

type Crumb struct {
	Label   string
	Href    string
	Current bool
}

type BreadcrumbsData struct {
	HomeHref string
	Crumbs   []Crumb
}

type IndexEntry struct {
	Name  string
	IsDir bool
	Href  string
}

// IndexCard is the sidebar's data shape. Mode is kept as an extensibility
// hook for future non-directory sources (search, tag); today only "dir"
// is populated.
type IndexCard struct {
	Mode        string
	Breadcrumbs BreadcrumbsData
	Entries     []IndexEntry
	Empty       string
}

// layoutFields is the common chrome passed to every full-page render.
// DirQuery is the canonical "?dir=..." suffix appended to hrefs that
// need to preserve the sidebar's sticky directory (currently just the
// SSE live-reload hx-get URL).
type layoutFields struct {
	Title    string
	EditPath string
	EditHref string
	DirQuery string
}

// ViewData is the full-page render context for a note view.
type ViewData struct {
	layoutFields
	NotePath    string
	NoteTitle   string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
	IndexCard   *IndexCard
}

// NotePartialData is the render context for an HX-Target: note-pane
// partial response. Only the fields the note-pane template needs;
// no sidebar, no topbar.
type NotePartialData struct {
	NotePath    string
	NoteTitle   string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
	DirQuery    string
	EditPath    string
	EditHref    string
}

// SidebarPartialData is the render context for an HX-Target: sidebar
// partial response. Only the fields the sidebar-body template needs.
type SidebarPartialData struct {
	IndexCard *IndexCard
}

type templateSet struct {
	view    *template.Template
	sidebar *template.Template
	note    *template.Template
}

var partials = []string{
	"templates/layout.html",
	"templates/breadcrumbs.html",
	"templates/index_card.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
}

func loadTemplates() (*templateSet, error) {
	view, err := parsePage("templates/view.html")
	if err != nil {
		return nil, fmt.Errorf("parse view template: %w", err)
	}
	sidebar, err := parsePartial("sidebar_body")
	if err != nil {
		return nil, fmt.Errorf("parse sidebar partial: %w", err)
	}
	note, err := parsePartial("note_pane_body")
	if err != nil {
		return nil, fmt.Errorf("parse note-pane partial: %w", err)
	}
	return &templateSet{view: view, sidebar: sidebar, note: note}, nil
}

func parsePage(page string) (*template.Template, error) {
	files := append([]string{}, partials...)
	files = append(files, page)
	return template.ParseFS(web.TemplatesFS, files...)
}

// parsePartial loads only the files needed to render one partial
// template, so a partial response doesn't accidentally include the
// full layout.
func parsePartial(name string) (*template.Template, error) {
	return template.ParseFS(web.TemplatesFS, "templates/"+name+".html", "templates/breadcrumbs.html", "templates/index_card.html")
}

func (t *templateSet) renderView(w io.Writer, data ViewData) error {
	return t.view.ExecuteTemplate(w, "layout", data)
}

func (t *templateSet) renderNotePartial(w io.Writer, data NotePartialData) error {
	return t.note.ExecuteTemplate(w, "note_pane_body", data)
}

func (t *templateSet) renderSidebarPartial(w io.Writer, data SidebarPartialData) error {
	return t.sidebar.ExecuteTemplate(w, "sidebar_body", data)
}
