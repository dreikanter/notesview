package server

import (
	"fmt"
	"html/template"
	"io"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/web"
)

type IndexEntry struct {
	Name     string
	IsDir    bool
	IsTag    bool
	Expanded bool
	Depth    int
	Href     string
}

// IndexCard is the sidebar's data shape.
type IndexCard struct {
	Entries []IndexEntry
	Empty   string
	// Flat suppresses the directory chevron toggle when rendering this
	// card — set on main-pane listings where the chevron has no meaning.
	Flat bool
}

// layoutFields is the common chrome passed to every full-page render.
type layoutFields struct {
	Title    string
	EditPath string
	EditHref string
}

// ViewData is the full-page render context for a note view.
type ViewData struct {
	layoutFields
	NotePath   string
	NoteTitle  string
	Note       *index.NoteEntry
	HTML       template.HTML
	SSEWatch   string
	ViewHref   string
	Sidebar    SidebarPartialData
	DirListing *DirListingData // non-nil when main panel shows a directory listing
}

// NotePartialData is the render context for an HX-Target: note-pane
// partial response. Only the fields the note-pane template needs;
// no sidebar, no topbar.
type NotePartialData struct {
	NotePath  string
	NoteTitle string
	Note      *index.NoteEntry
	HTML      template.HTML
	SSEWatch  string
	ViewHref  string
	EditPath  string
	EditHref  string
}

// SidebarPartialData is the render context for the sidebar tree.
type SidebarPartialData struct {
	Files       *IndexCard  // FILES section entries (rendered empty for now)
	Tags        *IndexCard  // TAGS section entries
	InitialJSON template.JS // {"selectedPath": "<path>" | null} — consumed by TreeView
}

// DirListingData is the render context for the dir_listing partial,
// used when a directory or tag listing is shown in the main panel.
type DirListingData struct {
	Title     string
	IndexCard *IndexCard
}

type templateSet struct {
	view       *template.Template
	sidebar    *template.Template
	note       *template.Template
	dirListing *template.Template
}

var partials = []string{
	"templates/layout.html",
	"templates/entry_list.html",
	"templates/sidebar_tree.html",
	"templates/sidebar_body.html",
	"templates/note_pane_body.html",
	"templates/dir_listing.html",
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
	dirListing, err := parsePartial("dir_listing")
	if err != nil {
		return nil, fmt.Errorf("parse dir-listing partial: %w", err)
	}
	return &templateSet{view: view, sidebar: sidebar, note: note, dirListing: dirListing}, nil
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
	return template.ParseFS(web.TemplatesFS, "templates/"+name+".html", "templates/entry_list.html", "templates/sidebar_tree.html")
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

func (t *templateSet) renderEntryList(w io.Writer, data *IndexCard) error {
	return t.sidebar.ExecuteTemplate(w, "entry_list", data)
}

func (t *templateSet) renderEntryListRows(w io.Writer, data *IndexCard) error {
	return t.sidebar.ExecuteTemplate(w, "entry_list_rows", data)
}

func (t *templateSet) renderDirListing(w io.Writer, data DirListingData) error {
	return t.dirListing.ExecuteTemplate(w, "dir_listing", data)
}
