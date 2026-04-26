package index

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dreikanter/notesctl/note"
)

// day returns a UTC midnight time for the given year, month, day — keeps
// test fixtures terse.
func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

// putEntry is a helper that adds an entry to a MemStore. The entry's ID must
// be non-zero; CreatedAt must be non-zero. Panics on error so callers don't
// need error-handling boilerplate.
func putEntry(s *note.MemStore, e note.Entry) {
	if _, err := s.Put(e); err != nil {
		panic(err)
	}
}

// singleEntryStore returns a MemStore containing one note with the given
// fields populated.
func singleEntryStore(id int, createdAt time.Time, slug, noteType string, tags []string) *note.MemStore {
	s := note.NewMemStore()
	putEntry(s, note.Entry{
		ID: id,
		Meta: note.Meta{
			CreatedAt: createdAt,
			Slug:      slug,
			Type:      noteType,
			Tags:      tags,
		},
	})
	return s
}

// --- Resolve ---

func TestResolveByID(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 9201, Meta: note.Meta{CreatedAt: day(2026, 3, 31), Slug: "todo"}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.Resolve("9201")
	if !ok {
		t.Fatal("Resolve(9201) = !ok, want ok")
	}
	if want := "2026/03/20260331_9201_todo.md"; got != want {
		t.Errorf("Resolve(9201) = %q, want %q", got, want)
	}
}

func TestResolveBySlug(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 9201, Meta: note.Meta{CreatedAt: day(2026, 3, 31), Slug: "todo"}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.Resolve("todo")
	if !ok {
		t.Fatal("Resolve(todo) = !ok, want ok")
	}
	if want := "2026/03/20260331_9201_todo.md"; got != want {
		t.Errorf("Resolve(todo) = %q, want %q", got, want)
	}
}

func TestResolveSlugCollisionPicksNewest(t *testing.T) {
	s := note.NewMemStore()
	// Two notes with the same slug; newer one should win.
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Slug: "weekly"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 6, 1), Slug: "weekly"}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.Resolve("weekly")
	if !ok {
		t.Fatal("Resolve(weekly) = !ok, want ok")
	}
	want := "2026/06/20260601_2_weekly.md"
	if got != want {
		t.Errorf("Resolve(weekly) = %q, want %q (newest)", got, want)
	}
}

func TestResolveAliasFallback(t *testing.T) {
	s := note.NewMemStore()
	// Note has no slug but has an alias.
	putEntry(s, note.Entry{ID: 42, Meta: note.Meta{
		CreatedAt: day(2026, 2, 15),
		Aliases:   []string{"kubernetes"},
	}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.Resolve("kubernetes")
	if !ok {
		t.Fatal("Resolve(kubernetes) = !ok, want ok")
	}
	want := "2026/02/20260215_42.md"
	if got != want {
		t.Errorf("Resolve(kubernetes) = %q, want %q", got, want)
	}
}

func TestResolveAliasCollisionPicksNewest(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 10, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Aliases: []string{"k8s"}}})
	putEntry(s, note.Entry{ID: 20, Meta: note.Meta{CreatedAt: day(2026, 9, 1), Aliases: []string{"k8s"}}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.Resolve("k8s")
	if !ok {
		t.Fatal("Resolve(k8s) = !ok, want ok")
	}
	want := "2026/09/20260901_20.md"
	if got != want {
		t.Errorf("Resolve(k8s) = %q, want %q (newest)", got, want)
	}
}

func TestResolveNotFound(t *testing.T) {
	idx := New(note.NewMemStore(), nil)
	if _, ok := idx.Resolve("nonexistent"); ok {
		t.Error("Resolve(nonexistent) = ok, want !ok")
	}
	if _, ok := idx.Resolve(""); ok {
		t.Error("Resolve('') = ok, want !ok")
	}
	if _, ok := idx.Resolve("0"); ok {
		t.Error("Resolve(0) = ok, want !ok (zero is not a valid ID)")
	}
}

func TestResolveIDZeroInvalid(t *testing.T) {
	// ID 0 is never stored; Resolve("0") must return false.
	idx := New(note.NewMemStore(), nil)
	if _, ok := idx.Resolve("0"); ok {
		t.Error("Resolve(0) = ok, want !ok")
	}
}

// --- NoteByID ---

func TestNoteByID(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 9201, Meta: note.Meta{CreatedAt: day(2026, 3, 31), Slug: "todo"}})
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.NoteByID(9201)
	if !ok {
		t.Fatal("NoteByID(9201) = !ok, want ok")
	}
	if want := "2026/03/20260331_9201_todo.md"; got != want {
		t.Errorf("NoteByID(9201) = %q, want %q", got, want)
	}
	if _, ok := idx.NoteByID(0); ok {
		t.Error("NoteByID(0) = ok, want !ok")
	}
}

