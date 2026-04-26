package index

import (
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notesctl/note"

	"github.com/dreikanter/nview/internal/logging"
)

// NoteEntry is the per-note record held in the index. It is a thin
// projection of note.Entry.Meta; filesystem paths are notesctl-internal.
type NoteEntry struct {
	ID          int
	Slug        string
	Title       string
	Type        string
	Description string
	Tags        []string
	Aliases     []string
	Date        time.Time
}

// NoteIndex is the unified in-memory index of the notes tree. It is safe
// for concurrent use. Build calls store.All(), projects each entry into a
// NoteEntry, and swaps all state in atomically.
type NoteIndex struct {
	store  note.Store
	logger *slog.Logger

	mu      sync.RWMutex
	byID    map[int]string      // numeric ID → forward-slash rel-path
	byRel   map[string]NoteEntry // rel-path → entry
	byTag   map[string][]string // tag → rel-path slice
	bySlug  map[string][]int    // slug → ID slice (multi-valued)
	byAlias map[string][]int    // alias → ID slice (multi-valued)
	byType  map[string][]string // type → rel-path slice
	byDate  map[string][]string // YYYYMMDD → rel-path slice
	allTags  []string
	allTypes []string

	// buildMu guards curDone and queuedDone — the rebuild state machine.
	// Separate from mu so Rebuild bookkeeping does not contend with read
	// lookups. See Rebuild for the scheduling semantics.
	buildMu    sync.Mutex
	curDone    chan struct{} // in-flight build's completion signal; nil when idle
	queuedDone chan struct{} // follow-up build queued while curDone runs; nil when none
}

// New creates a NoteIndex backed by store. A nil logger is replaced with
// a discard logger.
func New(store note.Store, logger *slog.Logger) *NoteIndex {
	if logger == nil {
		logger = logging.Discard()
	}
	return &NoteIndex{
		store:   store,
		logger:  logger,
		byID:    make(map[int]string),
		byRel:   make(map[string]NoteEntry),
		byTag:   make(map[string][]string),
		bySlug:  make(map[string][]int),
		byAlias: make(map[string][]int),
		byType:  make(map[string][]string),
		byDate:  make(map[string][]string),
	}
}

// Build fetches all entries from the store and rebuilds the index atomically.
func (i *NoteIndex) Build() error {
	entries, err := i.store.All()
	if err != nil {
		return err
	}

	byID := make(map[int]string)
	byRel := make(map[string]NoteEntry)
	byTag := make(map[string][]string)
	bySlug := make(map[string][]int)
	byAlias := make(map[string][]int)
	byType := make(map[string][]string)
	byDate := make(map[string][]string)

	for _, e := range entries {
		rel := entryRelPath(e)
		ne := NoteEntry{
			ID:          e.ID,
			Slug:        e.Meta.Slug,
			Title:       e.Meta.Title,
			Type:        e.Meta.Type,
			Description: e.Meta.Description,
			Tags:        dedupStrings(e.Meta.Tags),
			Aliases:     append([]string(nil), e.Meta.Aliases...),
			Date:        e.Meta.CreatedAt,
		}
		byID[e.ID] = rel
		byRel[rel] = ne
		for _, tag := range ne.Tags {
			byTag[tag] = append(byTag[tag], rel)
		}
		if ne.Slug != "" {
			bySlug[ne.Slug] = append(bySlug[ne.Slug], e.ID)
		}
		for _, alias := range ne.Aliases {
			byAlias[alias] = append(byAlias[alias], e.ID)
		}
		if ne.Type != "" {
			byType[ne.Type] = append(byType[ne.Type], rel)
		}
		dateKey := e.Meta.CreatedAt.Format(note.DateFormat)
		byDate[dateKey] = append(byDate[dateKey], rel)
	}

	allTags := make([]string, 0, len(byTag))
	for t := range byTag {
		allTags = append(allTags, t)
	}
	sort.Strings(allTags)
	for t := range byTag {
		sort.Strings(byTag[t])
	}

	allTypes := make([]string, 0, len(byType))
	for t := range byType {
		allTypes = append(allTypes, t)
	}
	sort.Strings(allTypes)
	for t := range byType {
		sort.Strings(byType[t])
	}

	for d := range byDate {
		sort.Strings(byDate[d])
	}

	i.mu.Lock()
	i.byID = byID
	i.byRel = byRel
	i.byTag = byTag
	i.bySlug = bySlug
	i.byAlias = byAlias
	i.byType = byType
	i.byDate = byDate
	i.allTags = allTags
	i.allTypes = allTypes
	i.mu.Unlock()
	return nil
}

