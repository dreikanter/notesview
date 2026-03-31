package server

import (
	"fmt"
	"path/filepath"
	"strings"
)

func SafePath(root, reqPath string) (string, error) {
	if filepath.IsAbs(reqPath) {
		return "", fmt.Errorf("absolute path not allowed: %s", reqPath)
	}
	joined := filepath.Join(root, reqPath)
	cleaned := filepath.Clean(joined)
	if !strings.HasPrefix(cleaned, filepath.Clean(root)) {
		return "", fmt.Errorf("path traversal detected: %s", reqPath)
	}
	return cleaned, nil
}
