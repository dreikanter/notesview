// Package renderer's noteext.go implements NoteLinkExtension: a goldmark
// extension that rewrites internal link destinations, resolves [[UID]]
// wiki-links, and emits HTMX attributes on internal <a> tags.
package renderer

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/dreikanter/notes-view/internal/index"
)

// wikiLinkParser is a goldmark InlineParser that recognizes [[UID]]
// syntax and emits a Link node pointing at the resolved note. The
// trigger byte is '[', which goldmark dispatches on as punctuation.
// If the pattern doesn't match or the UID doesn't resolve, the parser
// returns nil and goldmark's standard link parser handles the '['.
type wikiLinkParser struct{}

func (p *wikiLinkParser) Trigger() []byte {
	return []byte{'['}
}

func (p *wikiLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	// Must start with [[
	if len(line) < 2 || line[0] != '[' || line[1] != '[' {
		return nil
	}

	// Find closing ]]
	end := bytes.Index(line[2:], []byte("]]"))
	if end < 0 {
		return nil
	}
	inner := line[2 : 2+end]

	// Validate UID pattern: 8 digits + '_' + 4+ digits
	if !isValidUID(inner) {
		return nil
	}
	uid := string(inner)

	stateAny := pc.Get(noteLinkStateKey)
	if stateAny == nil {
		return nil
	}
	state := stateAny.(*noteLinkState)
	if state.idx == nil {
		return nil
	}

	relPath, ok := state.idx.NoteByUID(uid)
	if !ok {
		return nil
	}

	// Consume [[ + UID + ]]
	block.Advance(2 + end + 2)
	link := ast.NewLink()
	link.Destination = []byte("/view/" + relPath)
	link.SetAttributeString("class", []byte("uid-link"))
	link.AppendChild(link, ast.NewString([]byte(uid)))
	return link
}

// isValidUID checks if b matches the UID pattern: exactly 8 digits,
// an underscore, then 4 or more digits.
func isValidUID(b []byte) bool {
	if len(b) < 13 { // 8 + 1 + 4 minimum
		return false
	}
	for i := 0; i < 8; i++ {
		if b[i] < '0' || b[i] > '9' {
			return false
		}
	}
	if b[8] != '_' {
		return false
	}
	for i := 9; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return false
		}
	}
	return true
}

// noteLinkStateKey identifies per-request state stored in parser.Context.
// The state travels from the Renderer.Render call into the AST transformer
// during goldmark's parse phase, and is discarded when the request is done.
var noteLinkStateKey = parser.NewContextKey()

// noteLinkState is the per-request context the extension reads during
// parsing. currentDir is the note's parent directory (used to resolve
// relative .md links).
type noteLinkState struct {
	idx        *index.NoteIndex
	currentDir string
}

// noteLinkExtension wires the AST transformer and custom link renderer
// into a goldmark.Markdown instance. The Renderer constructor registers
// this as an extension, once, at startup.
type noteLinkExtension struct{}

// NoteLinkExtension is the registerable goldmark extension. There is no
// per-request configuration here — everything that varies per request
// lives in parser.Context (see noteLinkState).
var NoteLinkExtension goldmark.Extender = &noteLinkExtension{}

func (e *noteLinkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&noteLinkTransformer{}, 100),
		),
		parser.WithInlineParsers(
			util.Prioritized(&wikiLinkParser{}, 99),
		),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&noteLinkRenderer{}, 100),
		),
	)
}

// noteLinkTransformer walks the AST after parsing and rewrites
// *ast.Link destinations for note:// and relative .md refs.
type noteLinkTransformer struct{}

func (t *noteLinkTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	stateAny := pc.Get(noteLinkStateKey)
	if stateAny == nil {
		return
	}
	state, ok := stateAny.(*noteLinkState)
	if !ok || state == nil || state.idx == nil {
		return
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if link, ok := n.(*ast.Link); ok {
			rewriteLinkDestination(link, state)
		}
		return ast.WalkContinue, nil
	})
}

