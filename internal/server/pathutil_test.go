package server

import (
	"testing"
)

func TestSafePath(t *testing.T) {
	root := "/notes"

	tests := []struct {
		name    string
		reqPath string
		want    string
		wantErr bool
	}{
		{"simple file", "2026/03/hello.md", "/notes/2026/03/hello.md", false},
		{"root dir", "", "/notes", false},
		{"dot segments rejected", "../etc/passwd", "", true},
		{"double dot in middle", "2026/../../etc/passwd", "", true},
		{"absolute path rejected", "/etc/passwd", "", true},
		{"clean path", "2026/03/../03/hello.md", "/notes/2026/03/hello.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafePath(root, tt.reqPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got %q", tt.reqPath, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tt.reqPath, err)
				return
			}
			if got != tt.want {
				t.Errorf("SafePath(%q, %q) = %q, want %q", root, tt.reqPath, got, tt.want)
			}
		})
	}
}
