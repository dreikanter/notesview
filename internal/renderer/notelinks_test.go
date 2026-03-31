package renderer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex/notesview/internal/index"
)

func setupTestIndex(t *testing.T) *index.Index {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "2026", "03"), 0o755)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"), []byte("# Todo"), 0o644)
	os.WriteFile(filepath.Join(dir, "2026", "03", "20260330_9198.md"), []byte("# Note"), 0o644)
	idx := index.New(dir)
	idx.Build()
	return idx
}

func TestNoteProtocolLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [my todo](note://20260331_9201) for details.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("note:// link not resolved:\n%s", html)
	}
}

func TestBrokenNoteLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [missing](note://99999999_0000) link.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `class="broken-link"`) {
		t.Errorf("broken note:// link not marked:\n%s", html)
	}
}

func TestAutoLinkUID(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `Refer to 20260331_9201 for the todo list.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<a href="/view/2026/03/20260331_9201_todo.md"`) {
		t.Errorf("UID not auto-linked:\n%s", html)
	}
}

func TestAutoLinkUIDNoMatch(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `Reference 99999999_0000 does not exist.`
	html, _, err := r.Render([]byte(input), "")
	if err != nil {
		t.Fatal(err)
	}
	// Should not contain any link
	if strings.Contains(html, `<a href="/view/`) {
		t.Errorf("non-matching UID should not be linked:\n%s", html)
	}
}

func TestRelativeMdLink(t *testing.T) {
	idx := setupTestIndex(t)
	r := NewRenderer(idx)
	input := `See [other note](../01/20260102_8814.md) for details.`
	html, _, err := r.Render([]byte(input), "2026/03")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `href="/view/2026/01/20260102_8814.md"`) {
		t.Errorf("relative .md link not rewritten:\n%s", html)
	}
}