// rewriteLinkDestination mutates a single *ast.Link in place. It handles
// two internal-link shapes: note://UID and relative .md paths. Anything
// else — absolute URLs with a scheme, protocol-relative, root-absolute
// paths, non-.md — is left alone and will render as a plain <a href>
// because the custom renderer's internal-link check won't match.
func rewriteLinkDestination(n *ast.Link, s *noteLinkState) {
	dest := string(n.Destination)

	if strings.HasPrefix(dest, "note://") {
		uid := strings.TrimPrefix(dest, "note://")
		if relPath, ok := s.idx.NoteByUID(uid); ok {
			n.Destination = []byte("/view/" + relPath)
		} else {
			n.Destination = []byte("#")
			n.SetAttributeString("class", []byte("broken-link"))
			n.SetAttributeString("title", []byte(fmt.Sprintf("Note %s not found", uid)))
		}
		return
	}

	// Relative .md: must end in .md, not be absolute, not have a scheme.
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") {
		return
	}
	if !strings.HasSuffix(dest, ".md") {
		return
	}
	resolved := path.Clean(path.Join(s.currentDir, dest))
	resolved = strings.TrimPrefix(resolved, "/")
	n.Destination = []byte("/view/" + resolved)
}

// noteLinkRenderer overrides goldmark's default *ast.Link renderer so
// we can emit HTMX attributes on internal links. External links (and
// anything else that didn't get rewritten to a /view/... destination)
// fall through as plain <a href>.
//
// This is a goldmark renderer.NodeRenderer; its RegisterFuncs method
// tells goldmark to call renderLink instead of the default when
// encountering *ast.Link nodes.
type noteLinkRenderer struct{}

func (r *noteLinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
}

// urlDisplayMax is the character threshold above which an autolink's
// visible label is trimmed with a trailing ellipsis. 60 fits on one line
// of the 900px note pane at default prose font size.
const urlDisplayMax = 60

// shortenURL trims u to roughly max characters plus a trailing "…". When
// possible it backs the cut up to the last "/" in the back half of the
// budget so the visible label ends on a path-segment boundary rather than
// mid-token. URLs are treated as byte strings — URL syntax is ASCII, and
// the rare Unicode host shows up percent-encoded in hrefs we see.
func shortenURL(u string, max int) string {
	if len(u) <= max {
		return u
	}
	head := u[:max]
	if i := strings.LastIndex(head, "/"); i > max/2 {
		head = head[:i]
	}
	return head + "…"
}

// renderAutoLink mirrors goldmark's default autolink renderer but trims the
// visible label with a trailing ellipsis when the URL is long, preserving
// the full URL in href and in a title attribute for hover/tap inspection.
func (r *noteLinkRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}
	url := n.URL(source)
	label := n.Label(source)
	_, _ = w.WriteString(`<a href="`)
	if n.AutoLinkType == ast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(url), []byte("mailto:")) {
		_, _ = w.WriteString("mailto:")
	}
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(url, false)))
	_ = w.WriteByte('"')
	display := string(label)
	shortened := shortenURL(display, urlDisplayMax)
	if shortened != display {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML([]byte(display)))
		_ = w.WriteByte('"')
	}
	_ = w.WriteByte('>')
	_, _ = w.Write(util.EscapeHTML([]byte(shortened)))
	_, _ = w.WriteString("</a>")
	return ast.WalkContinue, nil
}

func (r *noteLinkRenderer) renderLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		_, _ = w.WriteString(`<a href="`)
		if goldmarkhtml.IsDangerousURL(n.Destination) {
			_ = w.WriteByte('#')
		} else {
			_, _ = w.Write(util.EscapeHTML(util.URLEscape(n.Destination, true)))
		}
		_ = w.WriteByte('"')

		if v, ok := n.AttributeString("class"); ok {
			if b, ok := v.([]byte); ok {
				_, _ = w.WriteString(` class="`)
				_, _ = w.Write(util.EscapeHTML(b))
				_ = w.WriteByte('"')
			}
		}
		if isInternalLink(n.Destination) {
			_, _ = w.WriteString(` hx-boost="true" hx-target="#note-pane"`)
		}
		if v, ok := n.AttributeString("title"); ok {
			if b, ok := v.([]byte); ok {
				_, _ = w.WriteString(` title="`)
				_, _ = w.Write(util.EscapeHTML(b))
				_ = w.WriteByte('"')
			}
		} else if len(n.Title) > 0 {
			_, _ = w.WriteString(` title="`)
			_, _ = w.Write(util.EscapeHTML(n.Title))
			_ = w.WriteByte('"')
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

// isInternalLink returns true if the destination points at a note the
// server knows how to serve (via /view/...). Broken note:// links have
// been rewritten to "#" and do NOT count as internal — they should
// render without hx-boost so clicking them is inert.
func isInternalLink(dest []byte) bool {
	return bytes.HasPrefix(dest, []byte("/view/"))
}
