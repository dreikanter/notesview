package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			"completed task",
			"- [x] Done item",
			[]string{`class="task-checked"`, "Done item"},
		},
		{
			"pending task",
			"- [ ] Pending item",
			[]string{`class="task-unchecked"`, "Pending item"},
		},
		{
			"daily tag",
			"- [daily] Morning routine",
			[]string{`class="task-tag"`, "daily", "Morning routine"},
		},
		{
			"normal list item unchanged",
			"- Normal item",
			[]string{"Normal item"},
		},
	}

	r := NewRenderer(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := r.Render([]byte(tt.input), "")
			require.NoError(t, err, "Render failed")
			for _, want := range tt.contains {
				assert.Contains(t, html, want)
			}
		})
	}
}
