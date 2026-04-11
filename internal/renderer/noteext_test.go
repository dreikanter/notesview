package renderer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dreikanter/notesview/internal/index"
)

func setupTestIndex(t *testing.T) *index.Index {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("# Todo"), 0o644); err != nil {
		t.Fatalf("write todo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026", "03", "20260330_9198.md"), []byte("# Note"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	idx := index.New(dir)
	idx.Build()
	return idx
}

func TestNoteProtocolLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [my todo](note://20260331_9201) for details.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := `<a href="/view/2026/03/20260331_9201_todo.md" hx-boost="true" hx-target="#note-pane"`
	if !strings.Contains(html, expected) {
		t.Errorf("note:// link missing expected anchor shape %q:\n%s", expected, html)
	}
}

func TestBrokenNoteLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [missing](note://99999999_0000) link.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="broken-link"`) {
		t.Errorf("broken note:// link not marked:\n%s", html)
	}
	if !strings.Contains(html, `href="#"`) {
		t.Errorf("broken link should href=\"#\":\n%s", html)
	}
	brokenRe := regexp.MustCompile(`<a [^>]*class="broken-link"[^>]*>`)
	match := brokenRe.FindString(html)
	if match == "" {
		t.Errorf("broken-link anchor not found:\n%s", html)
	}
	if strings.Contains(match, "hx-boost") {
		t.Errorf("broken-link anchor should not have hx-boost, got tag: %s", match)
	}
}

func TestAutoLinkUID(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`Refer to 20260331_9201 for the todo list.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := `<a href="/view/2026/03/20260331_9201_todo.md" class="uid-link" hx-boost="true" hx-target="#note-pane"`
	if !strings.Contains(html, expected) {
		t.Errorf("UID auto-link anchor shape mismatch:\n%s", html)
	}
}

// TestAutoLinkUIDAtLineStart pins the fact that a bare UID at the very
// start of a paragraph (no preceding space) still auto-links. This was
// a regression in the first pass of the goldmark extension which used
// a whitespace trigger for an inline parser.
func TestAutoLinkUIDAtLineStart(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`20260331_9201 is the primary note.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("UID at line start not auto-linked:\n%s", html)
	}
	if !strings.Contains(html, `class="uid-link"`) {
		t.Errorf("UID at line start missing uid-link class:\n%s", html)
	}
}

// TestUIDInsideLinkLabelNotWrapped guards against wrapping a bare UID
// in a nested <a> when the UID appears in the display text of an
// existing markdown link. The AST walker should skip Text nodes whose
// ancestor chain includes a Link.
func TestUIDInsideLinkLabelNotWrapped(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`[see 20260331_9201 here](https://example.com)`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	// There should be exactly one <a> tag (the outer markdown link),
	// not two (outer + nested UID autolink).
	if count := strings.Count(html, "<a "); count != 1 {
		t.Errorf("expected 1 <a> tag, got %d:\n%s", count, html)
	}
}

// TestAutoLinkUIDInCodeSpanSkipped pins that a bare UID inside an
// inline code span is left as literal text, not wrapped in a link.
func TestAutoLinkUIDInCodeSpanSkipped(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte("See `20260331_9201` in code."), "", "")
	if err != nil {
		t.Fatal(err)
	}
	// UID is inside a <code> span; the walker should skip it entirely.
	if strings.Contains(html, `class="uid-link"`) {
		t.Errorf("UID inside code span should not be auto-linked:\n%s", html)
	}
	if !strings.Contains(html, "20260331_9201") {
		t.Errorf("UID literal text should still appear in code span:\n%s", html)
	}
}

func TestAutoLinkUIDNoMatch(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`Reference 99999999_0000 does not exist.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `<a href="/view/`) {
		t.Errorf("non-matching UID should not be linked:\n%s", html)
	}
}

func TestRelativeMdLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [other](../01/foo.md) for details.`), "2026/03", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := `<a href="/view/2026/01/foo.md" hx-boost="true" hx-target="#note-pane"`
	if !strings.Contains(html, expected) {
		t.Errorf("relative .md link missing expected anchor shape %q:\n%s", expected, html)
	}
}

// TestExternalLinksStayPlain pins the "external links are plain HTML"
// rule. No hx-boost, no hx-target, no HTMX attributes of any kind on
// links whose destination points outside the notes system.
func TestExternalLinksStayPlain(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `[web](https://example.com) [mail](mailto:a@b.com) [asset](/static/foo.png)`
	html, _, err := r.Render([]byte(input), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, href := range []string{`href="https://example.com"`, `href="mailto:a@b.com"`, `href="/static/foo.png"`} {
		if !strings.Contains(html, href) {
			t.Errorf("expected %s in output:\n%s", href, html)
		}
	}
	// Sanity-check: none of the external-link anchors carry hx-* attrs.
	if strings.Contains(html, `href="https://example.com" hx-boost`) ||
		strings.Contains(html, `href="mailto:a@b.com" hx-boost`) ||
		strings.Contains(html, `href="/static/foo.png" hx-boost`) {
		t.Errorf("external link picked up hx-boost:\n%s", html)
	}
}

// TestDirQueryThreading pins the per-request state contract: when the
// renderer is given a dirQuery suffix, every internal /view/... href
// it emits must carry that suffix.
// TestDangerousURLsSanitized guards against malicious note content
// smuggling javascript:/vbscript:/file:/data: URLs into the rendered
// href. The renderer must rewrite these to "#" so clicking them is
// inert, matching the security comment in NewRenderer about cloned
// untrusted repos.
func TestDangerousURLsSanitized(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `[xss](javascript:alert(1)) [vb](vbscript:msgbox) [f](file:///etc/passwd) [d](data:text/html,<script>alert(1)</script>)`
	html, _, err := r.Render([]byte(input), "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"javascript:", "vbscript:", "file:", "data:text/html"} {
		if strings.Contains(html, bad) {
			t.Errorf("dangerous URL %q reached rendered href:\n%s", bad, html)
		}
	}
	// An image data URL is allowed (links may legitimately point at
	// base64 images in rare cases).
	html2, _, err := r.Render([]byte(`[ok](data:image/png;base64,iVBORw0K)`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html2, `data:image/png`) {
		t.Errorf("data:image/png URL should be preserved, got: %s", html2)
	}
}

func TestDirQueryThreading(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [todo](note://20260331_9201), [rel](../03/20260330_9198.md), and bare 20260331_9201.`
	html, _, err := r.Render([]byte(input), "2026/03", "?dir=2026%2F03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03"`) {
		t.Errorf("note:// link dropped dirQuery:\n%s", html)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260330_9198.md?dir=2026%2F03"`) {
		t.Errorf("relative .md link dropped dirQuery:\n%s", html)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03" class="uid-link"`) {
		t.Errorf("bare UID auto-link dropped dirQuery:\n%s", html)
	}
}
