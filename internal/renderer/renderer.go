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

	"github.com/dreikanter/notesview/internal/index"
)

// leadingH1 matches the first <h1>…</h1> at the start of the document,
// tolerating leading whitespace.
var leadingH1 = regexp.MustCompile(`(?s)^\s*<h1[^>]*>\s*(.*?)\s*</h1>`)

// tagStripper removes inline HTML tags from a heading's text content so we
// can compare it against a plain frontmatter title.
var tagStripper = regexp.MustCompile(`<[^>]+>`)

type Frontmatter struct {
	Title       string   `yaml:"title"`
	Tags        []string `yaml:"tags"`
	Description string   `yaml:"description"`
	Slug        string   `yaml:"slug"`
}

type Renderer struct {
	md    goldmark.Markdown
	index *index.Index
}

func NewRenderer(idx *index.Index) *Renderer {
	// NOTE: html.WithUnsafe() is deliberately NOT set. Without it, goldmark
	// escapes raw HTML from markdown sources (e.g. a malicious <script> block
	// becomes text). This matters even for a local-only previewer because a
	// note file cloned from an untrusted repo could otherwise run JS in the
	// notesview origin and hit the /api/edit endpoint.
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			NoteLinkExtension,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)
	return &Renderer{md: md, index: idx}
}

// Render converts markdown to HTML. currentDir is the note's parent
// directory relative to the notes root (used to resolve `[text](./rel.md)`).
// dirQuery is appended verbatim to every internal /view/... href emitted
// by the goldmark extension, so the caller can thread the current
// sidebar directory through wiki-link navigation. Pass "" when there
// is no panel state to preserve.
func (r *Renderer) Render(source []byte, currentDir, dirQuery string) (string, *Frontmatter, error) {
	ctx := parser.NewContext()
	if r.index != nil {
		ctx.Set(noteLinkStateKey, &noteLinkState{
			idx:        r.index,
			currentDir: currentDir,
			dirQuery:   dirQuery,
		})
	}

	var buf bytes.Buffer
	if err := r.md.Convert(source, &buf, parser.WithContext(ctx)); err != nil {
		return "", nil, err
	}

	var fm *Frontmatter
	metaData := meta.Get(ctx)
	if metaData != nil {
		fm = &Frontmatter{}
		if t, ok := metaData["title"].(string); ok {
			fm.Title = t
		}
		if d, ok := metaData["description"].(string); ok {
			fm.Description = d
		}
		if s, ok := metaData["slug"].(string); ok {
			fm.Slug = s
		}
		if tags, ok := metaData["tags"].([]interface{}); ok {
			for _, tag := range tags {
				if s, ok := tag.(string); ok {
					fm.Tags = append(fm.Tags, s)
				}
			}
		}
	}

	html := buf.String()
	html = processTaskSyntax(html)
	if fm != nil && fm.Title != "" {
		html = stripRedundantTitle(html, fm.Title)
	}
	return html, fm, nil
}

// stripRedundantTitle removes a leading <h1> whose plain-text content equals
// the frontmatter title, avoiding a duplicate heading when the frontmatter
// bar already shows the title. HTML entities in the heading are decoded so
// titles like "A & B" match both `# A & B` (rendered as "A &amp; B") and the
// plain frontmatter value.
func stripRedundantTitle(html, title string) string {
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
