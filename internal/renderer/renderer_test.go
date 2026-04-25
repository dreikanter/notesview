package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderBasicMarkdown(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Hello\n\nSome text."), "")
	require.NoError(t, err, "Render failed")
	assert.Contains(t, html, "Hello")
	assert.Contains(t, html, "Some text")
}

// TestRenderSkipsFrontmatter pins that goldmark-meta still consumes the
// leading `---…---` fence so it is not rendered as content. We do not
// read the metadata — that is the index's job — but the fence must not
// leak into the HTML as an <hr> or plain text.
func TestRenderSkipsFrontmatter(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("---\ntitle: My Note\n---\n# Hello\n\nBody."), "")
	require.NoError(t, err, "Render failed")
	assert.NotContains(t, html, "title: My Note", "frontmatter YAML leaked into HTML")
	assert.Contains(t, html, "Hello")
}

func TestRenderEmptyDocument(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte(""), "")
	require.NoError(t, err, "Render failed")
	assert.Empty(t, html)
}

func TestStripRedundantTitleExact(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Hello World\n\nBody text."), "")
	require.NoError(t, err, "Render failed")
	html = StripRedundantTitle(html, "Hello World")
	assert.NotContains(t, html, "<h1", "redundant <h1> should be stripped")
	assert.Contains(t, html, "Body text")
}

func TestStripRedundantTitleHTMLEntities(t *testing.T) {
	r := NewRenderer(nil)
	// Goldmark renders "A & B" in a heading as "A &amp; B".
	// StripRedundantTitle should decode entities before comparing.
	html, err := r.Render([]byte("# A & B\n\nBody."), "")
	require.NoError(t, err, "Render failed")
	html = StripRedundantTitle(html, "A & B")
	assert.NotContains(t, html, "<h1", "h1 with HTML entities should be stripped when matching title")
}

func TestStripRedundantTitleInlineMarkup(t *testing.T) {
	r := NewRenderer(nil)
	// "# **Bold** title" renders as <h1><strong>Bold</strong> title</h1>.
	// After stripping inner tags, the plain text is "Bold title".
	html, err := r.Render([]byte("# **Bold** title\n\nBody."), "")
	require.NoError(t, err, "Render failed")
	html = StripRedundantTitle(html, "Bold title")
	assert.NotContains(t, html, "<h1", "h1 with inline markup should be stripped when plain text matches title")
}

func TestStripRedundantTitleMismatch(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("# Different Heading\n\nBody."), "")
	require.NoError(t, err, "Render failed")
	html = StripRedundantTitle(html, "Actual Title")
	assert.Contains(t, html, "<h1", "non-matching h1 should be preserved")
	assert.Contains(t, html, "Different Heading")
}

func TestStripRedundantTitleLeadingWhitespace(t *testing.T) {
	// The leadingH1 regex tolerates leading whitespace before <h1>.
	got := StripRedundantTitle("  \n<h1>Hello</h1>\n<p>Body</p>", "Hello")
	assert.NotContains(t, got, "<h1", "leading whitespace before h1 should still match")
	assert.Contains(t, got, "<p>Body</p>")
}

func TestStripRedundantTitleNoH1(t *testing.T) {
	r := NewRenderer(nil)
	html, err := r.Render([]byte("## Subtitle\n\nBody."), "")
	require.NoError(t, err, "Render failed")
	html = StripRedundantTitle(html, "My Title")
	assert.Contains(t, html, "Subtitle")
}

func TestStripRedundantTitleEmptyTitle(t *testing.T) {
	// An empty title must be a no-op even if the document starts with an
	// <h1> — otherwise a misconfigured caller could accidentally strip
	// legitimate headings.
	html := StripRedundantTitle("<h1>Keep This</h1><p>Body</p>", "")
	assert.Contains(t, html, "<h1", "empty title should leave h1 intact")
}
