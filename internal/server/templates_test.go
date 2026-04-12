package server

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/dreikanter/notesview/internal/renderer"
)

func TestLoadTemplates(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}
	if ts.view == nil {
		t.Error("view template is nil")
	}
	if ts.sidebar == nil {
		t.Error("sidebar template is nil")
	}
	if ts.note == nil {
		t.Error("note template is nil")
	}
}

func TestLoadTemplates_DefinedTemplates(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	// The view template should include layout and all partials.
	for _, name := range []string{"layout", "sidebar_body", "note_pane_body", "breadcrumbs", "index_card"} {
		if ts.view.Lookup(name) == nil {
			t.Errorf("view template set missing %q", name)
		}
	}
}

func TestParsePage(t *testing.T) {
	tmpl, err := parsePage("templates/view.html")
	if err != nil {
		t.Fatalf("parsePage() error: %v", err)
	}

	// Should include the page plus all partials.
	for _, name := range []string{"layout", "sidebar_body", "note_pane_body", "breadcrumbs", "index_card"} {
		if tmpl.Lookup(name) == nil {
			t.Errorf("parsePage result missing template %q", name)
		}
	}
}

func TestParsePartial_SidebarBody(t *testing.T) {
	tmpl, err := parsePartial("sidebar_body")
	if err != nil {
		t.Fatalf("parsePartial(sidebar_body) error: %v", err)
	}
	if tmpl.Lookup("sidebar_body") == nil {
		t.Error("sidebar_body template not defined")
	}
}

func TestParsePartial_NotePaneBody(t *testing.T) {
	tmpl, err := parsePartial("note_pane_body")
	if err != nil {
		t.Fatalf("parsePartial(note_pane_body) error: %v", err)
	}
	if tmpl.Lookup("note_pane_body") == nil {
		t.Error("note_pane_body template not defined")
	}
}

func TestRenderView(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := ViewData{
		layoutFields: layoutFields{
			Title:    "Test Note",
			EditPath: "notes/test.md",
			EditHref: "/edit/notes/test.md",
		},
		NotePath:  "notes/test.md",
		NoteTitle: "Test Note",
		Frontmatter: &renderer.Frontmatter{
			Title:       "Test Note",
			Tags:        []string{"go", "testing"},
			Description: "A test note",
		},
		HTML:     template.HTML("<p>Hello world</p>"),
		SSEWatch: "/events?watch=notes%2Ftest.md",
		ViewHref: "/view/notes/test.md",
		IndexCard: &IndexCard{
			Mode: "dir",
			Breadcrumbs: BreadcrumbsData{
				Mode:   "dir",
				Crumbs: []Crumb{{Label: "notes", Href: "/dir/notes", Current: true}},
			},
			Entries: []IndexEntry{
				{Name: "test.md", IsDir: false, Href: "/view/notes/test.md"},
			},
		},
	}

	var buf bytes.Buffer
	if err := ts.renderView(&buf, data); err != nil {
		t.Fatalf("renderView() error: %v", err)
	}

	body := buf.String()
	checks := []struct {
		label    string
		contains string
	}{
		{"doctype", "<!DOCTYPE html>"},
		{"title", "Test Note — notesview"},
		{"note content", "Hello world"},
		{"frontmatter title", ">Test Note<"},
		{"tag go", ">go<"},
		{"tag testing", ">testing<"},
		{"description", "A test note"},
		{"sse-connect", `sse-connect="/events?watch=notes%2Ftest.md"`},
		{"sidebar", `id="sidebar"`},
		{"note-pane", `id="note-pane"`},
		{"edit button", `/edit/notes/test.md`},
	}

	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("renderView: expected %s (%q) in output", c.label, c.contains)
		}
	}
}

func TestRenderView_EmptyTitle(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := ViewData{
		HTML: template.HTML("<p>No title</p>"),
	}

	var buf bytes.Buffer
	if err := ts.renderView(&buf, data); err != nil {
		t.Fatalf("renderView() error: %v", err)
	}

	body := buf.String()
	// When Title is empty, the page title should just be "notesview" without a dash.
	if !strings.Contains(body, "<title>notesview</title>") {
		t.Error("expected plain 'notesview' title when Title is empty")
	}
}

func TestRenderNotePartial(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := NotePartialData{
		NotePath:  "2026/03/note.md",
		NoteTitle: "March Note",
		Frontmatter: &renderer.Frontmatter{
			Title: "March Note",
			Tags:  []string{"journal"},
		},
		HTML:     template.HTML("<p>Partial content</p>"),
		SSEWatch: "/events?watch=2026%2F03%2Fnote.md",
		ViewHref: "/view/2026/03/note.md",
		EditPath: "2026/03/note.md",
		EditHref: "/edit/2026/03/note.md",
	}

	var buf bytes.Buffer
	if err := ts.renderNotePartial(&buf, data); err != nil {
		t.Fatalf("renderNotePartial() error: %v", err)
	}

	body := buf.String()
	checks := []struct {
		label    string
		contains string
	}{
		{"note card", `id="note-card"`},
		{"note title", ">March Note<"},
		{"tag", ">journal<"},
		{"content", "Partial content"},
		{"sse-connect", `sse-connect="/events?watch=2026%2F03%2Fnote.md"`},
		{"edit button", `/edit/2026/03/note.md`},
	}

	for _, c := range checks {
		if !strings.Contains(body, c.contains) {
			t.Errorf("renderNotePartial: expected %s (%q) in output", c.label, c.contains)
		}
	}

	// Should NOT contain full layout elements.
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("renderNotePartial should not contain full HTML document")
	}
}

func TestRenderSidebarPartial(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{
		IndexCard: &IndexCard{
			Mode: "dir",
			Breadcrumbs: BreadcrumbsData{
				Mode:   "dir",
				Crumbs: []Crumb{{Label: "docs", Href: "/dir/docs", Current: true}},
			},
			Entries: []IndexEntry{
				{Name: "readme.md", IsDir: false, Href: "/view/docs/readme.md"},
				{Name: "subdir", IsDir: true, Href: "/view/docs/subdir/"},
			},
		},
	}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() error: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "readme.md") {
		t.Error("renderSidebarPartial: expected file entry 'readme.md'")
	}
	if !strings.Contains(body, "subdir") {
		t.Error("renderSidebarPartial: expected directory entry 'subdir'")
	}
	if !strings.Contains(body, "docs") {
		t.Error("renderSidebarPartial: expected breadcrumb 'docs'")
	}

	// Should NOT contain full layout elements.
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("renderSidebarPartial should not contain full HTML document")
	}
}

func TestRenderSidebarPartial_NilIndexCard(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := SidebarPartialData{IndexCard: nil}

	var buf bytes.Buffer
	if err := ts.renderSidebarPartial(&buf, data); err != nil {
		t.Fatalf("renderSidebarPartial() with nil IndexCard error: %v", err)
	}
}

func TestRenderNotePartial_NoFrontmatter(t *testing.T) {
	ts, err := loadTemplates()
	if err != nil {
		t.Fatalf("loadTemplates() error: %v", err)
	}

	data := NotePartialData{
		NotePath: "plain.md",
		HTML:     template.HTML("<p>Plain note</p>"),
	}

	var buf bytes.Buffer
	if err := ts.renderNotePartial(&buf, data); err != nil {
		t.Fatalf("renderNotePartial() error: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "Plain note") {
		t.Error("expected note content in output")
	}
}
