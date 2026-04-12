//go:build !windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// terminalEditors are editors that need an interactive terminal to run.
var terminalEditors = map[string]bool{
	"vim": true, "nvim": true, "vi": true, "nano": true,
	"emacs": true, "micro": true, "helix": true, "hx": true,
	"joe": true, "ne": true, "mcedit": true, "ed": true,
}

// openEditor launches the editor for the given file. GUI editors are spawned
// directly. Terminal editors are opened in a new terminal window.
func openEditor(editorBin string, args []string) error {
	if terminalEditors[filepath.Base(editorBin)] {
		return openInTerminal(editorBin, args)
	}
	cmd := exec.Command(editorBin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// openInTerminal opens a terminal editor in a new terminal window.
func openInTerminal(editorBin string, args []string) error {
	switch runtime.GOOS {
	case "darwin":
		return openInTerminalDarwin(editorBin, args)
	case "linux":
		return openInTerminalLinux(editorBin, args)
	default:
		// Fallback: try to run directly (will likely fail for TUI editors
		// but there's no portable way to open a terminal on this OS)
		cmd := exec.Command(editorBin, args...)
		return cmd.Start()
	}
}

func openInTerminalDarwin(editorBin string, args []string) error {
	// Prefer Ghostty via its bundled binary. Launching via
	// `open -na Ghostty.app --args …` is unreliable for .app bundles
	// because AppKit doesn't always forward `--args` to the inner
	// executable, so we invoke the binary directly when it's present.
	ghosttyBin := "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	if _, err := os.Stat(ghosttyBin); err == nil {
		ghosttyArgs := append([]string{"-e", editorBin}, args...)
		cmd := exec.Command(ghosttyBin, ghosttyArgs...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd.Start()
	}

	// Fall back to AppleScript: prefer iTerm2 if running, else Terminal.app.
	// Both execute a single shell command string, so we POSIX-quote every
	// argument and concatenate.
	shellCmd := shellJoin(append([]string{editorBin}, args...))
	scriptCmd := appleEscape(shellCmd)
	script := fmt.Sprintf(`
		tell application "System Events"
			set iterm_running to (name of processes) contains "iTerm2"
		end tell
		if iterm_running then
			tell application "iTerm2"
				activate
				tell current window
					create tab with default profile
					tell current session
						write text "%s"
					end tell
				end tell
			end tell
		else
			tell application "Terminal"
				activate
				do script "%s"
			end tell
		end if
	`, scriptCmd, scriptCmd)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Start()
}

func openInTerminalLinux(editorBin string, args []string) error {
	// Per-terminal invocation style. Most accept `-e cmd [args…]` with
	// separate argv tokens, but xfce4-terminal's `-e` expects a single
	// shell-string, so we POSIX-quote into one arg for it.
	editorArgv := append([]string{editorBin}, args...)
	shellCmd := shellJoin(editorArgv)
	terminals := []struct {
		cmd  string
		args []string
	}{
		{"ghostty", append([]string{"-e"}, editorArgv...)},
		{"x-terminal-emulator", append([]string{"-e"}, editorArgv...)},
		{"gnome-terminal", append([]string{"--"}, editorArgv...)},
		{"konsole", append([]string{"-e"}, editorArgv...)},
		{"xfce4-terminal", []string{"-e", shellCmd}},
		{"xterm", append([]string{"-e"}, editorArgv...)},
	}
	for _, t := range terminals {
		if path, err := exec.LookPath(t.cmd); err == nil {
			cmd := exec.Command(path, t.args...)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return cmd.Start()
		}
	}
	return fmt.Errorf("no terminal emulator found; install one or use a GUI editor")
}

// shellJoin POSIX-quotes each arg and joins with spaces, producing a string
// safe to pass to `sh -c` or a terminal that expects a single command line.
func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// via the classic `'\''` dance. This is POSIX-safe for any string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// appleEscape escapes a string for inclusion inside an AppleScript
// double-quoted string literal.
func appleEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
