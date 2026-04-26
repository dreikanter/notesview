package index

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

// --- NoteByUID ---

func TestNoteByUID(t *testing.T) {
	s := note.NewMemStore()
	putEntry(s, note.Entry{ID: 9201, Meta: note.Meta{CreatedAt: day(2026, 3, 31), Slug: "todo"}})
	putEntry(s, note.Entry{ID: 9198, Meta: note.Meta{CreatedAt: day(2026, 3, 30)}})
	putEntry(s, note.Entry{ID: 8814, Meta: note.Meta{CreatedAt: day(2026, 1, 2), Slug: "report"}})

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	cases := []struct {
		uid     string
		wantRel string
		wantOK  bool
	}{
		{"20260331_9201", "2026/03/20260331_9201_todo.md", true},
		{"20260330_9198", "2026/03/20260330_9198.md", true},
		{"20260102_8814", "2026/01/20260102_8814_report.md", true},
		{"99999999_0000", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.uid, func(t *testing.T) {
			got, ok := idx.NoteByUID(tt.uid)
			if ok != tt.wantOK {
				t.Errorf("NoteByUID(%q) ok = %v, want %v", tt.uid, ok, tt.wantOK)
			}
			if ok && got != tt.wantRel {
				t.Errorf("NoteByUID(%q) = %q, want %q", tt.uid, got, tt.wantRel)
			}
		})
	}
}

func TestNoteByUIDUsesForwardSlashes(t *testing.T) {
	s := singleEntryStore(9201, day(2026, 3, 31), "todo", "", nil)
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, ok := idx.NoteByUID("20260331_9201")
	if !ok {
		t.Fatal("expected UID to be found")
	}
	if strings.Contains(got, "\\") {
		t.Errorf("rel-path %q contains backslash; expected forward slashes only", got)
	}
}

func TestNoteByUIDMalformed(t *testing.T) {
	idx := New(note.NewMemStore(), nil)
	if _, ok := idx.NoteByUID("nounderscore"); ok {
		t.Error("NoteByUID(no underscore) should return false")
	}
	if _, ok := idx.NoteByUID("20260331_abc"); ok {
		t.Error("NoteByUID(non-numeric id) should return false")
	}
	if _, ok := idx.NoteByUID(""); ok {
		t.Error("NoteByUID('') should return false")
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
	if _, ok := idx.NoteByUID("20260331_9201"); !ok {
		t.Error("expected 20260331_9201 to be indexed")
	}
	if _, ok := idx.NoteByUID("20260401_0001"); ok {
		t.Error("expected 20260401_0001 to be skipped (unreadable dir)")
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

// --- IsUID ---

func TestIsUID(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"20260331_9201", true},
		{"20261231_0001", true},
		{"12026_0001", true},
		{"12345_0001", true},
		{"2026_0001", false},
		{"1234_0001", false},
		{"20260331_", false},
		{"20260331_abc", false},
		{"hello_world", false},
		{"202603319201", false},
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := IsUID(tt.s); got != tt.want {
				t.Errorf("IsUID(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// --- Rebuild ---

func TestRebuildDoneReflectsLatestState(t *testing.T) {
	s := note.NewMemStore()
	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("initial Build: %v", err)
	}

	putEntry(s, note.Entry{ID: 1, Meta: note.Meta{CreatedAt: day(2026, 1, 1), Title: "Fresh"}})

	if _, ok := idx.NoteEntryByRel("2026/01/20260101_1.md"); ok {
		t.Fatal("entry should not be indexed before Rebuild")
	}

	<-idx.Rebuild()

	entry, ok := idx.NoteEntryByRel("2026/01/20260101_1.md")
	if !ok {
		t.Fatal("entry should be indexed after Rebuild done fires")
	}
	if entry.Title != "Fresh" {
		t.Errorf("Title = %q, want Fresh", entry.Title)
	}
}

func TestRebuildCoalescesRequestsDuringInflight(t *testing.T) {
	s := note.NewMemStore()
	// Prime the store with enough entries that Build takes measurable time.
	for i := 1; i <= 200; i++ {
		putEntry(s, note.Entry{ID: i, Meta: note.Meta{CreatedAt: day(2026, 1, i%28+1), Title: "T"}})
	}

	idx := New(s, nil)
	if err := idx.Build(); err != nil {
		t.Fatalf("initial Build: %v", err)
	}

	first := idx.Rebuild()

	// Add a new entry while the first rebuild is in flight.
	lateID := 9999
	putEntry(s, note.Entry{ID: lateID, Meta: note.Meta{CreatedAt: day(2026, 6, 15), Title: "Late"}})

	second := idx.Rebuild()

	select {
	case <-second:
		select {
		case <-first:
		default:
			t.Fatal("second done closed before in-flight build finished")
		}
	case <-first:
	}

	<-second

	lateRel := "2026/06/20260615_" + strconv.Itoa(lateID) + ".md"
	if _, ok := idx.NoteEntryByRel(lateRel); !ok {
		t.Errorf("late entry %s must be indexed once second Rebuild's done fires", lateRel)
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
