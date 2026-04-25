package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFencedCodeBlockEmitsLanguageClass(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("```go\nfunc foo() {}\n```\n")
	out, err := r.Render(src, "")
	require.NoError(t, err, "Render")
	assert.Contains(t, out, `<code class="language-go">`)
	assert.NotContains(t, out, "style=", "expected output to contain no inline style attributes (chroma fingerprint)")
}

func TestRenderFencedCodeBlockWithoutLanguage(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("```\nplain text\n```\n")
	out, err := r.Render(src, "")
	require.NoError(t, err, "Render")
	assert.Contains(t, out, "<pre><code>")
}

// TestRenderEscapesRawHTML locks in the goldmark.WithUnsafe() absence:
// inline <script> in a markdown source must be escaped, not passed through.
func TestRenderEscapesRawHTML(t *testing.T) {
	r := NewRenderer(nil)
	src := []byte("Hello\n\n<script>alert('xss')</script>\n")
	out, err := r.Render(src, "")
	require.NoError(t, err, "Render")
	assert.NotContains(t, out, "<script>", "expected raw <script> to be escaped")
}
