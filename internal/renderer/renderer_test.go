package renderer

import (
	"strings"
	"testing"
)

func TestRenderBasicMarkdown(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Hello\n\nSome text."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(html, "Hello") {
		t.Errorf("expected HTML to contain heading, got: %s", html)
	}
	if !strings.Contains(html, "Some text") {
		t.Errorf("expected HTML to contain body, got: %s", html)
	}
}

// TestRenderSkipsFrontmatter pins that goldmark-meta still consumes the
// leading `---…---` fence so it is not rendered as content. We do not
// read the metadata — that is the index's job — but the fence must not
// leak into the HTML as an <hr> or plain text.
func TestRenderSkipsFrontmatter(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("---\ntitle: My Note\n---\n# Hello\n\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if strings.Contains(html, "title: My Note") {
		t.Errorf("frontmatter YAML leaked into HTML: %s", html)
	}
	if !strings.Contains(html, "Hello") {
		t.Errorf("expected body content in HTML: %s", html)
	}
}

func TestRenderEmptyDocument(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte(""), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if html != "" {
		t.Errorf("expected empty HTML for empty input, got %q", html)
	}
}

func TestStripRedundantTitleExact(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Hello World\n\nBody text."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html = StripRedundantTitle(html, "Hello World")
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
	// StripRedundantTitle should decode entities before comparing.
	html, err := r.Render([]byte("# A & B\n\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html = StripRedundantTitle(html, "A & B")
	if strings.Contains(html, "<h1") {
		t.Errorf("h1 with HTML entities should be stripped when matching title, got: %s", html)
	}
}

func TestStripRedundantTitleInlineMarkup(t *testing.T) {
	r := NewRenderer(nil)
	// "# **Bold** title" renders as <h1><strong>Bold</strong> title</h1>.
	// After stripping inner tags, the plain text is "Bold title".
	html, err := r.Render([]byte("# **Bold** title\n\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html = StripRedundantTitle(html, "Bold title")
	if strings.Contains(html, "<h1") {
		t.Errorf("h1 with inline markup should be stripped when plain text matches title, got: %s", html)
	}
}

func TestStripRedundantTitleMismatch(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Different Heading\n\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html = StripRedundantTitle(html, "Actual Title")
	if !strings.Contains(html, "<h1") {
		t.Errorf("non-matching h1 should be preserved, got: %s", html)
	}
	if !strings.Contains(html, "Different Heading") {
		t.Errorf("heading text should be preserved, got: %s", html)
	}
}

func TestStripRedundantTitleLeadingWhitespace(t *testing.T) {
	// The leadingH1 regex tolerates leading whitespace before <h1>.
	got := StripRedundantTitle("  \n<h1>Hello</h1>\n<p>Body</p>", "Hello")
	if strings.Contains(got, "<h1") {
		t.Errorf("leading whitespace before h1 should still match, got: %s", got)
	}
	if !strings.Contains(got, "<p>Body</p>") {
		t.Errorf("body should be preserved, got: %s", got)
	}
}

func TestStripRedundantTitleNoH1(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("## Subtitle\n\nBody."), "")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	html = StripRedundantTitle(html, "My Title")
	if !strings.Contains(html, "Subtitle") {
		t.Errorf("h2 should be preserved, got: %s", html)
	}
}

func TestStripRedundantTitleEmptyTitle(t *testing.T) {
	// An empty title must be a no-op even if the document starts with an
	// <h1> — otherwise a misconfigured caller could accidentally strip
	// legitimate headings.
	html := StripRedundantTitle("<h1>Keep This</h1><p>Body</p>", "")
	if !strings.Contains(html, "<h1") {
		t.Errorf("empty title should leave h1 intact, got: %s", html)
	}
}
