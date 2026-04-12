package renderer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dreikanter/notesview/internal/index"
)

// TestWikiLinkResolved verifies that [[UID]] syntax produces a link
// to the resolved note with uid-link class and HTMX attributes.
func TestWikiLinkResolved(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [[20260331_9201]] for details.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("[[UID]] not resolved:\n%s", html)
	}
	if !strings.Contains(html, `class="uid-link"`) {
		t.Errorf("[[UID]] missing uid-link class:\n%s", html)
	}
	if !strings.Contains(html, `hx-boost="true"`) {
		t.Errorf("[[UID]] missing hx-boost:\n%s", html)
	}
}

// TestWikiLinkUnresolved verifies that [[UID]] with a non-existent
// UID passes through as literal text (goldmark renders it as
// [[text]] since neither [ nor ] is consumed).
func TestWikiLinkUnresolved(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [[99999999_0000]] here.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `href="/view/`) {
		t.Errorf("unresolved [[UID]] should not produce a link:\n%s", html)
	}
	// The literal text should remain visible.
	if !strings.Contains(html, "99999999_0000") {
		t.Errorf("unresolved UID text should be preserved:\n%s", html)
	}
}

// TestWikiLinkInvalidPattern verifies that [[not-a-uid]] is not
// consumed by the wiki-link parser and passes through as literal text.
func TestWikiLinkInvalidPattern(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [[hello_world]] here.`), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `class="uid-link"`) {
		t.Errorf("[[non-UID]] should not be treated as wiki-link:\n%s", html)
	}
}

// TestWikiLinkDirQuery verifies that [[UID]] links thread the
// dirQuery suffix through.
func TestWikiLinkDirQuery(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, _, err := r.Render([]byte(`See [[20260331_9201]].`), "", "?dir=2026%2F03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md?dir=2026%2F03"`) {
		t.Errorf("[[UID]] link dropped dirQuery:\n%s", html)
	}
}

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
	input := `See [todo](note://20260331_9201), [rel](../03/20260330_9198.md), and [[20260331_9201]].`
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
		t.Errorf("[[UID]] wiki-link dropped dirQuery:\n%s", html)
	}
}
