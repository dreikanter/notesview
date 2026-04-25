package renderer

import (
	"bytes"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/dreikanter/nview/internal/index"
)

// leadingH1 matches the first <h1>…</h1> at the start of the document,
// tolerating leading whitespace.
var leadingH1 = regexp.MustCompile(`(?s)^\s*<h1[^>]*>\s*(.*?)\s*</h1>`)

// tagStripper removes inline HTML tags from a heading's text content so we
// can compare it against a plain frontmatter title.
var tagStripper = regexp.MustCompile(`<[^>]+>`)

type Renderer struct {
	md    goldmark.Markdown
	index *index.NoteIndex
}

func NewRenderer(idx *index.NoteIndex) *Renderer {
	// NOTE: html.WithUnsafe() is deliberately NOT set. Without it, goldmark
	// escapes raw HTML from markdown sources (e.g. a malicious <script> block
	// becomes text). This matters even for a local-only previewer because a
	// note file cloned from an untrusted repo could otherwise run JS in the
	// nview origin and hit the /api/edit endpoint.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			NoteLinkExtension,
			TaskCheckBoxExtension,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	return &Renderer{md: md, index: idx}
}

// Render converts markdown to HTML. currentDir is the note's parent
// directory relative to the notes root (used to resolve `[text](./rel.md)`).
// Frontmatter is consumed by goldmark-meta (so its YAML fence does not
// render as content), but this method does not return it — the index is
// the single source of truth for note metadata.
func (r *Renderer) Render(source []byte, currentDir string) (string, error) {
	ctx := parser.NewContext()
	if r.index != nil {
		ctx.Set(noteLinkStateKey, &noteLinkState{
			idx:        r.index,
			currentDir: currentDir,
		})
	}

	var buf bytes.Buffer
	if err := r.md.Convert(source, &buf, parser.WithContext(ctx)); err != nil {
		return "", err
	}

	html := buf.String()
	html = processTaskSyntax(html)
	return html, nil
}

// StripRedundantTitle removes a leading <h1> whose plain-text content equals
// title, avoiding a duplicate heading when the frontmatter bar already shows
// it. Returns html unchanged when title is empty or no match is found. HTML
// entities in the heading are decoded so titles like "A & B" match both
// `# A & B` (rendered as "A &amp; B") and the plain frontmatter value.
func StripRedundantTitle(html, title string) string {
	if title == "" {
		return html
	}
	m := leadingH1.FindStringSubmatchIndex(html)
	if m == nil {
		return html
	}
	innerStart, innerEnd := m[2], m[3]
	plain := stdhtml.UnescapeString(tagStripper.ReplaceAllString(html[innerStart:innerEnd], ""))
	if strings.TrimSpace(plain) != strings.TrimSpace(title) {
		return html
	}
	return html[m[1]:]
}