// --- Build errors ---

func TestBuildReturnsErrorForBrokenStore(t *testing.T) {
	// OSStore on a nonexistent root returns an error from All().
	store := note.NewOSStore("/nonexistent-root-that-does-not-exist")
	idx := New(store, nil)
	if err := idx.Build(); err == nil {
		t.Fatal("expected Build to return an error for nonexistent root")
	}
}

func TestBuildSkipsUnreadableMonthDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission-based test not reliable when running as root")
	}

	dir := t.TempDir()
	// Readable note in 2026/03/
	mustMkdirAll(t, filepath.Join(dir, "2026", "03"))
	mustWriteFile(t, filepath.Join(dir, "2026", "03", "20260331_9201_todo.md"),
		"---\ntitle: Todo\n---\n# Todo\n")

	// Unreadable month dir 2026/04/ — OSStore silently skips it.
	mustMkdirAll(t, filepath.Join(dir, "2026", "04"))
	mustWriteFile(t, filepath.Join(dir, "2026", "04", "20260401_0001.md"), "# Secret\n")
	if err := os.Chmod(filepath.Join(dir, "2026", "04"), 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "2026", "04"), 0o755) })

	store := note.NewOSStore(dir)
	idx := New(store, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build returned unexpected error: %v", err)
	}
	if _, ok := idx.Resolve("9201"); !ok {
		t.Error("expected note 9201 to be indexed")
	}
	if _, ok := idx.NoteByID(1); ok {
		t.Error("expected note 1 to be skipped (unreadable dir)")
	}
}

// --- Tags ---

func TestTagsSorted(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Tags: []string{"golang", "web"}}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Tags: []string{"golang", "testing"}}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.Tags()
	want := []string{"golang", "testing", "web"}
	if len(got) != len(want) {
		t.Fatalf("Tags() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Tags()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNotesByTag(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Tags: []string{"golang", "web"}}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Tags: []string{"golang", "testing"}}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := idx.NotesByTag("golang"); len(got) != 2 {
		t.Errorf("NotesByTag(golang) = %v, want 2 entries", got)
	}
	if got := idx.NotesByTag("web"); len(got) != 1 {
		t.Errorf("NotesByTag(web) = %v, want 1 entry", got)
	}
	none := idx.NotesByTag("nonexistent")
	if none == nil {
		t.Error("NotesByTag(nonexistent) = nil, want non-nil empty slice")
	}
	if len(none) != 0 {
		t.Errorf("NotesByTag(nonexistent) = %v, want empty", none)
	}
}

func TestNotesByTagDeduplicatesWithinEntry(t *testing.T) {
	s := note.NewMemStore()
	// MemStore does not deduplicate tags; the index must dedup on ingestion.
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Tags: []string{"go", "go"}}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Tags: []string{"go"}}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 1, 3), Tags: []string{"go"}}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	tags := idx.Tags()
	if len(tags) != 1 || tags[0] != "go" {
		t.Errorf("Tags() = %v, want [go]", tags)
	}
	// Entry 1 contributes exactly one "go" slot after within-entry dedup;
	// entries 2 and 3 each contribute one → total 3.
	notes := idx.NotesByTag("go")
	if len(notes) != 3 {
		t.Errorf("NotesByTag(go) = %v, want 3 entries", notes)
	}
}

