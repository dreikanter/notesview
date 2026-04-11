// Package renderer's noteext.go implements NoteLinkExtension: a goldmark
// extension that rewrites internal link destinations, auto-links bare
// note UIDs, and emits HTMX attributes on internal <a> tags.
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

	"github.com/dreikanter/notesview/internal/index"
)

// noteLinkStateKey identifies per-request state stored in parser.Context.
// The state travels from the Renderer.Render call into the AST transformer
// during goldmark's parse phase, and is discarded when the request is done.
var noteLinkStateKey = parser.NewContextKey()

// noteLinkState is the per-request context the extension reads during
// parsing. currentDir is the note's parent directory (used to resolve
// relative .md links); dirQuery is the URL suffix ("?dir=..." or "")
// threaded into every rewritten internal href for panel state coherence.
type noteLinkState struct {
	idx        *index.Index
	currentDir string
	dirQuery   string
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
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&noteLinkRenderer{}, 100),
		),
	)
}

// noteLinkTransformer walks the AST twice after parsing. The first
// pass rewrites *ast.Link destinations for note:// and relative .md
// refs. The second pass collects runs of contiguous Text siblings
// outside existing links, code spans, and code blocks, scans each run
// for bare UIDs, and wraps resolved ones in new *ast.Link children.
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

	// First pass: rewrite Link destinations.
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if link, ok := n.(*ast.Link); ok {
			rewriteLinkDestination(link, state)
		}
		return ast.WalkContinue, nil
	})

	// Second pass: auto-link bare UIDs. Goldmark splits Text nodes at
	// emphasis-candidate bytes like '_', so a UID like "20260331_9201"
	// may be split across two sibling Text nodes whose segments are
	// contiguous in the source. We collect runs of contiguous Text
	// siblings (under auto-linkable parents), scan each run as a
	// single virtual span, and replace the nodes for each match.
	//
	// Collect runs first to avoid mutating during the walk.
	var runs [][]*ast.Text
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		// Skip into subtrees that should not be auto-linked. Returning
		// SkipChildren prevents us from descending into Link labels,
		// code spans, or code blocks.
		switch n.Kind() {
		case ast.KindLink, ast.KindAutoLink, ast.KindCodeSpan,
			ast.KindCodeBlock, ast.KindFencedCodeBlock:
			return ast.WalkSkipChildren, nil
		}
		// Gather contiguous Text children runs for this node.
		var run []*ast.Text
		flush := func() {
			if len(run) > 0 {
				runs = append(runs, run)
				run = nil
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			t, ok := c.(*ast.Text)
			if !ok {
				flush()
				continue
			}
			if len(run) == 0 {
				run = append(run, t)
				continue
			}
			prev := run[len(run)-1]
			if prev.Segment.Stop == t.Segment.Start {
				run = append(run, t)
			} else {
				flush()
				run = append(run, t)
			}
		}
		flush()
		return ast.WalkContinue, nil
	})
	src := reader.Source()
	for _, run := range runs {
		autoLinkUIDsInRun(run, src, state)
	}
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
		if relPath, ok := s.idx.Lookup(uid); ok {
			n.Destination = []byte("/view/" + relPath + s.dirQuery)
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
	n.Destination = []byte("/view/" + resolved + s.dirQuery)
}

// autoLinkUIDsInRun scans a run of contiguous Text siblings for bare
// UIDs (8 digits, underscore, 4+ digits) that resolve in the index and
// rewrites covered Text nodes into a sequence of Text + Link siblings.
// The run must be non-empty and consist of Text nodes whose segments
// are adjacent in the source (prev.Stop == next.Start). The run's
// nodes must share the same parent.
func autoLinkUIDsInRun(run []*ast.Text, src []byte, s *noteLinkState) {
	if len(run) == 0 {
		return
	}
	runStart := run[0].Segment.Start
	runStop := run[len(run)-1].Segment.Stop
	content := src[runStart:runStop]

	// Find all non-overlapping UID positions within content using a
	// manual word-boundary scan. A UID is 8 digits + '_' + 4+ digits,
	// preceded by start-of-content or a non-word byte, and followed by
	// end-of-content or a non-word byte.
	var ranges [][2]int
	i := 0
	for i < len(content) {
		// A candidate UID must start either at position 0, or right
		// after a non-word byte. Find such a position.
		if i > 0 && isWordByte(content[i-1]) {
			// Not at a word boundary; skip ahead.
			i++
			continue
		}
		// Try to match a UID at position i: 8 digits, '_', 4+ digits,
		// followed by end-of-content or a non-word byte.
		if i+13 > len(content) {
			break
		}
		if !allDigits(content[i:i+8]) || content[i+8] != '_' {
			i++
			continue
		}
		k := i + 9
		for k < len(content) && content[k] >= '0' && content[k] <= '9' {
			k++
		}
		if k-(i+9) < 4 {
			i++
			continue
		}
		if k < len(content) && isWordByte(content[k]) {
			i++
			continue
		}
		ranges = append(ranges, [2]int{i, k})
		i = k
	}

	if len(ranges) == 0 {
		return
	}

	// Filter to UIDs that resolve in the index.
	type match struct {
		start, end int
		relPath    string
	}
	var matches []match
	for _, r := range ranges {
		uid := string(content[r[0]:r[1]])
		if relPath, ok := s.idx.Lookup(uid); ok {
			matches = append(matches, match{r[0], r[1], relPath})
		}
	}
	if len(matches) == 0 {
		return
	}

	parent := run[0].Parent()
	if parent == nil {
		return
	}

	// Build replacement nodes: Text for gaps between matches, Link for
	// each match. Offsets are relative to content; we add runStart to
	// convert back to source positions.
	cursor := 0
	var newNodes []ast.Node
	for _, m := range matches {
		if m.start > cursor {
			leading := ast.NewTextSegment(text.NewSegment(runStart+cursor, runStart+m.start))
			newNodes = append(newNodes, leading)
		}
		link := ast.NewLink()
		link.Destination = []byte("/view/" + m.relPath + s.dirQuery)
		link.SetAttributeString("class", []byte("uid-link"))
		linkText := ast.NewTextSegment(text.NewSegment(runStart+m.start, runStart+m.end))
		link.AppendChild(link, linkText)
		newNodes = append(newNodes, link)
		cursor = m.end
	}
	if cursor < len(content) {
		trailing := ast.NewTextSegment(text.NewSegment(runStart+cursor, runStart+len(content)))
		newNodes = append(newNodes, trailing)
	}

	// Insert replacements before the first run node, then remove all
	// original run nodes.
	anchor := run[0]
	for _, newNode := range newNodes {
		parent.InsertBefore(parent, anchor, newNode)
	}
	for _, original := range run {
		parent.RemoveChild(parent, original)
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func allDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
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

		// Emit class (if any) before HTMX attrs so tests that check
		// for "href=... class=..." adjacency match regardless of
		// whether hx-* is present. Broken-link anchors also benefit:
		// their class="broken-link" and title="" show up together.
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
