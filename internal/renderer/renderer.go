package renderer

import (
	"bytes"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/dreikanter/notesview/internal/index"
)

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
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	return &Renderer{md: md, index: idx}
}

func (r *Renderer) Render(source []byte, currentDir string) (string, *Frontmatter, error) {
	ctx := parser.NewContext()
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
	if r.index != nil {
		html = processNoteLinks(html, r.index, currentDir)
	}

	return html, fm, nil
}