func TestNotesByTagSortedRelPaths(t *testing.T) {
	s := note.NewMemStore()
	// Same date, different IDs — rel-paths end up in different YYYY/MM dirs
	// because we manipulate dates to ensure lexicographic ordering.
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 2, 1), Tags: []string{"t"}}})
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Tags: []string{"t"}}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	notes := idx.NotesByTag("t")
	if len(notes) != 2 {
		t.Fatalf("NotesByTag(t) = %v, want 2 entries", notes)
	}
	// Sorted lexicographically: 2026/01/... before 2026/02/...
	if notes[0] >= notes[1] {
		t.Errorf("NotesByTag(t) not sorted: %v", notes)
	}
}

// --- Types ---

func TestTypesSorted(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Type: "journal"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Type: "task"}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 1, 3), Type: "journal"}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.Types()
	want := []string{"journal", "task"}
	if len(got) != len(want) {
		t.Fatalf("Types() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Types()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNotesByType(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Type: "journal"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Type: "task"}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 1, 3), Type: "journal"}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := idx.NotesByType("journal"); len(got) != 2 {
		t.Errorf("NotesByType(journal) = %v, want 2 entries", got)
	}
	if got := idx.NotesByType("task"); len(got) != 1 {
		t.Errorf("NotesByType(task) = %v, want 1 entry", got)
	}
	none := idx.NotesByType("nonexistent")
	if none == nil {
		t.Error("NotesByType(nonexistent) = nil, want non-nil empty slice")
	}
	if len(none) != 0 {
		t.Errorf("NotesByType(nonexistent) = %v, want empty", none)
	}
}

// --- Dates ---

func TestDatesSorted(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 3, 31)}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 1)}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 6, 15)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.Dates()
	want := []string{"20260101", "20260331", "20260615"}
	if len(got) != len(want) {
		t.Fatalf("Dates() = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Dates()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestNotesByDatePrefixYear(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1)}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 6, 15)}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2025, 12, 31)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.NotesByDatePrefix("2026")
	if len(got) != 2 {
		t.Errorf("NotesByDatePrefix(2026) = %v, want 2 entries", got)
	}
	got2025 := idx.NotesByDatePrefix("2025")
	if len(got2025) != 1 {
		t.Errorf("NotesByDatePrefix(2025) = %v, want 1 entry", got2025)
	}
}

func TestNotesByDatePrefixMonth(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 3, 1)}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 3, 31)}})
	putEntry(s, note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 4, 1)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.NotesByDatePrefix("202603")
	if len(got) != 2 {
		t.Errorf("NotesByDatePrefix(202603) = %v, want 2 entries", got)
	}
}

func TestNotesByDatePrefixDay(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 3, 31)}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 3, 30)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := idx.NotesByDatePrefix("20260331")
	if len(got) != 1 {
		t.Errorf("NotesByDatePrefix(20260331) = %v, want 1 entry", got)
	}
}

// --- NoteEntryByRel ---

func TestNoteEntryByRel(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{
		ID: 9201,
		Meta: note.Meta{
			CreatedAt: day(2026, 3, 31),
			Slug:      "nested",
			Title:     "Nested",
		},
	})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	got, ok := idx.NoteEntryByRel("2026/03/20260331_9201_nested.md")
	if !ok {
		t.Fatal("NoteEntryByRel = !ok, want ok")
	}
	if got.Title != "Nested" {
		t.Errorf("Title = %q, want Nested", got.Title)
	}
	if got.ID != 9201 {
		t.Errorf("ID = %d, want 9201", got.ID)
	}

	if _, ok := idx.NoteEntryByRel("missing.md"); ok {
		t.Error("NoteEntryByRel(missing.md) = ok, want !ok")
	}
}

