# Sidebar Tree Navigation

Replace the sidebar's hidden Files/Tags mode toggle with an always-visible tree structure where FILES and TAGS are permanent root nodes.

## Problem

The current sidebar uses two small icon-only buttons to switch between Files and Tags modes. This has two issues:

1. **Poor discoverability** -- unlabeled icons don't communicate what they do.
2. **Hidden conceptual model** -- the relationship between Files/Tags as top-level navigation axes and their children (directories, individual tags) is not apparent. The mode switch silently replaces the entire sidebar content.

## Design

### Structure

The sidebar renders a single tree with two fixed root nodes:

```
▾ FILES
  notes/
  journal/
  ideas.md
▸ TAGS
```

- **FILES** and **TAGS** are always visible as labeled root-level headings.
- Each root is collapsible via its disclosure triangle (`▾`/`▸`).
- Children appear indented beneath their root.
- Only one path is expanded at a time within each root (single-drill-down, not multi-expand).

### Drill-down Behavior

Clicking a directory drills into it, replacing the root file listing with that directory's contents:

```
▾ FILES
  ▾ notes/
    2024-03-01.md
    2024-03-02.md
▸ TAGS
```

Only one level is visible at a time -- sibling directories are not shown while drilled in.

Clicking the expanded `notes/` (or its `▾`) collapses back to the root directory listing.

Same for tags. Clicking a tag replaces the tag list with that tag's notes:

```
▾ FILES
  notes/
  journal/
  ideas.md
▾ TAGS
  ▾ golang
    go-channels.md
    error-handling.md
```

Clicking the expanded tag name collapses back to the tag list.

### Collapse/Expand

- Clicking a root heading (`FILES` / `TAGS`) toggles that entire section open/closed.
- Both sections can be open simultaneously, or one can be collapsed.
- Collapse state is persisted in localStorage.

### State Persistence (localStorage)

Replace the current keys:

| Current key | New key | Values |
|---|---|---|
| `notesview.sidebarMode` | removed | -- |
| `notesview.sidebarDir` | `notesview.filesDir` | Current expanded directory path (empty = root) |
| `notesview.sidebarTag` | `notesview.tagsTag` | Current expanded tag (empty = tag list) |
| -- | `notesview.filesOpen` | `"0"` or `"1"` (default `"1"`) |
| -- | `notesview.tagsOpen` | `"0"` or `"1"` (default `"1"`) |

### Visual Treatment

- Root headings: uppercase, small, semibold, gray-500 text with disclosure triangle. Styled as section headers, not clickable links.
- Children: indented under their root, same item styling as today (icon + name, hover highlight, click sound).
- A thin separator line between the FILES and TAGS sections.
- The current breadcrumb bar (`<header class="bg-gray-50 ...">`) is removed entirely.

### Removed

- The icon-only segmented toggle buttons.
- The `breadcrumbs.html` template (breadcrumb navigation is no longer needed).
- The `BreadcrumbsData` struct and associated builder functions (`buildDirBreadcrumbs`, `buildTagsListBreadcrumbs`, `buildTagBreadcrumbs`).

## Backend Changes

### New Endpoint Structure

The sidebar is now always rendered as a composite of two sections. Two approaches:

**Option chosen: Two independent HTMX partials.** Each section fetches its own content independently. The sidebar HTML contains two `<section>` containers, each with its own HTMX target. Clicking a directory fetches into the FILES section; clicking a tag fetches into the TAGS section.

Endpoints stay the same:
- `GET /dir/{path}` -- returns FILES section content (list of entries)
- `GET /tags` -- returns TAGS section content (list of tags)
- `GET /tags/{tag}` -- returns TAGS section content (notes for one tag)

What changes is the **response shape**: each endpoint returns just the entry list (`<ul>...</ul>` or empty state), not the full sidebar body with header. The section headings and collapse controls live in the outer sidebar template, not in the partial responses.

### Template Changes

- **Remove**: `breadcrumbs.html` template.
- **Replace**: `sidebar_body.html` with a new template containing two `<section>` elements, each with a heading toggle and an HTMX target div for its content.
- **Simplify**: `index_card.html` to render only the entry list (no header/breadcrumbs wrapper).
- **Update**: `parsePartial` to include the new sidebar template files.

### Data Model Changes

- Remove `BreadcrumbsData` and `Crumb` from `templates.go`.
- Remove `Breadcrumbs` field from `IndexCard`.
- Remove `buildDirBreadcrumbs`, `buildTagsListBreadcrumbs`, `buildTagBreadcrumbs` from `chrome.go`.
- Handlers return entry-list-only partials instead of full sidebar partials.

## Frontend Changes

### JavaScript (`app.js`)

- Remove `switchToFiles()`, `switchToTags()`, `setSidebarTag()`, `setSidebarDir()`.
- Add `toggleSection(name)` -- toggles `notesview.{name}Open` in localStorage, shows/hides the section content.
- Add `drillDir(href)` -- stores expanded dir in `notesview.filesDir`, fetches content via HTMX into the FILES section target.
- Add `collapseDir()` -- clears `notesview.filesDir`, fetches root dir listing.
- Add `drillTag(tag)` -- stores expanded tag in `notesview.tagsTag`, fetches content via HTMX into the TAGS section target.
- Add `collapseTag()` -- clears `notesview.tagsTag`, fetches tag list.
- Update `restoreSidebarState()` to restore both sections' open/collapsed state and drill-down positions independently.

### Sidebar HTML Structure

```html
<section id="files-section">
  <button onclick="toggleSection('files')" class="section-heading">
    <span class="disclosure">▾</span> FILES
  </button>
  <div id="files-content">
    <!-- HTMX target: entry list from /dir/{path} -->
  </div>
</section>

<hr class="section-divider" />

<section id="tags-section">
  <button onclick="toggleSection('tags')" class="section-heading">
    <span class="disclosure">▾</span> TAGS
  </button>
  <div id="tags-content">
    <!-- HTMX target: entry list from /tags or /tags/{tag} -->
  </div>
</section>
```

### Drill-Down Indicator

When drilled into a subdirectory or tag, show the expanded item as a clickable "back" row at the top of that section's list:

```html
<!-- Inside files-content when drilled into notes/ -->
<div class="drill-back" onclick="collapseDir()">
  <span class="disclosure">▾</span> notes/
</div>
<ul>
  <li>2024-03-01.md</li>
  <li>2024-03-02.md</li>
</ul>
```

This row uses the same `▾` triangle to indicate it's expanded and clickable to go back. At root level, this row doesn't appear.

## Tag Clicks in Note Content

Tags displayed in the note pane (from the existing clickable-tags feature) should drill the TAGS section to that tag. This replaces the current behavior of switching the sidebar mode to tags. If the TAGS section is collapsed, expand it first.
