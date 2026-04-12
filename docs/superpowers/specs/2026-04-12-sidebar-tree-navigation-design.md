# Sidebar Tree Navigation

Replace the sidebar's hidden Files/Tags mode toggle with a GitHub-repo-browser-style tree where FILES and TAGS are permanent root nodes, and the main panel serves as both directory listing and note viewer.

## Problem

The current sidebar uses two small icon-only buttons to switch between Files and Tags modes. This has two issues:

1. **Poor discoverability** -- unlabeled icons don't communicate what they do.
2. **Hidden conceptual model** -- the relationship between Files/Tags as top-level navigation axes and their children (directories, individual tags) is not apparent. The mode switch silently replaces the entire sidebar content.

## Design

### Navigation Model (GitHub-style)

The sidebar is a persistent navigation tree. The main panel displays either a **directory/tag listing** or a **note**, depending on what's selected. This mirrors GitHub's repository browser: the sidebar tree stays visible and highlights the current selection, while the main area shows content.

### Sidebar Structure

Two fixed root nodes, always visible. The sidebar shows a **real tree with visible ancestry** -- when drilled into a nested directory, all ancestor directories remain visible and indented:

```
▾ FILES
  ▾ 2026/
    ▾ 04/
      2024-04-01.md     ← highlighted (currently open)
      2024-04-02.md
  journal/
  ideas.md
▾ TAGS
  ▾ golang
    go-channels.md
    error-handling.md
  architecture
  til
```

- **FILES** and **TAGS** are labeled section headings that act as collapse toggles.
- Both can be open simultaneously.
- Directories and tags behave identically: clicking expands them in the sidebar and shows their content listing in the main panel.
- Only one path can be expanded at a time within FILES. Expanding a directory at the same level collapses the previously expanded one. Same for tags within TAGS.
- Notes are listed under their expanded parent directory or tag.
- Each nesting level is visually indented (left padding increases with depth).
- The full ancestor chain from root to the current directory is always visible, providing context for where you are in the hierarchy.

### Interaction Flow

**Clicking a directory in the sidebar:**
1. Expands the directory in the sidebar (shows its children -- subdirs and notes).
2. Shows a listing of that directory's contents in the main panel.
3. Highlights the directory as selected in the sidebar.

**Clicking a tag in the sidebar:**
1. Expands the tag in the sidebar (shows its notes).
2. Shows a listing of that tag's notes in the main panel.
3. Highlights the tag as selected in the sidebar.

**Clicking a note (from sidebar OR from main panel listing):**
1. Opens the note in the main panel.
2. Highlights the note in the sidebar.
3. The note's parent directory (or tag) stays expanded so the note is visible among siblings.

**Clicking a note from the main panel listing** behaves identically to clicking it in the sidebar.

### Main Panel: Dual Purpose

The main panel (`#note-pane`) displays one of two things:

1. **Directory/tag listing** -- a list of linked entries (files and subdirectories), visually identical to how they appear in the sidebar. Same icons, same text styling, same hover states. It should be visually clear that both lists perform the same function.

2. **Note view** -- the rendered markdown note (same as today).

### Selected State

The currently-selected item in the sidebar gets a persistent highlight (distinct from hover). This applies to:
- A directory, when its listing is shown in the main panel.
- A tag, when its listing is shown in the main panel.
- A note, when it's open in the main panel.

Only one item is selected at a time.

### Collapse/Expand Behavior

- Clicking a root heading (FILES/TAGS) collapses/expands that entire section.
- Clicking an expanded directory/tag name collapses it (hides children, returns to parent listing in main panel).
- Drilling into a subdirectory from the main panel listing expands that subdirectory in the sidebar and shows its contents in both places.

### State Persistence (localStorage)

| Key | Values | Default |
|---|---|---|
| `notesview.filesOpen` | `"0"` / `"1"` | `"1"` |
| `notesview.tagsOpen` | `"0"` / `"1"` | `"1"` |
| `notesview.filesDir` | Expanded directory path (empty = root level) | `""` |
| `notesview.tagsTag` | Expanded tag name (empty = tag list level) | `""` |
| `notesview.selected` | Path of selected item (dir path, tag name, or note path) | `""` |

Remove the current `notesview.sidebarMode`, `notesview.sidebarDir`, `notesview.sidebarTag` keys.

### Visual Treatment

- **Section headings** (FILES, TAGS): uppercase, small, semibold, gray text with disclosure triangle. Not links.
- **Entries** (dirs, tags, notes): same styling in sidebar and main panel -- icon + name, same font size, same hover highlight, same click sound.
- **Selected state**: distinct background color (e.g. `bg-blue-50 border-blue-200`) that differs from hover (`hover:bg-blue-100`).
- **Indentation**: children are indented one level under their expanded parent.
- **Separator**: thin line between FILES and TAGS sections.
- The current breadcrumb bar is removed entirely.