func TestNoteEntryByRelReturnsDefensiveCopy(t *testing.T) {
	st := note.NewMemStore()
	putEntry(st, note.Entry{ID: 1, Meta: note.Meta{
		CreatedAt: day(2026, 1, 1),
		Tags:      []string{"a", "b"},
		Aliases:   []string{"x", "y"},
	}})
	idx := New(st, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	rel := "2026/01/20260101_1.md"
	first, ok := idx.NoteEntryByRel(rel)
	if !ok {
		t.Fatal("NoteEntryByRel = !ok, want ok")
	}
	first.Tags[0] = "MUTATED"
	first.Aliases[0] = "MUTATED"

	second, _ := idx.NoteEntryByRel(rel)
	if second.Tags[0] != "a" {
		t.Errorf("Tags[0] = %q, want %q (mutation leaked)", second.Tags[0], "a")
	}
	if second.Aliases[0] != "x" {
		t.Errorf("Aliases[0] = %q, want %q (mutation leaked)", second.Aliases[0], "x")
	}
}

// --- NoteEntry fields ---

func TestNoteEntryTitle(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Title: "Hello World"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	a, _ := idx.NoteEntryByRel("2026/01/20260101_1.md")
	if a.Title != "Hello World" {
		t.Errorf("Title = %q, want Hello World", a.Title)
	}
	b, _ := idx.NoteEntryByRel("2026/01/20260102_2.md")
	if b.Title != "" {
		t.Errorf("Title = %q, want empty", b.Title)
	}
}

func TestNoteEntryDescription(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Description: "A short summary"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	a, _ := idx.NoteEntryByRel("2026/01/20260101_1.md")
	if a.Description != "A short summary" {
		t.Errorf("Description = %q, want %q", a.Description, "A short summary")
	}
	b, _ := idx.NoteEntryByRel("2026/01/20260102_2.md")
	if b.Description != "" {
		t.Errorf("Description = %q, want empty", b.Description)
	}
}

func TestNoteEntryAliases(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{
		CreatedAt: day(2026, 1, 1),
		Aliases:   []string{"k8s", "kube"},
	}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{
		CreatedAt: day(2026, 1, 2),
		Aliases:   []string{"one", "two"},
	}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e1, _ := idx.NoteEntryByRel("2026/01/20260101_1.md")
	if len(e1.Aliases) != 2 || e1.Aliases[0] != "k8s" || e1.Aliases[1] != "kube" {
		t.Errorf("Aliases = %v", e1.Aliases)
	}
	e2, _ := idx.NoteEntryByRel("2026/01/20260102_2.md")
	if len(e2.Aliases) != 2 || e2.Aliases[0] != "one" || e2.Aliases[1] != "two" {
		t.Errorf("Aliases = %v", e2.Aliases)
	}
}

func TestNoteEntrySlug(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 3, 31), Slug: "weekly-digest"}})
	putEntry(s, note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 3, 31)}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e1, _ := idx.NoteEntryByRel("2026/03/20260331_1_weekly-digest.md")
	if e1.Slug != "weekly-digest" {
		t.Errorf("Slug = %q, want weekly-digest", e1.Slug)
	}
	e2, _ := idx.NoteEntryByRel("2026/03/20260331_2.md")
	if e2.Slug != "" {
		t.Errorf("Slug = %q, want empty", e2.Slug)
	}
}

func TestNoteEntryDate(t *testing.T) {
	createdAt := day(2026, 3, 31)
	s := singleEntryStore(9201, createdAt, "", "", nil)
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e, _ := idx.NoteEntryByRel("2026/03/20260331_9201.md")
	if !e.Date.Equal(createdAt) {
		t.Errorf("Date = %v, want %v", e.Date, createdAt)
	}
}

