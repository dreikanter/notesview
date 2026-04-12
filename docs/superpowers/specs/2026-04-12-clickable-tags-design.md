# Clickable Tags — Design Spec

## Overview

Make tags in the note frontmatter bar clickable to filter notes in the sidebar. Introduce a two-mode sidebar (Files / Tags) with a toggle in the breadcrumb bar.

## Sidebar Modes

Three sidebar states, two entry points:

| State | Content | Entry point |
|-------|---------|-------------|
| **Files** (default) | Directory tree, same as today | Click "Files" toggle |
| **Tags list** | All tags, alphabetical column | Click "Tags" toggle |
| **Tag filter** | Flat list of note filenames matching a tag | Click a tag (from tags list or note body pill) |

### State Transitions

| From | Action | To |
|------|--------|----|
| Files | Click "Tags" button | Tags list |
| Tags list | Click "Files" button | Files (current note's parent dir, or root if no note open) |
| Tags list | Click a tag | Tag filter |
| Tag filter | Click "Files" button | Files (current note's parent dir, or root if no note open) |
| Tag filter | Click "Tags" button | Tags list |
| Any | Click tag pill in note body | Tag filter for that tag |
| Tag filter | Click tag pill in note body | Tag filter for new tag |
| Any | Click already-active mode button | Reload current mode's content |

### Filtered Notes List

- Flat list of filenames (no directory grouping)
- Same list item presentation as file entries in the directory tree
- Sidebar stays in tag filter mode when navigating between notes

## Tag Index

New `TagIndex` in `internal/index`, built alongside the UID index.

- Walks all `.md` files, parses YAML frontmatter only (not full markdown body)
- Stores `tagToFiles map[string][]string` (tag → sorted relative paths) and `allTags []string` (sorted, deduplicated)
- Exposes `Tags() []string` and `NotesByTag(tag string) []string`
- Thread-safe with `sync.RWMutex`
- Full rebuild on every trigger (same as UID index) — incremental optimization deferred to #52
- Rebuilt on startup and on fsnotify file changes via existing `Rebuild()` mechanism

## Routing

New routes:

- `GET /tags` — all tags list
- `GET /tags/{tag}` — notes for a tag (`{tag}` is percent-encoded)

Both follow the existing HTMX partial pattern:
- `HX-Target: sidebar` → sidebar partial only
- Full browser navigation → full two-pane layout with sidebar pre-populated

Existing routes unchanged. The `?dir=` query param and all sticky-directory logic are removed.

## Breadcrumb Bar

The mode toggle replaces the current "Root" link as the root breadcrumb element. It's a segmented icon button group `[Files | Tags]`, always present, always clickable.

Breadcrumb states:

| Sidebar state | Breadcrumb trail |
|---------------|-----------------|
| Files root | `[Files\|Tags]` |
| Files / subdir | `[Files\|Tags] / 2026 / 04` |
| Tags list | `[Files\|Tags]` |
| Tag filter | `[Files\|Tags] / golang` |

The `BreadcrumbsData` struct gains a `Mode` field for the template to set active button state.

## Removing `?dir=` (Sticky Directory)

The `?dir=` query parameter, `parseDirParam()`, `dirQuery()`, `dirLinkHref()`, and related threading through handlers, types, and templates are all removed. This is a net deletion of code.

Sidebar directory state moves to localStorage. The SSE live-reload `hx-get` on the note pane simplifies to just `/view/{filepath}` targeting `#note-pane` — the sidebar is not touched by note reloads.

## Template Changes

**`breadcrumbs.html`** — Rewritten to include the mode toggle as root element. Reads `.Mode` from `BreadcrumbsData` for active button state.

**`index_card.html`** — Extended to handle three modes:
- `Mode: "dir"` — directory tree (existing)
- `Mode: "tags"` — all-tags list, each entry links to `/tags/{tag}` with `hx-target="#sidebar"`
- `Mode: "tag"` — filtered notes list, each entry links to `/view/{filepath}` with `hx-target="#note-pane"`

**`note_pane_body.html`** — Tag pills become `<a>` tags with `href="/tags/{tag}" hx-target="#sidebar" hx-swap="innerHTML"` plus JS to save mode to localStorage.

**`sidebar_body.html`** — No structural change.

No new template files — all modes fit within `index_card` by switching on `.Mode`.

## Client-Side JavaScript

Plain JS (Alpine.js migration deferred to #54). Light footprint:

**localStorage keys:**
- `notesview.sidebarMode`: `"files"`, `"tags"`, or `"tag"`
- `notesview.sidebarTag`: active tag name (when mode is `"tag"`)
- `notesview.sidebarDir`: active directory (when mode is `"files"`)

**Page load:**
1. Server renders files-mode sidebar (default)
2. `DOMContentLoaded` reads localStorage
3. If mode is not files, fires HTMX request to the appropriate endpoint to swap sidebar

**Mode switch / tag click handlers:**
Update localStorage, let HTMX `hx-get` handle the swap.

## Testing

**Go unit tests:**
- Tag index: `Build()` with fixtures covering tags present, missing, empty, duplicates. Verify `Tags()` and `NotesByTag()`.
- Handlers: `handleTags` and `handleTagNotes` — full-page vs HTMX partial, valid tag vs unknown tag.
- Breadcrumbs: updated tests for mode-aware output.
- Chrome helpers: removed `?dir=` tests; new tag URL helpers.

**Manual testing:**
- Tag pill click → sidebar shows filtered notes
- Files/Tags toggle → mode switches correctly
- Page reload → localStorage restores sidebar state
- Navigate between notes in tag mode → sidebar persists
- Click active mode button → content reloads

## References

- closes #27
- relates to #52 (incremental tag index rebuild)
- relates to #54 (Alpine.js adoption)