## Removed

- The icon-only segmented toggle buttons.
- The `breadcrumbs.html` template.
- The `BreadcrumbsData` struct and breadcrumb builder functions.
- The concept of sidebar "modes" -- the sidebar always shows the full tree.

## Backend Changes

### New Endpoint: Directory/Tag Listing for Main Panel

Add a new response shape for directory and tag content rendered into the main panel. The existing `/dir/{path}`, `/tags`, `/tags/{tag}` endpoints are reused but need to support two targets:

- **`HX-Target: sidebar`** -- returns the sidebar section content (entry list for the tree).
- **`HX-Target: note-pane`** -- returns the same entries rendered as a main-panel listing page (same visual style, wrapped in the main panel container).

Alternatively, the main panel listing can reuse the same entry list partial, wrapped differently. The key requirement: **the entry markup is identical** between sidebar and main panel so they look and behave the same.

### Template Changes

- **Remove**: `breadcrumbs.html`.
- **New**: `sidebar_tree.html` -- the two-section sidebar with FILES/TAGS roots, collapse toggles, and HTMX targets for each section's content.
- **New**: `dir_listing.html` -- main panel template for directory/tag listings. Renders the same entry items as the sidebar but in the main content area.
- **Refactor**: `index_card.html` -- extract the entry list (`<ul>` of entries) into a shared partial used by both sidebar sections and the main panel listing. Remove the header/breadcrumbs wrapper.
- **Update**: `sidebar_body.html` -- render the new two-section tree instead of the single IndexCard.
- **Update**: `parsePartial` -- include new template files.

### Data Model Changes

- Remove `BreadcrumbsData`, `Crumb` from `templates.go`.
- Remove `Breadcrumbs` field from `IndexCard`.
- Remove breadcrumb builder functions from `chrome.go`.
- Add a `DirListingData` (or similar) for main-panel directory/tag listings.

## Frontend Changes

### JavaScript (`app.js`)

Remove:
- `switchToFiles()`, `switchToTags()`, `setSidebarTag()`, `setSidebarDir()`
- `getSidebarMode()`, `getSidebarTag()`, `getSidebarDir()`
- `refreshSidebar()` (current implementation)

Add:
- `toggleSection(name)` -- collapse/expand a root section, persist to localStorage.
- `selectDir(href)` -- expand directory in sidebar, load listing in main panel, update selected state.
- `selectTag(tag)` -- expand tag in sidebar, load listing in main panel, update selected state.
- `selectNote(href)` -- load note in main panel, highlight in sidebar, keep parent expanded.
- `collapseDir()` / `collapseTag()` -- collapse back, show parent listing.
- `restoreSidebarState()` -- restore section open/closed, expanded dir/tag, and selected highlight from localStorage.

### Tag Clicks in Note Content

Tags in the note pane (clickable tag pills) call `selectTag(tag)` instead of the current `setSidebarTag()`. This expands the TAGS section if collapsed, drills into that tag, and shows its note listing in the main panel.

### URL Routing

Navigation must update the browser URL so that reload returns to the current view and back/forward buttons work.

- `selectNote(href)` pushes `/view/{path}` to browser history.
- `selectDir(href)` pushes `/dir/{path}` to browser history.
- `selectTag(tag)` pushes `/tags/{tag}` to browser history.
- `popstate` handler parses the URL and calls the appropriate function.
- The server's `/dir/{path}` and `/tags/{tag}` handlers serve full-page HTML for non-HTMX requests (direct URL visits / reload).

### Sidebar Tree Data Model

The sidebar `/dir/{path}` endpoint returns a **tree with ancestor chain**, not a flat list. For a request to `/dir/2026/04`, the response includes:

```
depth=0: 2026/ (expanded)
depth=1:   04/ (expanded)
depth=2:     file1.md
depth=2:     file2.md
depth=0: journal/
depth=0: README.md
```

Each entry carries a `Depth` (indentation level) and `Expanded` flag. The template renders indentation based on depth. Expanded entries show a `▾` disclosure triangle; collapsed directories show `▸`.

For tags, when a tag is selected the response includes all tags at depth 0 with the selected tag expanded and its notes at depth 1.

### Initial Page Load

On first load (or when navigating to `/`):
- Both sidebar sections render expanded.
- If a README.md exists, it opens in the main panel (existing behavior).
- The sidebar highlights the corresponding note.
- Restore persisted state from localStorage.
