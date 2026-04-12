//go:build windows

package server

import "fmt"

// openEditor is not supported on Windows. The notes viewer works fine in
// the browser; only the "open in editor" feature requires Unix process
// management (Setpgid) and platform-specific terminal launchers.
func openEditor(editorBin string, args []string) error {
	return fmt.Errorf("editor launching is not supported on Windows")
}
