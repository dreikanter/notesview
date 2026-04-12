package renderer

import (
	"strings"
	"testing"
)

func TestTaskSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			"completed task",
			"- [+] Done item",
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
			html, _, err := r.Render([]byte(tt.input), "", "")
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(html, want) {
					t.Errorf("output missing %q\ngot: %s", want, html)
				}
			}
		})
	}
}
