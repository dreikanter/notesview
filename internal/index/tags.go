package index

import (
	"bufio"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dreikanter/notesview/internal/logging"
)

// TagIndex maps tag names to the relative paths of notes that carry them.
type TagIndex struct {
	root      string
	logger    *slog.Logger
	mu        sync.RWMutex
	tags      map[string][]string
	all       []string
	building  sync.Mutex
}

// NewTagIndex creates a TagIndex rooted at root. A nil logger is replaced with
// a discard logger, matching the pattern used by New() in index.go.
func NewTagIndex(root string, logger *slog.Logger) *TagIndex {
	if logger == nil {
		logger = logging.Discard()
	}
	return &TagIndex{
		root:   root,
		logger: logger,
		tags:   make(map[string][]string),
	}
}

// Build walks all .md files under root, parses their frontmatter, and builds
// the tag→paths map and sorted tag list. It is safe to call concurrently; the
// internal state is replaced atomically under a write lock.
func (ti *TagIndex) Build() error {
	tags := make(map[string][]string)

	err := filepath.WalkDir(ti.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				ti.logger.Warn("skipping path: permission denied", "path", path)
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		rel, err := filepath.Rel(ti.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		fileTags := parseFrontmatterTags(path)

		// Deduplicate tags within a single file so a note only appears once per tag.
		seen := make(map[string]bool, len(fileTags))
		for _, tag := range fileTags {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			tags[tag] = append(tags[tag], rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Build sorted tag list.
	all := make([]string, 0, len(tags))
	for tag := range tags {
		all = append(all, tag)
	}
	sort.Strings(all)

	// Sort each tag's path list for deterministic output.
	for tag := range tags {
		sort.Strings(tags[tag])
	}

	ti.mu.Lock()
	ti.tags = tags
	ti.all = all
	ti.mu.Unlock()
	return nil
}

// Rebuild triggers a background tag index rebuild, coalescing concurrent
// calls. Mirrors Index.Rebuild().
func (ti *TagIndex) Rebuild() {
	if !ti.building.TryLock() {
		return
	}
	go func() {
		defer ti.building.Unlock()
		if err := ti.Build(); err != nil {
			ti.logger.Error("tag index rebuild failed", "err", err)
		}
	}()
}

// Tags returns a copy of the sorted list of unique tag names.
func (ti *TagIndex) Tags() []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	out := make([]string, len(ti.all))
	copy(out, ti.all)
	return out
}

// NotesByTag returns a copy of the sorted list of relative paths for notes
// that carry the given tag. Returns an empty (non-nil) slice for unknown tags.
func (ti *TagIndex) NotesByTag(tag string) []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	paths := ti.tags[tag]
	out := make([]string, len(paths))
	copy(out, paths)
	return out
}

// parseFrontmatterTags reads the YAML frontmatter of the file at path and
// returns the list of tags declared there. It supports both the inline format
// (tags: [a, b]) and the block-list format (tags:\n  - a\n  - b).
func parseFrontmatterTags(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// The first non-empty line must be "---" to start frontmatter.
	if !scanner.Scan() {
		return nil
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return nil
	}

	var tags []string
	inTagsBlock := false

	for scanner.Scan() {
		line := scanner.Text()

		// End of frontmatter.
		if strings.TrimSpace(line) == "---" {
			break
		}

		if inTagsBlock {
			// A block list item starts with optional whitespace then "- ".
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				tag := stripYAMLQuotes(strings.TrimSpace(trimmed[2:]))
				if tag != "" {
					tags = append(tags, tag)
				}
				continue
			}
			// Any non-list line ends the block.
			inTagsBlock = false
		}

		// Look for the "tags:" key.
		if !strings.HasPrefix(line, "tags:") {
			continue
		}

		rest := strings.TrimSpace(line[len("tags:"):])

		if rest == "" {
			// Block list format: tags will be on subsequent lines.
			inTagsBlock = true
			continue
		}

		// Inline format: tags: [a, b, c] or tags: ["a", "b", 'c']
		if strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]") {
			inner := rest[1 : len(rest)-1]
			for _, part := range strings.Split(inner, ",") {
				tag := strings.TrimSpace(part)
				tag = stripYAMLQuotes(tag)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	return tags
}

// stripYAMLQuotes removes surrounding single or double quotes from a
// YAML scalar value, e.g. `"bash"` → `bash`, `'go'` → `go`.
func stripYAMLQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