// entryRelPath reconstructs the forward-slash relative path for e using the
// same YYYY/MM/YYYYMMDD_ID[_slug][.type].md layout that notesctl writes.
func entryRelPath(e note.Entry) string {
	date := e.Meta.CreatedAt.Format(note.DateFormat)
	filename := note.Filename(date, e.ID, e.Meta.Slug, e.Meta.Type)
	year := date[:len(date)-4]
	month := date[len(date)-4 : len(date)-2]
	return path.Join(year, month, filename)
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
//
// The state-machine cleanup runs in a deferred block so that even if
// Build panics, waiters on done are released and any queued follow-up
// still gets scheduled — without this, an SSE timer goroutine would
// block forever.
func (i *NoteIndex) runBuild(done chan struct{}) {
	defer func() {
		i.buildMu.Lock()
		next := i.queuedDone
		i.queuedDone = nil
		i.curDone = next
		i.buildMu.Unlock()

		close(done)

		if next != nil {
			go i.runBuild(next)
		}
	}()

	if err := i.Build(); err != nil {
		i.logger.Error("note index rebuild failed", "err", err)
	}
}

// Resolve returns the forward-slash rel-path for a slug-or-id reference:
//  1. All-digit x → integer ID → look up byID.
//  2. Otherwise look up bySlug; multiple matches pick newest by Date.
//  3. If bySlug misses → look up byAlias; multiple matches pick newest by Date.
//  4. No match → return "", false.
func (i *NoteIndex) Resolve(x string) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if isAllDigits(x) {
		id, err := strconv.Atoi(x)
		if err == nil && id > 0 {
			rel, ok := i.byID[id]
			return rel, ok
		}
	}

	if ids, ok := i.bySlug[x]; ok && len(ids) > 0 {
		return i.newestRelPath(ids), true
	}

	if ids, ok := i.byAlias[x]; ok && len(ids) > 0 {
		return i.newestRelPath(ids), true
	}

	return "", false
}

// newestRelPath returns the rel-path of the note with the latest Date
// among the given IDs. Called with mu.RLock held.
func (i *NoteIndex) newestRelPath(ids []int) string {
	var bestRel string
	var bestDate time.Time
	for _, id := range ids {
		rel, ok := i.byID[id]
		if !ok {
			continue
		}
		entry, ok := i.byRel[rel]
		if !ok {
			continue
		}
		if bestRel == "" || entry.Date.After(bestDate) {
			bestRel = rel
			bestDate = entry.Date
		}
	}
	return bestRel
}

// isAllDigits reports whether s consists entirely of ASCII decimal digits.
func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NoteByID returns the forward-slash rel-path for a numeric note ID.
func (i *NoteIndex) NoteByID(id int) (string, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	rel, ok := i.byID[id]
	return rel, ok
}

// NoteEntryByRel returns the NoteEntry whose rel-path equals rel (in
// forward-slash form) and a found flag. Slice fields (Tags, Aliases)
// are defensively copied so callers cannot mutate the index's storage.
func (i *NoteIndex) NoteEntryByRel(rel string) (NoteEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	entry, ok := i.byRel[rel]
	if !ok {
		return NoteEntry{}, false
	}
	entry.Tags = cloneStrings(entry.Tags)
	entry.Aliases = cloneStrings(entry.Aliases)
	return entry, true
}

// Tags returns a copy of the sorted, deduplicated tag list.
func (i *NoteIndex) Tags() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return cloneStrings(i.allTags)
}

// NotesByTag returns a copy of the sorted rel-path slice for a tag.
// Unknown tags return a non-nil empty slice.
func (i *NoteIndex) NotesByTag(tag string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return cloneStrings(i.byTag[tag])
}

// Types returns a copy of the sorted, deduplicated type list.
func (i *NoteIndex) Types() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return cloneStrings(i.allTypes)
}

// NotesByType returns a copy of the sorted rel-path slice for a type.
// Unknown types return a non-nil empty slice.
func (i *NoteIndex) NotesByType(typ string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return cloneStrings(i.byType[typ])
}

// Dates returns a sorted list of all YYYYMMDD date keys that have notes.
func (i *NoteIndex) Dates() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.byDate))
	for d := range i.byDate {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// NotesByDatePrefix returns sorted rel-paths for notes whose YYYYMMDD
// date key starts with prefix (e.g. "2026", "202603", "20260331").
func (i *NoteIndex) NotesByDatePrefix(prefix string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []string
	for key, rels := range i.byDate {
		if strings.HasPrefix(key, prefix) {
			out = append(out, rels...)
		}
	}
	sort.Strings(out)
	return out
}

// cloneStrings returns a fresh copy of s. A nil input yields a non-nil,
// zero-length slice so callers always get a usable value.
func cloneStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
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
