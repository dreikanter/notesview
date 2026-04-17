package renderer

import (
	"strings"
	"testing"
)

func TestRenderFencedCodeBlockEmitsLanguageClass(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("```go\nfunc foo() {}\n```\n")
	out, err := r.Render(src, "")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, `<code class="language-go">`) {
		t.Errorf("expected output to contain <code class=\"language-go\">, got:\n%s", out)
	}
	if strings.Contains(out, "style=") {
		t.Errorf("expected output to contain no inline style attributes (chroma fingerprint), got:\n%s", out)
	}
}

func TestRenderFencedCodeBlockWithoutLanguage(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("```\nplain text\n```\n")
	out, err := r.Render(src, "")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "<pre><code>") {
		t.Errorf("expected output to contain <pre><code>, got:\n%s", out)
	}
}

// TestRenderEscapesRawHTML locks in the goldmark.WithUnsafe() absence:
// inline <script> in a markdown source must be escaped, not passed through.
func TestRenderEscapesRawHTML(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("Hello\n\n<script>alert('xss')</script>\n")
	out, err := r.Render(src, "")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("expected raw <script> to be escaped, got:\n%s", out)
	}
}
