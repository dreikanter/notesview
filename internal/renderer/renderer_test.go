package renderer

import (
	"strings"
	"testing"
)

func TestFrontmatterMissing(t *testing.T) {
	r := NewRenderer(nil)
	html, fm, err := r.Render([]byte("# Hello\n\nSome text."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %+v", fm)
	}
	if !strings.Contains(html, "Hello") {
		t.Errorf("expected HTML to contain heading, got: %s", html)
	}
}

func TestFrontmatterEmpty(t *testing.T) {
	r := NewRenderer(nil)
	html, fm, err := r.Render([]byte("---\n---\n# Hello"), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter for empty YAML block")
	}
	if fm.Title != "" || fm.Description != "" || fm.Slug != "" || len(fm.Tags) != 0 {
		t.Errorf("expected all fields empty, got %+v", fm)
	}
	if !strings.Contains(html, "Hello") {
		t.Errorf("expected HTML to contain heading, got: %s", html)
	}
}

func TestFrontmatterTitleOnly(t *testing.T) {
	r := NewRenderer(nil)
	_, fm, err := r.Render([]byte("---\ntitle: My Note\n---\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.Title != "My Note" {
		t.Errorf("title = %q, want %q", fm.Title, "My Note")
	}
	if fm.Description != "" || fm.Slug != "" || len(fm.Tags) != 0 {
		t.Errorf("expected other fields empty, got %+v", fm)
	}
}

func TestFrontmatterTagsOnly(t *testing.T) {
	r := NewRenderer(nil)
	_, fm, err := r.Render([]byte("---\ntags:\n  - go\n  - testing\n---\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.Title != "" {
		t.Errorf("title should be empty, got %q", fm.Title)
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "go" || fm.Tags[1] != "testing" {
		t.Errorf("tags = %v, want [go testing]", fm.Tags)
	}
}

func TestFrontmatterAllFields(t *testing.T) {
	r := NewRenderer(nil)
	input := "---\ntitle: Full\ndescription: A note\nslug: full-note\ntags:\n  - a\n  - b\n---\nBody."
	_, fm, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.Title != "Full" {
		t.Errorf("title = %q, want %q", fm.Title, "Full")
	}
	if fm.Description != "A note" {
		t.Errorf("description = %q, want %q", fm.Description, "A note")
	}
	if fm.Slug != "full-note" {
		t.Errorf("slug = %q, want %q", fm.Slug, "full-note")
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "a" || fm.Tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", fm.Tags)
	}
}

func TestFrontmatterMalformedYAML(t *testing.T) {
	r := NewRenderer(nil)
	// Scalar value instead of a mapping — goldmark-meta returns nil
	// because it expects a YAML map.
	_, fm, err := r.Render([]byte("---\njust a string\n---\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil frontmatter for non-mapping YAML, got %+v", fm)
	}
	_ = r
}

func TestFrontmatterUnknownKeys(t *testing.T) {
	r := NewRenderer(nil)
	// Valid YAML mapping but with keys we don't recognize — fm is
	// non-nil but all known fields should be zero-valued.
	_, fm, err := r.Render([]byte("---\nauthor: Jane\nrating: 5\n---\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter for valid YAML mapping")
	}
	if fm.Title != "" || fm.Description != "" || fm.Slug != "" || len(fm.Tags) != 0 {
		t.Errorf("expected all known fields empty, got %+v", fm)
	}
	_ = r
}

func TestFrontmatterWrongTypes(t *testing.T) {
	r := NewRenderer(nil)
	// title is a list, tags is a string — wrong types for our mapping.
	input := "---\ntitle:\n  - not\n  - a string\ntags: just-a-string\n---\nBody."
	_, fm, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm == nil {
		t.Fatal("expected non-nil frontmatter")
	}
	if fm.Title != "" {
		t.Errorf("title should be empty for non-string value, got %q", fm.Title)
	}
	if len(fm.Tags) != 0 {
		t.Errorf("tags should be empty for non-list value, got %v", fm.Tags)
	}
}

func TestEmptyDocument(t *testing.T) {
	r := NewRenderer(nil)
	html, fm, err := r.Render([]byte(""), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if fm != nil {
		t.Errorf("expected nil frontmatter for empty input, got %+v", fm)
	}
	if html != "" {
		t.Errorf("expected empty HTML for empty input, got %q", html)
	}
}

func TestStripRedundantTitleExact(t *testing.T) {
	r := NewRenderer(nil)
	input := "---\ntitle: Hello World\n---\n# Hello World\n\nBody text."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(html, "<h1") {
		t.Errorf("redundant <h1> should be stripped, got: %s", html)
	}
	if !strings.Contains(html, "Body text") {
		t.Errorf("body text should be preserved, got: %s", html)
	}
}

func TestStripRedundantTitleHTMLEntities(t *testing.T) {
	r := NewRenderer(nil)
	// Goldmark renders "A & B" in a heading as "A &amp; B".
	// stripRedundantTitle should decode entities before comparing.
	input := "---\ntitle: A & B\n---\n# A & B\n\nBody."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(html, "<h1") {
		t.Errorf("h1 with HTML entities should be stripped when matching title, got: %s", html)
	}
}

func TestStripRedundantTitleInlineMarkup(t *testing.T) {
	r := NewRenderer(nil)
	// "# **Bold** title" renders as <h1><strong>Bold</strong> title</h1>.
	// After stripping inner tags, the plain text is "Bold title".
	input := "---\ntitle: Bold title\n---\n# **Bold** title\n\nBody."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(html, "<h1") {
		t.Errorf("h1 with inline markup should be stripped when plain text matches title, got: %s", html)
	}
}

func TestStripRedundantTitleMismatch(t *testing.T) {
	r := NewRenderer(nil)
	// When the h1 content doesn't match the frontmatter title, keep it.
	input := "---\ntitle: Actual Title\n---\n# Different Heading\n\nBody."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "<h1") {
		t.Errorf("non-matching h1 should be preserved, got: %s", html)
	}
	if !strings.Contains(html, "Different Heading") {
		t.Errorf("heading text should be preserved, got: %s", html)
	}
}

func TestStripRedundantTitleLeadingWhitespace(t *testing.T) {
	// The leadingH1 regex tolerates leading whitespace before <h1>.
	got := stripRedundantTitle("  \n<h1>Hello</h1>\n<p>Body</p>", "Hello")
	if strings.Contains(got, "<h1") {
		t.Errorf("leading whitespace before h1 should still match, got: %s", got)
	}
	if !strings.Contains(got, "<p>Body</p>") {
		t.Errorf("body should be preserved, got: %s", got)
	}
}

func TestStripRedundantTitleNoH1(t *testing.T) {
	r := NewRenderer(nil)
	// Document with a title in frontmatter but no h1 in the body.
	input := "---\ntitle: My Title\n---\n## Subtitle\n\nBody."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "Subtitle") {
		t.Errorf("h2 should be preserved, got: %s", html)
	}
}

func TestStripRedundantTitleNoFrontmatterTitle(t *testing.T) {
	r := NewRenderer(nil)
	// Frontmatter exists but has no title — h1 should be preserved.
	input := "---\ntags:\n  - go\n---\n# Keep This\n\nBody."
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "<h1") {
		t.Errorf("h1 should be preserved when frontmatter has no title, got: %s", html)
	}
}
