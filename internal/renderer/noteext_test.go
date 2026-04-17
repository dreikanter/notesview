package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dreikanter/notes-view/internal/index"
)

// TestWikiLinkResolved verifies that [[UID]] syntax produces a link
// to the resolved note with uid-link class and HTMX attributes.
func TestWikiLinkResolved(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, err := r.Render([]byte(`See [[20260331_9201]] for details.`), "")
	if err != nil {
		t.Fatal(err)
	}
	a := findAnchor(t, html, "href", "/view/2026/03/20260331_9201_todo.md")
	assertAttr(t, a, "class", "uid-link")
	assertAttr(t, a, "hx-boost", "true")
	assertAttr(t, a, "hx-target", "#note-pane")
}

// TestWikiLinkUnresolved verifies that [[UID]] with a non-existent
// UID passes through as literal text (goldmark renders it as
// [[text]] since neither [ nor ] is consumed).
func TestWikiLinkUnresolved(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, err := r.Render([]byte(`See [[99999999_0000]] here.`), "")
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
	html, err := r.Render([]byte(`See [[hello_world]] here.`), "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `class="uid-link"`) {
		t.Errorf("[[non-UID]] should not be treated as wiki-link:\n%s", html)
	}
}

func setupTestIndex(t *testing.T) *index.NoteIndex {
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
	idx := index.New(dir, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return idx
}

func TestNoteProtocolLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, err := r.Render([]byte(`See [my todo](note://20260331_9201) for details.`), "")
	if err != nil {
		t.Fatal(err)
	}
	a := findAnchor(t, html, "href", "/view/2026/03/20260331_9201_todo.md")
	assertAttr(t, a, "hx-boost", "true")
	assertAttr(t, a, "hx-target", "#note-pane")
}

func TestBrokenNoteLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, err := r.Render([]byte(`See [missing](note://99999999_0000) link.`), "")
	if err != nil {
		t.Fatal(err)
	}
	a := findAnchor(t, html, "class", "broken-link")
	assertAttr(t, a, "href", "#")
	assertNoAttr(t, a, "hx-boost")
	assertNoAttr(t, a, "hx-target")
}

func TestRelativeMdLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	html, err := r.Render([]byte(`See [other](../01/foo.md) for details.`), "2026/03")
	if err != nil {
		t.Fatal(err)
	}
	a := findAnchor(t, html, "href", "/view/2026/01/foo.md")
	assertAttr(t, a, "hx-boost", "true")
	assertAttr(t, a, "hx-target", "#note-pane")
}

// TestExternalLinksStayPlain pins the "external links are plain HTML"
// rule. No hx-boost, no hx-target, no HTMX attributes of any kind on
// links whose destination points outside the notes system.
func TestExternalLinksStayPlain(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `[web](https://example.com) [mail](mailto:a@b.com) [asset](/static/foo.png)`
	html, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, href := range []string{"https://example.com", "mailto:a@b.com", "/static/foo.png"} {
		a := findAnchor(t, html, "href", href)
		assertNoAttr(t, a, "hx-boost")
		assertNoAttr(t, a, "hx-target")
	}
}

// TestDangerousURLsSanitized guards against malicious note content
// smuggling javascript:/vbscript:/file:/data: URLs into the rendered
// href. The renderer must rewrite these to "#" so clicking them is
// inert, matching the security comment in NewRenderer about cloned
// untrusted repos.
func TestDangerousURLsSanitized(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `[xss](javascript:alert(1)) [vb](vbscript:msgbox) [f](file:///etc/passwd) [d](data:text/html,<script>alert(1)</script>)`
	html, err := r.Render([]byte(input), "")
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
	html2, err := r.Render([]byte(`[ok](data:image/png;base64,iVBORw0K)`), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html2, `data:image/png`) {
		t.Errorf("data:image/png URL should be preserved, got: %s", html2)
	}
}

// TestInternalLinksNoDirQuery verifies that internal links do not carry
// any ?dir= query parameter.
func TestInternalLinksNoDirQuery(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [todo](note://20260331_9201), [rel](../03/20260330_9198.md), and [[20260331_9201]].`
	html, err := r.Render([]byte(input), "2026/03")
	if err != nil {
		t.Fatal(err)
	}
	findAnchor(t, html, "href", "/view/2026/03/20260331_9201_todo.md")
	findAnchor(t, html, "href", "/view/2026/03/20260330_9198.md")
	if strings.Contains(html, "?dir=") {
		t.Errorf("internal links should not contain ?dir= query parameter:\n%s", html)
	}
}
