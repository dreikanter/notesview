package index

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

		var info os.FileInfo
		if fi, ierr := d.Info(); ierr == nil {
			info = fi
		}
		date, source := resolveDate(uid, fm.Date, info)

		entry := NoteEntry{
			RelPath:    rel,
			UID:        uid,
			Stem:       stem,
			Slug:       deriveSlug(stem, uid, fm.Slug),
			Title:      fm.Title,
			Tags:       tags,
			Aliases:    append([]string(nil), fm.Aliases...),
			Date:       date,
			DateSource: source,
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
	if yearLen < 1 {
		return time.Time{}, false
	}
	y, err := parseIntASCII(head[:yearLen])
	if err != nil {
		return time.Time{}, false
	}
	m, err := parseIntASCII(head[yearLen : yearLen+2])
	if err != nil {
		return time.Time{}, false
	}
	d, err := parseIntASCII(head[yearLen+2:])
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

func parseIntASCII(s string) (int, error) {
	if s == "" {
		return 0, errEmptyInt
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNonDigit
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var (
	errEmptyInt = errors.New("empty int")
	errNonDigit = errors.New("non-digit in int")
)

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
