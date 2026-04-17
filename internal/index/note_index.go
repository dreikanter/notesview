package index

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/logging"
)

var uidPattern = regexp.MustCompile(`^(\d{5,}_\d+)`)

var fullUIDPattern = regexp.MustCompile(`^\d{5,}_\d+$`)

// IsUID reports whether s matches the UID format: 5+ digits, an
// underscore, then 1+ digits.
func IsUID(s string) bool {
	return fullUIDPattern.MatchString(s)
}

// NoteEntry is the per-file record built during a single walk. Fields not
// needed for today's lookups are populated for future derived maps
// (bySlug, byAlias, byDate) without requiring a second walk.
type NoteEntry struct {
	RelPath     string
	UID         string
	Stem        string
	Slug        string
	Title       string
	Description string
	Tags        []string
	Aliases     []string
	Date        time.Time
	DateSource  string
}

// NoteIndex is the unified in-memory index of the notes tree. It is safe
// for concurrent use. Build performs a single filepath.WalkDir, parses
// frontmatter once per file, and swaps all state in atomically.
type NoteIndex struct {
	root    string
	logger  *slog.Logger
	mu      sync.RWMutex
	entries []NoteEntry
	byUID   map[string]string
	byRel   map[string]int
	byTag   map[string][]string
	allTags []string

	// buildMu guards curDone and queuedDone — the rebuild state machine.
	// Separate from mu so Rebuild bookkeeping does not contend with read
	// lookups. See Rebuild for the scheduling semantics.
	buildMu    sync.Mutex
	curDone    chan struct{} // in-flight build's completion signal; nil when idle
	queuedDone chan struct{} // follow-up build queued while curDone runs; nil when none
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
		byRel:  make(map[string]int),
		byTag:  make(map[string][]string),
	}
}

// Build walks the notes tree once, reads each .md file, and rebuilds all
// state. The swap at the end is atomic. Non-permission walk errors are
// propagated; permission-denied directories are warned and skipped.
func (i *NoteIndex) Build() error {
	var entries []NoteEntry
	byUID := make(map[string]string)
	byRel := make(map[string]int)
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
			return fmt.Errorf("filepath.Rel %s -> %s: %w", i.root, path, err)
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

		var info os.FileInfo
		if fi, ierr := d.Info(); ierr == nil {
			info = fi
		}
		date, source := resolveDate(uid, fm.Date, info)

		entry := NoteEntry{
			RelPath:     rel,
			UID:         uid,
			Stem:        stem,
			Slug:        deriveSlug(stem, uid, fm.Slug),
			Title:       fm.Title,
			Description: fm.Description,
			Tags:        tags,
			Aliases:     append([]string(nil), fm.Aliases...),
			Date:        date,
			DateSource:  source,
		}
		byRel[rel] = len(entries)
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
	i.byRel = byRel
	i.byTag = byTag
	i.allTags = allTags
	i.mu.Unlock()
	return nil
}

// Rebuild requests an index rebuild and returns a channel that closes
// when a build has completed that reflects the tree state at or after
// this call.
//
// Scheduling rules:
//   - Idle: start a new build immediately.
//   - Build in-flight: coalesce — queue at most one follow-up. Every
//     caller that arrives while the in-flight build runs receives the
//     same follow-up's done channel, so they only observe completion
//     after a full walk that started after their request.
//
// Waiters that only need "the current build" can read the returned
// channel; callers that do not care (e.g. warmup on a navigation) may
// ignore it.
func (i *NoteIndex) Rebuild() <-chan struct{} {
	i.buildMu.Lock()
	if i.curDone == nil {
		done := make(chan struct{})
		i.curDone = done
		i.buildMu.Unlock()
		go i.runBuild(done)
		return done
	}
	if i.queuedDone == nil {
		i.queuedDone = make(chan struct{})
	}
	done := i.queuedDone
	i.buildMu.Unlock()
	return done
}

