package server

import (
	"fmt"
	"html/template"
	"io"

	"github.com/dreikanter/notesview/internal/renderer"
	"github.com/dreikanter/notesview/web"
)

// Crumb is one segment of the breadcrumbs nav.
type Crumb struct {
	Label   string
	Href    string
	Current bool
}

// BreadcrumbsData is what the breadcrumbs partial renders. HomeHref is the
// root link, computed in the handler so it can preserve the index query
// string — the partial itself is a dumb renderer.
type BreadcrumbsData struct {
	HomeHref string
	Crumbs   []Crumb
}

// IndexEntry is one row in the index card's entry list.
type IndexEntry struct {
	Name  string
	IsDir bool
	Href  string
}

// IndexCard is the rendered-side shape of the navigation panel. Mode is a
// discriminator for future non-directory sources (search, tag). For now
// only "dir" is populated and the Breadcrumbs field carries the current
// path; future modes would add their own header fields (Query, Tag, …)
// rendered by the index_card template's mode switch.
type IndexCard struct {
	Mode        string
	Breadcrumbs BreadcrumbsData
	Entries     []IndexEntry
	Empty       string
}

// layoutFields is the common chrome passed to every page.
//
// IndexOpen drives whether the index card is rendered alongside the note
// card. It is set from the `?index=dir` query parameter so the state is
// linkable and carried across HTMX-boosted navigation via per-link query
// string preservation (see IndexQuery).
//
// ShowToggle is true on view pages (where the note card is the fallback
// view and the user can choose whether to also see the index) and false
// on browse pages (where the index IS the page and hiding it would leave
// a blank screen). ToggleHref is the URL the hamburger links to when
// ShowToggle is true.
type layoutFields struct {
	Title      string
	EditPath   string
	EditHref   string
	IndexOpen  bool
	IndexQuery string
	IndexCard  *IndexCard
	ShowToggle bool
	ToggleHref string
}

// ViewData is the render context for a single-file view.
type ViewData struct {
	layoutFields
	FilePath    string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
	SSEWatch    string
	ViewHref    string
}

// BrowseData is the render context for a directory listing. The browse
// page is index-card-only — there is no note card — so it reuses the
// IndexCard from layoutFields and adds nothing of its own beyond the path
// stashed on #content for client-side reference.
type BrowseData struct {
	layoutFields
	DirPath string
}

// templateSet holds the pre-parsed template sets used by the server.
type templateSet struct {
	view   *template.Template
	browse *template.Template
}

// partials are template files shared by every page.
var partials = []string{
	"templates/layout.html",
	"templates/breadcrumbs.html",
	"templates/index_card.html",
}

func loadTemplates() (*templateSet, error) {
	view, err := parsePage("templates/view.html")
	if err != nil {
		return nil, fmt.Errorf("parse view template: %w", err)
	}
	browse, err := parsePage("templates/browse.html")
	if err != nil {
		return nil, fmt.Errorf("parse browse template: %w", err)
	}
	return &templateSet{view: view, browse: browse}, nil
}

func parsePage(page string) (*template.Template, error) {
	files := append([]string{}, partials...)
	files = append(files, page)
	return template.ParseFS(web.TemplatesFS, files...)
}

func (t *templateSet) renderView(w io.Writer, data ViewData) error {
	return t.view.ExecuteTemplate(w, "layout", data)
}

func (t *templateSet) renderBrowse(w io.Writer, data BrowseData) error {
	return t.browse.ExecuteTemplate(w, "layout", data)
}
