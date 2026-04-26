package index

import (
	"errors"
	"log/slog"
	"maps"
	"path"
	"slices"
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
	RelPath     string
	Slug        string
	Title       string
	Type        string
	Description string
	Tags        []string
	Aliases     []string
	Date        time.Time
	UpdatedAt   time.Time
}

// NoteIndex is the unified in-memory index of the notes tree. It is safe
// for concurrent use. Build calls store.All() to populate from scratch;
// Apply patches a single entry in place from a watcher event.
type NoteIndex struct {
	store  note.Store
	logger *slog.Logger

	mu       sync.RWMutex
	byID     map[int]NoteEntry    // numeric ID → entry (canonical)
	byRel    map[string]NoteEntry // rel-path → entry (mirror for path-keyed lookups)
	byTag    map[string][]string  // tag → sorted rel-path slice
	bySlug   map[string][]int     // slug → ID slice (multi-valued)
	byAlias  map[string][]int     // alias → ID slice (multi-valued)
	byType   map[string][]string  // type → sorted rel-path slice
	byDate   map[string][]string  // YYYYMMDD → sorted rel-path slice
	allTags  []string
	allTypes []string
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
		byID:    make(map[int]NoteEntry),
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

	fresh := New(i.store, i.logger)
	for _, e := range entries {
		ne := projectEntry(e)
		fresh.byID[e.ID] = ne
		fresh.byRel[ne.RelPath] = ne
		fresh.addToBuckets(ne)
	}

	i.mu.Lock()
	i.byID = fresh.byID
	i.byRel = fresh.byRel
	i.byTag = fresh.byTag
	i.bySlug = fresh.bySlug
	i.byAlias = fresh.byAlias
	i.byType = fresh.byType
	i.byDate = fresh.byDate
	i.allTags = fresh.allTags
	i.allTypes = fresh.allTypes
	i.mu.Unlock()
	return nil
}

