package index

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/logging"
)

// NoteEntry is the per-file record built during a single walk. Fields not
// needed for today's lookups are populated for future derived maps
// (bySlug, byAlias, byDate) without requiring a second walk.
type NoteEntry struct {
	RelPath    string
	UID        string
	Stem       string
	Slug       string
	Title      string
	Tags       []string
	Aliases    []string
	Date       time.Time
	DateSource string
}

// NoteIndex is the unified in-memory index of the notes tree. It is safe
// for concurrent use. Build performs a single filepath.WalkDir, parses
// frontmatter once per file, and swaps all state in atomically.
type NoteIndex struct {
	root     string
	logger   *slog.Logger
	mu       sync.RWMutex
	entries  []NoteEntry
	byUID    map[string]string
	byTag    map[string][]string
	allTags  []string
	building sync.Mutex
}

// New creates a NoteIndex rooted at root. A nil logger is replaced with
// a discard logger.
func New(root string, logger *slog.Logger) *NoteIndex {
	if logger == nil {
		logger = logging.Discard()
	}
	return &NoteIndex{
		root:   root,
		logger: logger,
		byUID:  make(map[string]string),
		byTag:  make(map[string][]string),
	}
}

// Build walks the notes tree once, reads each .md file, and rebuilds all
// state. The swap at the end is atomic. Non-permission walk errors are
// propagated; permission-denied directories are warned and skipped.
func (i *NoteIndex) Build() error {
	var entries []NoteEntry
	byUID := make(map[string]string)
	byTag := make(map[string][]string)

	err := filepath.WalkDir(i.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				i.logger.Warn("skipping path: permission denied", "path", path)
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

		rel, err := filepath.Rel(i.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		stem := strings.TrimSuffix(d.Name(), ".md")
		uid := ""
		if m := uidPattern.FindStringSubmatch(d.Name()); m != nil {
			uid = m[1]
		}

		fm, fmErr := parseFrontmatter(path)
		if fmErr != nil {
			i.logger.Warn("frontmatter parse failed", "path", rel, "err", fmErr)
			fm = frontmatter{}
		}

		tags := dedupStrings(fm.Tags)

		entry := NoteEntry{
			RelPath: rel,
			UID:     uid,
			Stem:    stem,
			Tags:    tags,
		}
		entries = append(entries, entry)

		if uid != "" {
			byUID[uid] = rel
		}
		for _, t := range tags {
			byTag[t] = append(byTag[t], rel)
		}
		return nil
	})
	if err != nil {
		return err
	}

	allTags := make([]string, 0, len(byTag))
	for t := range byTag {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags)
	for t := range byTag {
		sort.Strings(byTag[t])
	}

	i.mu.Lock()
	i.entries = entries
	i.byUID = byUID
	i.byTag = byTag
	i.allTags = allTags
	i.mu.Unlock()
	return nil
}

// Rebuild triggers a background index build, coalescing concurrent calls.
// If a build is already in progress, the call returns immediately.
func (i *NoteIndex) Rebuild() {
	if !i.building.TryLock() {
		return
	}
	go func() {
		defer i.building.Unlock()
		if err := i.Build(); err != nil {
			i.logger.Error("note index rebuild failed", "err", err)
		}
	}()
}

// NoteByUID returns the forward-slash rel-path for a UID and a boolean
// found flag. UIDs are unique.
func (i *NoteIndex) NoteByUID(uid string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	p, ok := i.byUID[uid]
	return p, ok
}

// Tags returns a copy of the sorted, deduplicated tag list.
func (i *NoteIndex) Tags() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, len(i.allTags))
	copy(out, i.allTags)
	return out
}

// NotesByTag returns a copy of the sorted rel-path slice for a tag.
// Unknown tags return a non-nil empty slice.
func (i *NoteIndex) NotesByTag(tag string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	paths := i.byTag[tag]
	out := make([]string, len(paths))
	copy(out, paths)
	return out
}

// dedupStrings returns s with duplicates removed, preserving first-seen
// order. A nil or empty input returns nil.
func dedupStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