func TestNoteEntryID(t *testing.T) {
	s := singleEntryStore(9201, day(2026, 3, 31), "todo", "", nil)
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	e, ok := idx.NoteEntryByRel("2026/03/20260331_9201_todo.md")
	if !ok {
		t.Fatal("NoteEntryByRel = !ok")
	}
	if e.ID != 9201 {
		t.Errorf("ID = %d, want 9201", e.ID)
	}
}

// --- Apply ---

func TestApplyCreatedInsertsEntry(t *testing.T) {
	s := note.NewMemStore()
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	stored, err := s.Put(note.Entry{ID: 7, Meta: note.Meta{
		CreatedAt: day(2026, 4, 1), Slug: "fresh", Title: "Fresh", Tags: []string{"new"},
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	idx.Apply(note.Event{Type: note.EventCreated, ID: stored.ID})

	rel := "2026/04/20260401_7_fresh.md"
	entry, ok := idx.NoteEntryByRel(rel)
	if !ok {
		t.Fatal("expected entry to be indexed after Apply")
	}
	if entry.Title != "Fresh" {
		t.Errorf("Title = %q, want Fresh", entry.Title)
	}
	if got := idx.NotesByTag("new"); len(got) != 1 || got[0] != rel {
		t.Errorf("NotesByTag(new) = %v, want [%q]", got, rel)
	}
	if tags := idx.Tags(); len(tags) != 1 || tags[0] != "new" {
		t.Errorf("Tags() = %v, want [new]", tags)
	}
	gotRel, ok := idx.NoteByID(stored.ID)
	if !ok || gotRel != rel {
		t.Errorf("NoteByID(%d) = %q ok=%v, want %q", stored.ID, gotRel, ok, rel)
	}
	gotRel, ok = idx.Resolve("fresh")
	if !ok || gotRel != rel {
		t.Errorf("Resolve(fresh) = %q ok=%v, want %q", gotRel, ok, rel)
	}
}

func TestApplyUpdatedReplacesBuckets(t *testing.T) {
	s := note.NewMemStore()
	stored, err := s.Put(note.Entry{ID: 1, Meta: note.Meta{
		CreatedAt: day(2026, 1, 1), Slug: "alpha", Title: "Alpha", Type: "journal", Tags: []string{"a"},
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := idx.NotesByTag("a"); len(got) != 1 {
		t.Fatalf("baseline NotesByTag(a) = %v, want 1", got)
	}

	// Update: drop tag "a", add "b"; rename slug.
	updated := stored
	updated.Meta.Tags = []string{"b"}
	updated.Meta.Slug = "beta"
	updated.Meta.Title = "Beta"
	if _, err := s.Put(updated); err != nil {
		t.Fatalf("Put updated: %v", err)
	}
	idx.Apply(note.Event{Type: note.EventUpdated, ID: stored.ID})

	if got := idx.NotesByTag("a"); len(got) != 0 {
		t.Errorf("after update NotesByTag(a) = %v, want empty", got)
	}
	newRel := "2026/01/20260101_1_beta.journal.md"
	if got := idx.NotesByTag("b"); len(got) != 1 || got[0] != newRel {
		t.Errorf("after update NotesByTag(b) = %v, want [%q]", got, newRel)
	}
	if _, ok := idx.NoteEntryByRel("2026/01/20260101_1_alpha.journal.md"); ok {
		t.Errorf("old rel-path should no longer be indexed")
	}
	if _, ok := idx.Resolve("alpha"); ok {
		t.Errorf("Resolve(alpha) should be gone after update")
	}
	if rel, ok := idx.Resolve("beta"); !ok || rel != newRel {
		t.Errorf("Resolve(beta) = %q ok=%v, want %q", rel, ok, newRel)
	}
}

func TestApplyDeletedRemovesEntry(t *testing.T) {
	s := note.NewMemStore()
	stored, err := s.Put(note.Entry{ID: 1, Meta: note.Meta{
		CreatedAt: day(2026, 1, 1), Slug: "doomed", Tags: []string{"a", "b"}, Type: "x",
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := s.Delete(stored.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	idx.Apply(note.Event{Type: note.EventDeleted, ID: stored.ID})

	if _, ok := idx.NoteByID(stored.ID); ok {
		t.Errorf("NoteByID after delete should miss")
	}
	if got := idx.Tags(); len(got) != 0 {
		t.Errorf("Tags() after delete = %v, want empty", got)
	}
	if got := idx.Types(); len(got) != 0 {
		t.Errorf("Types() after delete = %v, want empty", got)
	}
	if _, ok := idx.Resolve("doomed"); ok {
		t.Errorf("Resolve(doomed) should miss after delete")
	}
}

func TestApplyMatchesFullRebuild(t *testing.T) {
	// After a sequence of incremental Apply calls, the index state must
	// equal a fresh Build over the same store contents.
	s := note.NewMemStore()
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("initial Build: %v", err)
	}

	mut := []struct {
		entry note.Entry
		typ   note.EventType
	}{
		{note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Slug: "a", Tags: []string{"x"}, Type: "journal"}}, note.EventCreated},
		{note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Slug: "b", Tags: []string{"x", "y"}, Type: "journal"}}, note.EventCreated},
		{note.Entry{ID: 3, Meta: note.Meta{CreatedAt: day(2026, 1, 3), Slug: "c", Tags: []string{"y"}, Type: "task"}}, note.EventCreated},
	}
	for _, m := range mut {
		if _, err := s.Put(m.entry); err != nil {
			t.Fatalf("Put: %v", err)
		}
		idx.Apply(note.Event{Type: m.typ, ID: m.entry.ID})
	}

	// Update entry 2 to drop tag "x"; delete entry 3.
	upd := note.Entry{ID: 2, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Slug: "b2", Tags: []string{"y"}, Type: "journal"}}
	if _, err := s.Put(upd); err != nil {
		t.Fatalf("Put upd: %v", err)
	}
	idx.Apply(note.Event{Type: note.EventUpdated, ID: 2})
	if err := s.Delete(3); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	idx.Apply(note.Event{Type: note.EventDeleted, ID: 3})

	// Build a second index from scratch and compare projections.
	expect := New(s, nil)
	if err := expect.Build(); err != nil {
		t.Fatalf("expect Build: %v", err)
	}

	if a, b := idx.Tags(), expect.Tags(); !equalStrings(a, b) {
		t.Errorf("Tags mismatch: got %v want %v", a, b)
	}
	if a, b := idx.Types(), expect.Types(); !equalStrings(a, b) {
		t.Errorf("Types mismatch: got %v want %v", a, b)
	}
	for _, tag := range expect.Tags() {
		if a, b := idx.NotesByTag(tag), expect.NotesByTag(tag); !equalStrings(a, b) {
			t.Errorf("NotesByTag(%s) mismatch: got %v want %v", tag, a, b)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Reconcile ---

func TestReconcileAppliesDiff(t *testing.T) {
	dir := t.TempDir()
	store := note.NewOSStore(dir)
	idx := New(store, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Add a note out-of-band (simulating a missed event).
	stored, err := store.Put(note.Entry{ID: 5, Meta: note.Meta{
		CreatedAt: day(2026, 5, 1), Slug: "drift", Tags: []string{"new"},
	}})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := idx.NoteByID(stored.ID); ok {
		t.Fatal("entry should not yet be indexed before Reconcile")
	}

	diff, err := idx.Reconcile()
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(diff.Added) != 1 || len(diff.Updated) != 0 || len(diff.Removed) != 0 {
		t.Errorf("counts = %d/%d/%d, want 1/0/0", len(diff.Added), len(diff.Updated), len(diff.Removed))
	}
	if _, ok := idx.NoteByID(stored.ID); !ok {
		t.Error("entry should be indexed after Reconcile")
	}
}

// --- helpers used by permission-based tests ---

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
}

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}