// projectEntry converts a note.Entry to the index's NoteEntry projection.
func projectEntry(e note.Entry) NoteEntry {
	return NoteEntry{
		ID:          e.ID,
		RelPath:     entryRelPath(e),
		Slug:        e.Meta.Slug,
		Title:       e.Meta.Title,
		Type:        e.Meta.Type,
		Description: e.Meta.Description,
		Tags:        dedupStrings(e.Meta.Tags),
		Aliases:     cloneStrings(e.Meta.Aliases),
		Date:        e.Meta.CreatedAt,
		UpdatedAt:   e.Meta.UpdatedAt,
	}
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

// Apply patches the index in place from a single watcher event. O(|tags|)
// per call, no full walk. Created/Updated calls store.Get; Deleted relies
// only on the cached entry.
func (i *NoteIndex) Apply(ev note.Event) {
	switch ev.Type {
	case note.EventCreated, note.EventUpdated:
		entry, err := i.store.Get(ev.ID)
		if err != nil {
			// A Created/Updated event for an entry that's already gone is
			// possible if a delete races the apply; treat it as a delete.
			if errors.Is(err, note.ErrNotFound) {
				i.applyDelete(ev.ID)
				return
			}
			i.logger.Warn("index apply: store.Get failed", "id", ev.ID, "err", err)
			return
		}
		i.mu.Lock()
		i.upsert(projectEntry(entry))
		i.mu.Unlock()
	case note.EventDeleted:
		i.applyDelete(ev.ID)
	}
}

// upsert replaces (or inserts) an entry across all buckets. Caller holds mu.
func (i *NoteIndex) upsert(ne NoteEntry) {
	if old, had := i.byID[ne.ID]; had {
		i.removeFromBuckets(old)
	}
	i.byID[ne.ID] = ne
	i.byRel[ne.RelPath] = ne
	i.addToBuckets(ne)
}

func (i *NoteIndex) applyDelete(id int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if old, ok := i.byID[id]; ok {
		i.removeFromBuckets(old)
		delete(i.byID, id)
	}
}

// Reconcile compares the index's known mtimes against the store and applies
// the resulting diff. Returns the raw note.Diff for callers that want
// counts or per-entry detail.
func (i *NoteIndex) Reconcile() (note.Diff, error) {
	// Build the known-mtime snapshot under the same write lock used for
	// applying the diff so no Apply call can squeeze in between snapshot
	// and apply (which would risk overwriting a fresher state with the
	// older diff). Reconcile is a recovery path; readers tolerate the
	// extra contention.
	i.mu.Lock()
	defer i.mu.Unlock()

	known := make(map[int]time.Time, len(i.byID))
	for id, e := range i.byID {
		known[id] = e.UpdatedAt
	}

	diff, err := i.store.Reconcile(known)
	if err != nil {
		return note.Diff{}, err
	}

	for _, e := range diff.Added {
		i.upsert(projectEntry(e))
	}
	for _, e := range diff.Updated {
		i.upsert(projectEntry(e))
	}
	for _, id := range diff.Removed {
		if old, ok := i.byID[id]; ok {
			i.removeFromBuckets(old)
			delete(i.byID, id)
		}
	}
	return diff, nil
}

// addToBuckets inserts e into every secondary bucket. Caller holds mu.
func (i *NoteIndex) addToBuckets(e NoteEntry) {
	for _, tag := range e.Tags {
		if _, exists := i.byTag[tag]; !exists {
			i.allTags = insertSorted(i.allTags, tag)
		}
		i.byTag[tag] = insertSorted(i.byTag[tag], e.RelPath)
	}
	if e.Slug != "" {
		i.bySlug[e.Slug] = appendUnique(i.bySlug[e.Slug], e.ID)
	}
	for _, alias := range e.Aliases {
		i.byAlias[alias] = appendUnique(i.byAlias[alias], e.ID)
	}
	if e.Type != "" {
		if _, exists := i.byType[e.Type]; !exists {
			i.allTypes = insertSorted(i.allTypes, e.Type)
		}
		i.byType[e.Type] = insertSorted(i.byType[e.Type], e.RelPath)
	}
	dateKey := e.Date.Format(note.DateFormat)
	i.byDate[dateKey] = insertSorted(i.byDate[dateKey], e.RelPath)
}

// removeFromBuckets erases e from every secondary bucket. Caller holds mu.
func (i *NoteIndex) removeFromBuckets(e NoteEntry) {
	for _, tag := range e.Tags {
		i.byTag[tag] = removeOne(i.byTag[tag], e.RelPath)
		if len(i.byTag[tag]) == 0 {
			delete(i.byTag, tag)
			i.allTags = removeOne(i.allTags, tag)
		}
	}
	if e.Slug != "" {
		i.bySlug[e.Slug] = removeOne(i.bySlug[e.Slug], e.ID)
		if len(i.bySlug[e.Slug]) == 0 {
			delete(i.bySlug, e.Slug)
		}
	}
	for _, alias := range e.Aliases {
		i.byAlias[alias] = removeOne(i.byAlias[alias], e.ID)
		if len(i.byAlias[alias]) == 0 {
			delete(i.byAlias, alias)
		}
	}
	if e.Type != "" {
		i.byType[e.Type] = removeOne(i.byType[e.Type], e.RelPath)
		if len(i.byType[e.Type]) == 0 {
			delete(i.byType, e.Type)
			i.allTypes = removeOne(i.allTypes, e.Type)
		}
	}
	dateKey := e.Date.Format(note.DateFormat)
	i.byDate[dateKey] = removeOne(i.byDate[dateKey], e.RelPath)
	if len(i.byDate[dateKey]) == 0 {
		delete(i.byDate, dateKey)
	}
	delete(i.byRel, e.RelPath)
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
			entry, ok := i.byID[id]
			if !ok {
				return "", false
			}
			return entry.RelPath, true
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
		entry, ok := i.byID[id]
		if !ok {
			continue
		}
		if bestRel == "" || entry.Date.After(bestDate) {
			bestRel = entry.RelPath
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
	entry, ok := i.byID[id]
	if !ok {
		return "", false
	}
	return entry.RelPath, true
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

// AllNotes returns every indexed note entry sorted newest first by date,
// then by ID and title for stable output.
func (i *NoteIndex) AllNotes() []NoteEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]NoteEntry, 0, len(i.byRel))
	for _, e := range i.byRel {
		e.Tags = cloneStrings(e.Tags)
		e.Aliases = cloneStrings(e.Aliases)
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b NoteEntry) int {
		if !a.Date.Equal(b.Date) {
			if a.Date.After(b.Date) {
				return -1
			}
			return 1
		}
		if a.ID != b.ID {
			return b.ID - a.ID
		}
		return strings.Compare(a.Title, b.Title)
	})
	return out
}

// Dates returns a sorted list of all YYYYMMDD date keys that have notes.
func (i *NoteIndex) Dates() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return slices.Sorted(maps.Keys(i.byDate))
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
	slices.Sort(out)
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

// insertSorted inserts v into a sorted slice (keeping it sorted), or
// returns s unchanged if v is already present.
func insertSorted(s []string, v string) []string {
	idx, found := slices.BinarySearch(s, v)
	if found {
		return s
	}
	return slices.Insert(s, idx, v)
}

// removeOne removes the first occurrence of v from s; returns s unchanged
// if v is not present.
func removeOne[T comparable](s []T, v T) []T {
	idx := slices.Index(s, v)
	if idx < 0 {
		return s
	}
	return slices.Delete(s, idx, idx+1)
}

// appendUnique appends v to s only if not already present.
func appendUnique[T comparable](s []T, v T) []T {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
