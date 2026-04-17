# NoteIndex Unification — Design Spec

## Overview

Merge `Index` (`internal/index/index.go`) and `TagIndex` (`internal/index/tags.go`) into a single `NoteIndex` that walks the notes tree once and builds every lookup structure in one pass. Today the two indexes each run an independent `filepath.WalkDir`, and `TagIndex` re-opens every `.md` file to read frontmatter. After the change there is one walk and one per-file read, with all derived data populated on a single per-file record.

This PR also fixes a latent path-separator inconsistency between the two existing indexes (`byUID` currently returns OS-native separators on Windows; `byTag` normalizes to forward slashes).

References #64.

## Scope

- Merge the two indexes. Public behavior of what's currently consumed is preserved.
- Parse frontmatter fully (title, tags, aliases, date) on the single walk, storing a populated `NoteEntry` per file. These fields are populated but **not exposed via public getters** in this PR — exposing them is deferred to the PR that needs them.
- Expose the same lookups used today: UID → path, tag → paths, all tags.
- Rename: one type `NoteIndex` replaces `Index` and `TagIndex`; both old types, their constructors, and all compat shims are deleted in the same PR.
- No backwards-compatibility aliases (pre-1.0, internal package, all callers in-tree).

Out of scope for this PR: `bySlug`, `byAlias`, `byDate` maps and their public methods. See [Extensibility](#extensibility).

## Public API

Package `internal/index`:

```go
type NoteIndex struct { /* ... */ }

func New(root string, logger *slog.Logger) *NoteIndex
func IsUID(s string) bool

func (i *NoteIndex) Build() error
func (i *NoteIndex) Rebuild()
func (i *NoteIndex) NoteByUID(uid string) (string, bool)
func (i *NoteIndex) NotesByTag(tag string) []string
func (i *NoteIndex) Tags() []string
```

### Method contracts

- `New(root, logger)` — constructs an empty index. A nil logger is replaced with `logging.Discard()`, matching existing constructors.
- `Build()` — synchronous. Walks `root`, reads every `.md` file, builds all state, swaps it in atomically under a write lock. Returns an error only for non-permission walk failures (e.g., nonexistent root). Permission-denied directories log a warning and `SkipDir`. File read / YAML-parse errors are warned and tolerated (see [Error handling](#error-handling)).
- `Rebuild()` — asynchronous, coalescing. If a build is already in progress, returns immediately; otherwise launches `Build()` in a goroutine. On error, logs and returns. Same pattern as today.
- `NoteByUID(uid)` — returns the rel-path for a UID and a boolean found flag. Singular (UIDs are unique).
- `NotesByTag(tag)` — returns a copy of the sorted rel-path slice for a tag. Unknown tag returns a **non-nil empty slice**.
- `Tags()` — returns a copy of the sorted, deduplicated tag list.
- `IsUID(s)` — pure predicate; matches `^\d{5,}_\d+$` (see [UID format](#uid-format)).

All rel-paths returned by the index use **forward slashes** on every platform.

## Internal data model

Per-file record, held internally as an unexported `entries []NoteEntry` on `NoteIndex`:

```go
type NoteEntry struct {
    RelPath    string    // "2026/03/20260331_9201.md" (always forward slashes)
    UID        string    // "20260331_9201", or "" if filename has no UID
    Stem       string    // "20260331_9201" (filename without .md)
    Slug       string    // normalized: frontmatter slug OR derived from stem
    Title      string    // frontmatter title, "" if absent
    Tags       []string  // deduplicated within a file
    Aliases    []string  // frontmatter aliases
    Date       time.Time // resolved date
    DateSource string    // "uid" | "frontmatter" | "mtime"
}
```

The `entries` slice is the bridge to future derived maps — adding `bySlug` is one loop over `entries` in `Build` plus a lookup method; no additional file I/O.

Alongside `entries`, `NoteIndex` holds:

- `byUID  map[string]string`   — UID → RelPath
- `byTag  map[string][]string` — tag → sorted RelPaths (each tag's slice sorted for determinism)
- `allTags []string`           — sorted, deduplicated tag names

All three public maps, plus `entries`, are guarded by a single `sync.RWMutex`. A separate `sync.Mutex` (`building`) is used with `TryLock` to coalesce concurrent `Rebuild` calls — same pattern as the existing indexes.

## UID format

`IsUID` accepts `^\d{5,}_\d+$` — digits (5 or more) + `_` + digits. This relaxes the current `^\d{8}_\d+$` to allow variable-width years (date is `[Y…][MM][DD]`: last 4 digits are month+day, anything before is year). Existing 4-digit-year UIDs continue to match unchanged.

The in-build extraction regex changes analogously from `^(\d{8}_\d+)` to `^(\d{5,}_\d+)`.

## Frontmatter parsing

Promote `gopkg.in/yaml.v2` from indirect to direct in `go.mod` (it is already transitively present via `goldmark-meta`).

Extract the text between the first two `---` fences on their own lines, `yaml.Unmarshal` into:

```go
type frontmatter struct {
    Title   string    `yaml:"title"`
    Slug    string    `yaml:"slug"`
    Tags    []string  `yaml:"tags"`
    Aliases []string  `yaml:"aliases"`
    Date    time.Time `yaml:"date"`
}
```

`yaml.v2` parses both inline (`tags: [a, b]`) and block-list (`tags:\n  - a\n  - b`) forms into `[]string`, and parses `2026-04-17` into `time.Time` via `time.Time`'s `UnmarshalYAML` / `TextUnmarshaler` path. Quoted values (`"bash"`, `'go'`) are handled by the YAML parser.

If the file has no frontmatter fences, the frontmatter struct is zero-valued — the entry is still recorded.

## Build flow

One `filepath.WalkDir(root)`. For each `.md` file:

1. Compute `RelPath` via `filepath.Rel`, then `filepath.ToSlash` (always — fixes the Windows-path inconsistency).
2. Compute `Stem` (filename minus `.md`).
3. Extract `UID` with the relaxed regex; leave empty if no match.
4. Open the file and parse frontmatter. On file-read error or malformed YAML, log a warning and treat as an empty frontmatter; the entry is still created.
5. Resolve `Date` + `DateSource` (see below).
6. Derive `Slug` (see below).
7. Build `NoteEntry`, append to local slice.
8. If `UID` non-empty: `byUID[UID] = RelPath`.
9. For each unique `Tag`: append `RelPath` to `byTag[tag]`.

After the walk: sort `allTags`, sort each `byTag[t]` slice for deterministic output, then swap all state under the write lock in a single critical section.

### Date resolution

Priority (first success wins):

1. **UID date** — take the leading digit-run of `Stem` up to the first `_`. Last 2 digits are day, preceding 2 are month, everything before is year. Build `time.Date(year, month, day, 0, 0, 0, 0, time.UTC)` only if month ∈ [1,12] and day matches the month (Go's `time.Date` normalizes out-of-range values — reject those). → `DateSource = "uid"`.
2. **Frontmatter `date`** — non-zero `time.Time` from the parsed frontmatter. → `DateSource = "frontmatter"`.
3. **File mtime** — `os.Stat` on the file, take `ModTime()`. → `DateSource = "mtime"`.

If all three fail (stat error), leave `Date` zero and `DateSource = ""`. Callers check `Date.IsZero()` rather than relying on the string. Nothing consumes this in the current PR.

### Slug derivation (provisional)

- If frontmatter `slug` is present: normalize it.
- Else: take `Stem`; if it starts with `UID` followed by `_`, strip that prefix plus the `_`; normalize the remainder. If the result is empty (filename is exactly `<UID>.md`), `Slug` is empty.

Normalization rules (provisional, pinned down when a consumer lands):

- Lowercase.
- Replace each `_` and run of Unicode whitespace with `-`.
- Collapse repeated `-` to a single `-`.
- Trim leading/trailing `-`.

Nothing reads `Slug` in this PR; the rules exist only so `NoteEntry.Slug` is deterministic for tests.

Note: the existing `renderer.Frontmatter` struct also has a `Slug` field. We intentionally do **not** share a struct here — the renderer's struct serves its own per-render lifecycle and carries fields (`Description`) that the index doesn't need. Unifying them is out of scope.

## Error handling

| Condition                                   | Behavior                                                                                                    |
|---------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| Permission-denied directory                 | Log warning, `filepath.SkipDir`. Build succeeds.                                                            |
| Non-permission walk error (e.g., root DNE)  | Propagate — `Build` returns the error.                                                                      |
| File open / read error on a `.md`           | Log warning. Entry still created from filename alone (`UID`, `Stem`, `DateSource = "mtime"` w/ zero time). UID lookup preserved. |
| Frontmatter YAML unmarshal error            | Log warning. Entry created; frontmatter fields zero-valued.                                                 |
| Non-`.md` file                              | Skipped silently (same as today).                                                                           |

The key invariant: **the existing `Index.Lookup` guarantee — UID lookup works even if the file can't be opened — is preserved.**

## Concurrency

- Single `sync.RWMutex` guards `entries`, `byUID`, `byTag`, `allTags`. All read methods take `RLock`; the final swap at the end of `Build` takes the write lock once.
- Single `sync.Mutex` (`building`) with `TryLock` coalesces concurrent `Rebuild` calls — identical to today's pattern on both `Index` and `TagIndex`.
- `Rebuild` logs errors on failure and does not surface them to callers (same as today).

## SSE integration

In `internal/server/sse.go`:

- `SSEHub` holds a single `*index.NoteIndex`; the `tagIndex` field and its parameter in `NewSSEHub` are removed.
- On any `fsnotify.Write` **or** `fsnotify.Create` event, call `h.index.Rebuild()` once. (Today: `Index.Rebuild` on Create only, `TagIndex.Rebuild` on both. Unified: both triggers call the single `Rebuild`, because frontmatter can change on Write.)

## Consumer updates

All in this PR:

- `internal/server/server.go` — `Server.index *index.NoteIndex`; delete `tagIndex` field; `NewServer` builds once; pass to `NewSSEHub` and `renderer.NewRenderer`.
- `internal/server/sse.go` — see above.
- `internal/server/handlers.go` — calls route to the single index: `s.index.Tags()`, `s.index.NotesByTag(tag)`, `s.index.Rebuild()`.
- `internal/renderer/renderer.go` — `Renderer.index *index.NoteIndex`; `NewRenderer(idx *index.NoteIndex)`.
- `internal/renderer/noteext.go` — `noteLinkState.idx *index.NoteIndex`; `state.idx.NoteByUID(uid)` replaces `state.idx.Lookup(uid)`.
- `internal/renderer/noteext_test.go` — `setupTestIndex` returns `*index.NoteIndex`; uses `index.New(...)`.

## Testing

Consolidate `internal/index/index_test.go` and `internal/index/tags_test.go` into a single `internal/index/note_index_test.go`.

Preserve existing coverage (behavior-equivalent, renamed symbols):

- `TestNoteByUID` — UID → path lookup, including unknown-UID case (was `TestIndexBuild`).
- `TestBuildSkipsUnreadableDirs` — permission-denied dir is logged and skipped; readable siblings still indexed.
- `TestBuildReturnsNonPermissionError` — nonexistent root returns an error.
- `TestIsUID` — covers both 4-digit-year and (new) variable-width-year cases.
- `TestTags` and `TestNotesByTag` — inline and block-list frontmatter, unknown-tag returns non-nil empty slice, duplicate-within-file handled, quoted tags.

Add:

- `TestNoteEntryTitle` — frontmatter title populated on the entry.
- `TestNoteEntryAliases` — inline and block alias forms.
- `TestNoteEntryDateResolution` — UID date wins; frontmatter wins when UID absent; mtime wins when both absent; `DateSource` string is correct in each case.
- `TestNoteEntrySlug` — frontmatter slug normalized; derived from stem when absent.
- `TestMalformedFrontmatter` — unparseable YAML → warn log + entry still registered in `byUID`.
- `TestUnreadableFileStillIndexed` — a file made unreadable has its UID still reachable via `NoteByUID`.
- `TestVariableWidthYearUID` — a UID like `12026_0001` is recognized by `IsUID` and extracted by `Build`.
- `TestRelPathForwardSlashes` — rel-paths on both lookups use `/`, never `\` (Go-portable: construct a nested path and assert).

Renderer and server tests compile against the new symbols; no new test surface needed beyond symbol updates.

## Extensibility

The point of populating `NoteEntry` fully now is that each new derived map is a **one-liner addition** in the future:

- Add `bySlug map[string]string` → one loop over `entries` in `Build` + one `NoteBySlug` method. No walk change, no YAML change.
- Add `byAlias map[string]string` → same.
- Add `byDate map[civilDate][]string` → same, reading `NoteEntry.Date` / `DateSource`.

Adding a new frontmatter field (e.g., `author`) is a single struct-tag line on the `frontmatter` struct plus a field on `NoteEntry`.

Keeping the `frontmatter` struct private (and separate from `renderer.Frontmatter`) means extending it is a local change with no cross-package ripple.

## Not doing

- No incremental rebuild. Full walk on each `Rebuild`, same as today.
- No struct sharing with `renderer.Frontmatter`.
- No public `Entries()` or `NoteEntry` getter.
- No `bySlug` / `byAlias` / `byDate` public API.
- No change to fsnotify watcher scope or debounce behavior.