// runBuild executes one Build and signals done; if another Rebuild
// request arrived during the build, it chains into the follow-up build
// in the same goroutine lineage.
func (i *NoteIndex) runBuild(done chan struct{}) {
	if err := i.Build(); err != nil {
		i.logger.Error("note index rebuild failed", "err", err)
	}

	i.buildMu.Lock()
	next := i.queuedDone
	i.queuedDone = nil
	if next != nil {
		i.curDone = next
	} else {
		i.curDone = nil
	}
	i.buildMu.Unlock()

	close(done)

	if next != nil {
		go i.runBuild(next)
	}
}

// NoteByUID returns the forward-slash rel-path for a UID and a boolean
// found flag. UIDs are unique.
func (i *NoteIndex) NoteByUID(uid string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	p, ok := i.byUID[uid]
	return p, ok
}

// NoteEntryByRel returns the NoteEntry whose RelPath equals rel (expected
// in forward-slash form) and a found flag. Slice fields (Tags, Aliases)
// are defensively copied so callers cannot mutate the index's internal
// storage — matching the convention used by Tags and NotesByTag.
func (i *NoteIndex) NoteEntryByRel(rel string) (NoteEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	idx, ok := i.byRel[rel]
	if !ok {
		return NoteEntry{}, false
	}
	entry := i.entries[idx]
	entry.Tags = append([]string(nil), entry.Tags...)
	entry.Aliases = append([]string(nil), entry.Aliases...)
	return entry, true
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

// deriveSlug returns the normalized slug for an entry. If the frontmatter
// supplies one, normalize it. Otherwise derive from the stem: strip the
// UID + trailing "_" prefix if present, then normalize. An empty residue
// yields an empty slug. Normalization: lowercase; runs of characters that
// are neither letters nor digits become a single "-"; trim leading and
// trailing "-".
func deriveSlug(stem, uid, frontmatterSlug string) string {
	raw := frontmatterSlug
	if raw == "" {
		residue := stem
		if uid != "" && strings.HasPrefix(residue, uid) {
			residue = strings.TrimPrefix(residue, uid)
			residue = strings.TrimPrefix(residue, "_")
		}
		raw = residue
	}
	if raw == "" {
		return ""
	}
	return normalizeSlug(raw)
}

func normalizeSlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}

// resolveDate returns (Date, DateSource) per the spec priority: UID date,
// then frontmatter date, then file mtime. stat may be nil in tests; if
// all branches fail, returns a zero Date and empty DateSource.
func resolveDate(uid string, fmDate time.Time, info os.FileInfo) (time.Time, string) {
	if d, ok := uidDate(uid); ok {
		return d, "uid"
	}
	if !fmDate.IsZero() {
		return fmDate, "frontmatter"
	}
	if info != nil {
		return info.ModTime(), "mtime"
	}
	return time.Time{}, ""
}

// uidDate parses the UID's leading digit run as [Y…][MM][DD]. Returns
// (time.Time{}, false) if the digit run is shorter than 5 or the
// resulting date is not real (e.g., month 13, Feb 30).
func uidDate(uid string) (time.Time, bool) {
	if uid == "" {
		return time.Time{}, false
	}
	underscore := strings.IndexByte(uid, '_')
	if underscore < 5 {
		return time.Time{}, false
	}
	head := uid[:underscore]
	yearLen := len(head) - 4
	y, err := strconv.Atoi(head[:yearLen])
	if err != nil {
		return time.Time{}, false
	}
	m, err := strconv.Atoi(head[yearLen : yearLen+2])
	if err != nil {
		return time.Time{}, false
	}
	d, err := strconv.Atoi(head[yearLen+2:])
	if err != nil {
		return time.Time{}, false
	}
	// Reject out-of-range dates. time.Date normalizes silently, so we
	// build then verify the fields round-trip.
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return time.Time{}, false
	}
	return t, true
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
