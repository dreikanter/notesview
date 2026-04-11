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

// SidebarNode represents a file or directory in the sidebar tree.
type SidebarNode struct {
	Name     string
	Path     string
	IsDir    bool
	Children []SidebarNode
}

// layoutFields is the common chrome passed to every page.
type layoutFields struct {
	Title       string
	Breadcrumbs []Crumb
	Sidebar     []SidebarNode
	EditPath    string
}

// ViewData is the render context for a single-file view.
type ViewData struct {
	layoutFields
	FilePath    string
	Frontmatter *renderer.Frontmatter
	HTML        template.HTML
}

// BrowseData is the render context for a directory listing.
type BrowseData struct {
	layoutFields
	DirPath string
	Entries []BrowseEntry
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
	"templates/sidebar.html",
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
