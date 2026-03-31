package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/alex/notesview/internal/renderer"
	"github.com/alex/notesview/web"
)

type ViewResponse struct {
	HTML        string                `json:"html"`
	Frontmatter *renderer.Frontmatter `json:"frontmatter,omitempty"`
	Path        string                `json:"path"`
}

type BrowseEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Path  string `json:"path"`
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.serveSPA(w, r)
		return
	}
	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/browse/", http.StatusFound)
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	data, err := web.StaticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	// Content negotiation: browser gets SPA shell, fetch gets JSON
	if strings.Contains(r.Header.Get("Accept"), "text/html") &&
		!strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.serveSPA(w, r)
		return
	}

	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentDir := filepath.Dir(reqPath)
	html, fm, err := s.renderer.Render(data, currentDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ViewResponse{
		HTML:        html,
		Frontmatter: fm,
		Path:        reqPath,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") &&
		!strings.Contains(r.Header.Get("Accept"), "application/json") {
		s.serveSPA(w, r)
		return
	}

	reqPath := r.PathValue("dirpath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entries []BrowseEntry
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryPath := filepath.Join(reqPath, name)
		entries = append(entries, BrowseEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Path:  entryPath,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	go s.index.Build()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

// terminalEditors are editors that need an interactive terminal to run.
var terminalEditors = map[string]bool{
	"vim": true, "nvim": true, "vi": true, "nano": true,
	"emacs": true, "micro": true, "helix": true, "hx": true,
	"joe": true, "ne": true, "mcedit": true, "ed": true,
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("filepath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	editor := s.editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		http.Error(w, "$EDITOR is not set", http.StatusBadRequest)
		return
	}

	if err := openEditor(editor, absPath); err != nil {
		http.Error(w, fmt.Sprintf("failed to start editor: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// openEditor launches the editor for the given file. GUI editors are spawned
// directly. Terminal editors are opened in a new terminal window.
func openEditor(editor, filePath string) error {
	base := filepath.Base(editor)
	if terminalEditors[base] {
		return openInTerminal(editor, filePath)
	}
	cmd := exec.Command(editor, filePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

// openInTerminal opens a terminal editor in a new terminal window.
func openInTerminal(editor, filePath string) error {
	switch runtime.GOOS {
	case "darwin":
		return openInTerminalDarwin(editor, filePath)
	case "linux":
		return openInTerminalLinux(editor, filePath)
	default:
		// Fallback: try to run directly (will likely fail for TUI editors
		// but there's no portable way to open a terminal on this OS)
		cmd := exec.Command(editor, filePath)
		return cmd.Start()
	}
}

func openInTerminalDarwin(editor, filePath string) error {
	// Try iTerm2 first, fall back to Terminal.app
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
						write text "%s %s"
					end tell
				end tell
			end tell
		else
			tell application "Terminal"
				activate
				do script "%s %s"
			end tell
		end if
	`, editor, shellescape(filePath), editor, shellescape(filePath))
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Start()
}

func openInTerminalLinux(editor, filePath string) error {
	// Try common terminal emulators in preference order
	terminals := []struct {
		cmd  string
		args func(string, string) []string
	}{
		{"x-terminal-emulator", func(e, f string) []string { return []string{"-e", e, f} }},
		{"gnome-terminal", func(e, f string) []string { return []string{"--", e, f} }},
		{"konsole", func(e, f string) []string { return []string{"-e", e, f} }},
		{"xfce4-terminal", func(e, f string) []string { return []string{"-e", e + " " + f} }},
		{"xterm", func(e, f string) []string { return []string{"-e", e, f} }},
	}
	for _, t := range terminals {
		if path, err := exec.LookPath(t.cmd); err == nil {
			args := t.args(editor, filePath)
			cmd := exec.Command(path, args...)
			return cmd.Start()
		}
	}
	return fmt.Errorf("no terminal emulator found; install one or use a GUI editor")
}

// shellescape does minimal escaping for use inside AppleScript strings.
func shellescape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
