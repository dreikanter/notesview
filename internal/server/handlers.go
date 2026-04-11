package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// indexState extracts the index-panel state from the request query. For
// now only the "dir" mode is supported. Returning the normalized query
// string (either "" or "?index=dir") keeps downstream href construction a
// single concatenation.
//
// This function is the one seam future index modes should extend: adding
// "search" or "tag" would return a mode discriminator and normalize any
// extra params (q, t) the mode needs.
func indexState(r *http.Request) (open bool, query string) {
	mode := r.URL.Query().Get("index")
	if mode == "" {
		return false, ""
	}
	// Only "dir" is understood today; unknown values collapse to closed
	// rather than silently opening an empty panel.
	if mode != "dir" {
		return false, ""
	}
	return true, "?index=dir"
}

// toggleIndexHref returns the URL the hamburger should link to in order
// to flip the index state on the current path. Preserving r.URL.Path
// means the toggle reloads the same resource with just the query
// adjusted — htmx boosts that as a fast swap.
func toggleIndexHref(r *http.Request, open bool) string {
	path := r.URL.Path
	if open {
		return path
	}
	return path + "?index=dir"
}

// buildLayoutFields assembles the common chrome every page needs. The
// caller provides the editable path (if any) so the edit button's form
// action is pre-resolved with the index query preserved.
func (s *Server) buildLayoutFields(r *http.Request, title, editPath string) layoutFields {
	open, query := indexState(r)
	lf := layoutFields{
		Title:      title,
		EditPath:   editPath,
		IndexOpen:  open,
		IndexQuery: query,
		ShowToggle: true,
		ToggleHref: toggleIndexHref(r, open),
	}
	if editPath != "" {
		lf.EditHref = "/api/edit/" + editPath
	}
	return lf
}

// viewSSEWatch is the value for the sse-connect attribute on view.html.
// The SSE URL needs the note path percent-encoded because file names may
// contain spaces, slashes, question marks, etc.
func viewSSEWatch(filePath string) string {
	return "/events?watch=" + url.QueryEscape(filePath)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	readme := filepath.Join(s.root, "README.md")
	if _, err := os.Stat(readme); err == nil {
		http.Redirect(w, r, "/view/README.md", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/browse/", http.StatusFound)
}

func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
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

	title := filepath.Base(reqPath)
	if fm != nil && fm.Title != "" {
		title = fm.Title
	}

	lf := s.buildLayoutFields(r, title, reqPath)

	// Build the index card for the note's parent directory when the
	// panel is open. Skip the filesystem walk entirely when it's
	// closed — the card is unused and every byte counts for large
	// notes trees.
	if lf.IndexOpen {
		parentRel := filepath.Dir(reqPath)
		if parentRel == "." {
			parentRel = ""
		}
		card, err := s.buildDirIndex(parentRel, lf.IndexQuery)
		if err != nil {
			// A read failure in the parent dir shouldn't 500 the whole
			// page — log and render without the card.
			s.logger.Warn("index card read failed", "path", parentRel, "err", err)
		} else {
			lf.IndexCard = card
		}
	}

	view := ViewData{
		layoutFields: lf,
		FilePath:     reqPath,
		Frontmatter:  fm,
		HTML:         template.HTML(html),
		SSEWatch:     viewSSEWatch(reqPath),
		ViewHref:     "/view/" + reqPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderView(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	reqPath := r.PathValue("dirpath")
	absPath, err := SafePath(s.root, reqPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lf := s.buildLayoutFields(r, dirTitle(reqPath), "")
	// The browse page IS the index card — it's always open here
	// regardless of the ?index query, otherwise the page would be
	// empty. The hamburger has no meaning on browse pages and the
	// template hides it.
	lf.IndexOpen = true
	lf.ShowToggle = false

	card, err := s.buildDirIndex(reqPath, lf.IndexQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lf.IndexCard = card

	go s.index.Build()

	browse := BrowseData{
		layoutFields: lf,
		DirPath:      reqPath,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.renderBrowse(w, browse); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// buildDirIndex assembles an IndexCard in directory mode for a path
// relative to the notes root. The caller is responsible for URL-level
// access control (SafePath) — this helper only walks an already-validated
// directory.
func (s *Server) buildDirIndex(relPath, indexQuery string) (*IndexCard, error) {
	absPath, err := SafePath(s.root, relPath)
	if err != nil {
		return nil, err
	}
	entries, err := readDirEntries(absPath, relPath, indexQuery)
	if err != nil {
		return nil, err
	}
	return &IndexCard{
		Mode:        "dir",
		Breadcrumbs: buildBreadcrumbs(relPath, false, indexQuery),
		Entries:     entries,
		Empty:       "No files here.",
	}, nil
}

func dirTitle(reqPath string) string {
	if reqPath == "" {
		return ""
	}
	return reqPath
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
	if s.editor == "" {
		http.Error(w, "no editor configured (set NOTESVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}

	// Parse the editor env var the way shells treat $EDITOR: the first
	// token is the binary, the rest are leading arguments (e.g.
	// `code --wait`, `subl -w`, `nvim -R`). Without this split, exec
	// looks for a literal binary named `"code --wait"` and 500s. A
	// whitespace-only value slips past the `== ""` guard above but
	// yields zero fields, so recheck after Fields to avoid indexing a
	// nil slice and panicking the handler.
	fields := strings.Fields(s.editor)
	if len(fields) == 0 {
		http.Error(w, "no editor configured (set NOTESVIEW_EDITOR, VISUAL, or EDITOR)", http.StatusBadRequest)
		return
	}
	editorBin, editorArgs := fields[0], fields[1:]
	args := append(append([]string{}, editorArgs...), absPath)

	if err := openEditor(editorBin, args); err != nil {
		http.Error(w, fmt.Sprintf("failed to start editor: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// openEditor launches the editor for the given file. GUI editors are spawned
// directly. Terminal editors are opened in a new terminal window.
func openEditor(editorBin string, args []string) error {
	if terminalEditors[filepath.Base(editorBin)] {
		return openInTerminal(editorBin, args)
	}
	cmd := exec.Command(editorBin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
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
