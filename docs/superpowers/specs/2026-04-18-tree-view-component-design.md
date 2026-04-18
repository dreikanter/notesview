# Client-side Tree View Component — Design Spec

## Overview

Extract the sidebar tree into a reusable, data-driven client-side component. Move tree UI state off the server and out of localStorage / DOM attributes / global JS flags into a single JS module with a clean public API (methods + events). Unify the two intended SSE streams (per-note content change + directory mutation) onto one endpoint.

Rationale and architectural context: issue #80 (comment). This spec implements the "TreeView component + unified event stream" direction proposed there.

Tracking issue: #88.

## Goals

- **Pure JS module**, no new framework dependency. The discipline of a Web Component (methods + events as the public API, no reaching into internals) without being one.
- **Decoupled from notes-view.** The component knows nothing about `/view/`, `/dir/`, `/tags/`, filesystems, or notes. It is given a loader function and emits events. The app glues events to navigation.
- **Reusable.** Parameterized data source, no hard-coded URLs, no assumptions about node shape beyond `{name, isDir, path}` (extensible via render hooks later).
- **Survives page reload.** Expanded state and selection persist to `localStorage` under a caller-supplied key; the current URL also contributes to first-paint expansion.
- **Keyboard-navigable.** Follows the W3C WAI-ARIA Authoring Practices tree pattern (single-select subset).
- **Styled with Tailwind utilities only.** No custom CSS module for the component; structural `tv-*` classes exist as selectors/hooks, not styling rules.
- **One SSE connection per page.** The existing `/events?watch=X` endpoint is generalized to emit both `change` (per-note content) and `dir-changed` (tree mutation) events.

## Non-goals (v1)

- Virtualization. Fine for thousands of nodes; defer until needed.
- Drag-and-drop, multi-select.
- `*` "expand all siblings at level" hotkey from APG.
- Server-side expansion state.
- No-JS degradation. Component requires JS; conscious tradeoff — the current SSR tree is replaced.
- Backward-compat shims for old client behavior. No external consumers, no shim.

## Why now

The retrospective in #80 identified entangled state: URL, `filesDir` in localStorage, `data-expanded` / `data-depth` DOM attributes, `pendingScrollToSelected` / `pendingNoteScrollReset` global flags, and server-rendered tree HTML. Tag clicks, dir clicks, chevron clicks, popstate, and SSE each had a different recipe for "what should the sidebar look like now?" The right mental model is a clean split:

- **Filesystem shape** is server-owned. Pure data.
- **Tree view state** is client-owned. Pure UI.
- **App glue** translates component events into navigation.

This spec operationalizes that split.

## Architecture

```
Server                    TreeView component          App glue
──────                    ──────────────────          ────────
GET /api/tree/list   ───► model: nodesByPath     ◄─── navigation
  ?path=X                 childrenByPath          ───► URL push
→ JSON children           expandedPaths           ───► main pane
                          selectedPath                  SSE consumer
GET /events               focusedPath
  ?watch=X (opt)          loadingPaths
  → change events         methods:
  → dir-changed            expand/collapse/toggle
                           select/scrollTo/refresh
                           setRoot/destroy

                          events on container:
                           tree:select
                           tree:toggle
                           tree:error
```

Three layers, each with one job. No layer reaches into another's internals.

## Component API

### Construction

```js
const tree = new TreeView(container, {
  loader,                 // required: (path) => Promise<Array<Node>>
  initial,                // optional: { selectedPath? } — metadata only, no entries
  persistKey,             // optional: localStorage key for expanded + selected
  rootPath = '',          // optional: path to pass to loader for the root level
  renderLabel,            // optional: (node) => string | Node  (v1 default = plain name)
  renderIcon,             // optional: (node) => string | Node  (v1 default = folder/file SVG)
  classPrefix = 'tv-',    // optional: structural CSS class prefix
});
```

On construction, the component **wipes `container.innerHTML`** and injects its own DOM. The container is expected to be an empty placeholder.

### Node shape

```ts
type Node = {
  name: string;       // display label (unless renderLabel provided)
  path: string;       // opaque, globally-unique identifier
  isDir: boolean;
  // arbitrary extra fields allowed; passed through to render hooks and events
};
```

Component never mutates received nodes. Paths are opaque; equality is `===`. The loader MUST return globally-unique path strings (full relative paths, not leaf names).

### Methods

| Method | Behavior |
|---|---|
| `expand(path)` | Idempotent. Loads children if not loaded; inserts rows; updates `aria-expanded`. Returns `Promise<void>`. If a load is in flight for `path`, returns the in-flight promise. |
| `collapse(path)` | Idempotent. Removes subtree rows from DOM; keeps children in the model so re-expand is instant if unchanged. |
| `toggle(path)` | `expand` if collapsed, `collapse` if expanded. |
| `select(path, {source='api'})` | Sets selection. Updates `aria-selected`, visual highlight, focus. Emits `tree:select` unless `source === 'silent'`. |
| `scrollTo(path, {block='center'})` | Scrolls the node into view. No-op if already fully visible. Never called automatically. |
| `refresh(path)` | Re-fetches children for `path` (if expanded or root). Reconciles DOM. Preserves descendants that still exist. If a load is in flight for `path`, flags a follow-up refresh instead of firing a second loader. |
| `setRoot(path)` | Replaces the root path (rare). Clears state. |
| `destroy()` | Removes DOM, listeners. LocalStorage cleanup is opt-in. |

### Events (dispatched on `container`)

| Name | `event.detail` |
|---|---|
| `tree:select` | `{ path, node, source: 'click' \| 'keyboard' \| 'api' }` |
| `tree:toggle` | `{ path, expanded }` |
| `tree:error` | `{ path, error }` — loader rejection |

No `tree:activate` and no double-click. Single click on a row body selects; the app decides what "selected" means. Chevron click toggles expansion without selecting.

### Internal state

```js
class TreeView {
  nodesByPath: Map<string, Node>
  childrenByPath: Map<string, string[]>
  expandedPaths: Set<string>
  selectedPath: string | null
  focusedPath: string | null
  loadingPaths: Map<string, { promise: Promise<void>, pendingRefresh: boolean }>
}
```

DOM is a pure projection of this state. `path=''` is the implicit root (not drawn; its children are the top level).

## DOM structure

```html
<div class="tv-root" role="tree">
  <ul class="tv-group" role="group">
    <li class="tv-item tv-item--dir" role="treeitem"
        data-path="a" aria-expanded="true" aria-level="1"
        aria-selected="false" tabindex="-1"
        style="--tv-depth: 0">
      <div class="tv-row">
        <button class="tv-toggle" tabindex="-1" aria-hidden="true">▾</button>
        <span class="tv-icon">…</span>
        <span class="tv-label">a</span>
      </div>
      <ul class="tv-group" role="group">
        <li class="tv-item tv-item--file" role="treeitem"
            data-path="a/readme.md" aria-level="2"
            aria-selected="true" tabindex="0"
            style="--tv-depth: 1">
          <div class="tv-row">
            <span class="tv-toggle-spacer"></span>
            <span class="tv-icon">…</span>
            <span class="tv-label">readme.md</span>
          </div>
        </li>
      </ul>
    </li>
  </ul>
</div>
```

- **Tailwind utilities for all styling.** Visual classes are attached at render time (inline Tailwind strings on the row, label, icon, toggle). The `tv-*` classes exist for ARIA/structural hooks (tests, selectors) and are NOT paired with `.tv-*` CSS rules.
- `role="treeitem"` sits on the `<li>`; the toggle `<button>` is `aria-hidden` (keyboard interaction is on the `<li>`; the button is a mouse affordance only).
- Roving tabindex: exactly one node has `tabindex=0` at a time; all others `-1`.
- `aria-level` reflects depth; `--tv-depth` CSS custom property drives indentation via Tailwind arbitrary-value utilities (e.g., `pl-[calc(var(--tv-depth)*1rem)]`). No `data-depth` attribute.
- `data-path` is the identity anchor. DOM selectors use `CSS.escape(path)` so paths with quotes/brackets work.

## Reconciliation

When children for `parentPath` arrive (from loader or refresh):

1. Compute `prev = childrenByPath.get(parentPath) ?? []`.
2. `next = result.map(n => n.path)`.
3. For each path in `prev \ next`: remove its `<li>` subtree. Delete entries under it from `nodesByPath`, `childrenByPath`, `expandedPaths`.
4. For each path in `next`: if it already has a row, move it to the correct position. Otherwise create a row.
5. Update `nodesByPath` / `childrenByPath`.
6. Paths that survive and were expanded: their DOM subtrees are preserved intact.
7. If selection was inside a removed subtree: clear `selectedPath` and emit `tree:select { path: null }`.
8. If focus was inside a removed subtree: move focus to the parent `<li>`.
9. Node type flip (`isDir` changed): replace the row in place; drop expansion state under the old path.

This makes `refresh(path)` safe for SSE: only the changed dir's children are touched; every expanded descendant below unrelated dirs keeps its expansion and selection.

### Refresh during load

If `refresh(x)` is called while `x` is loading:

- Set `loadingPaths[x].pendingRefresh = true`.
- Return the in-flight promise.

When the loader resolves and reconciliation completes, if `pendingRefresh` was set, fire one follow-up `refresh(x)`. Bursty SSE produces at most one extra fetch per path.

## Persistence

When `persistKey` is set, the component writes on every state change:

```json
{"version": 1, "expanded": ["a", "a/b"], "selected": "a/b/readme.md"}
```

to `localStorage.setItem(persistKey, ...)`.

### Bootstrap order

1. Read and parse. If invalid or wrong version, drop.
2. Load root via the loader.
3. Merge expansion sources:
   - `expandedFromStorage` = `parsed.expanded` (if any)
   - `expandedFromInitial` = ancestors of `initial.selectedPath` (if provided)
   - `expandedPaths` = union
4. Walk `expandedPaths` in depth-sorted order (parent-first) and call `expand(path)` sequentially.
5. Apply selection: `initial.selectedPath` if provided (URL is authoritative); else `parsed.selected`.
6. Any expanded path that 404s or is absent from current data: drop and re-persist.

`initial.selectedPath` always wins over persisted `selected`.

## Keyboard

Follows [W3C WAI-ARIA APG Tree View pattern](https://www.w3.org/WAI/ARIA/apg/patterns/treeview/), single-select. The `*` expand-siblings binding is omitted (YAGNI).

| Key | Behavior |
|---|---|
| ↓ | Focus next visible node. |
| ↑ | Focus previous visible node. |
| → | Collapsed dir: expand. Expanded dir with children: focus first child. Else: no-op. |
| ← | Expanded dir: collapse. Else: focus parent (if any). |
| Home | Focus first visible node. |
| End | Focus last visible node. |
| Enter / Space | Select focused node. Emits `tree:select { source: 'keyboard' }`. |
| a-z / 0-9 | Typeahead: focus next visible node whose name starts with the typed prefix. Buffer resets after ~500ms idle. |
| Tab / Shift+Tab | Exits the tree (roving tabindex, single tab stop). |

Arrow keys, Home/End, Space, and typeahead are not free browser behavior — each handler prevents default (page scroll) and runs the tree-level action. Roving tabindex is managed manually.

Click mapping:

- Row body (`tv-label` or `tv-icon`): `select(path, {source: 'click'})`. Sets focus.
- Chevron (`tv-toggle`): `toggle(path)`. Does not change selection. Does not move focus.
- Detection order: `event.target.closest('.tv-toggle')` first, else `.tv-item`.

## Unified SSE endpoint

The existing per-note SSE and the new tree-mutation events are unified onto one endpoint so each page opens one SSE connection, not two.

### Endpoint

```
GET /events
  ?watch=<relative path>   (optional)
```

Emits:

- `event: connected\ndata: {"type":"connected"}\n\n` on handshake.
- `event: change\ndata: {"type":"change","path":"<path>"}\n\n` when the watched note's content changes. Only emitted if `watch` was set, only for that path.
- `event: dir-changed\ndata: {"path":"<dir path>"}\n\n` when a directory's immediate contents change. All connected clients receive these. Root uses `"path": ""`.

### Server-side

- `SSEHub` → `EventHub`. Subscription is a small struct (`{ watchPath: string | "", events: chan }`).
- `fsnotify` loop splits into two broadcast paths:
  - File-content path (existing): `Write|Create` on `.md` files → `change` event to subscribers whose `watchPath` matches.
  - Dir-mutation path (new): `Create|Remove|Rename` on any entry inside a watched dir → `dir-changed` event to all subscribers. Dir events are filtered to match the loader's visibility rules (no hidden dotfiles; non-`.md` files ignored). Per-dir debounced at ~200ms.
- `dir-changed` also fires on `.md` file add/remove/rename (since these change the dir listing), even though the existing `change` path handles content updates. The two events serve different purposes and are not deduplicated.

### Dir-watch coverage

Today `SSEHub.addClient` only watches the parent directory of the client's `watchPath`. For tree-wide dir-changed coverage, the hub must watch every directory under the notes root. Implementation: recursive walk on startup, `fsnotify.Add` each dir, and on Create events for new dirs add them to the watcher dynamically. On Remove, fsnotify drops the watch automatically.

### Client glue

```js
const es = new EventSource('/events?watch=' + encodeURIComponent(currentNotePath || ''));

es.addEventListener('change', e => { /* existing note-pane refresh */ });
es.addEventListener('dir-changed', e => {
  tree.refresh(JSON.parse(e.data).path);
});
```

No reconnection logic beyond `EventSource`'s built-in retry.

## Server endpoints

### `GET /api/tree/list?path=X`

```json
[
  {"name": "a", "path": "a", "isDir": true},
  {"name": "readme.md", "path": "readme.md", "isDir": false}
]
```

- `path` is a slash-separated relative path, may arrive `%2F`-encoded. Server decodes via Go's `net/url`. Root is `""`.
- Sorted: dirs first, then files, alphabetical within each group (mirrors `readDirEntries`).
- 400 for path-traversal attempts; 404 for non-existent dirs.
- `Cache-Control: no-store`.

### Initial-state embed

Full-page HTML embeds only metadata:

```html
<script type="application/json" id="tv-initial">
{"selectedPath": "journal/day-one.md"}
</script>
```

- `/view/X` → `selectedPath: "X"`.
- `/dir/X` → `selectedPath: "X"`.
- `/tags/T`, `/`, empty state → `selectedPath: null`.

The sidebar placeholder renders empty on first HTML paint; TreeView fills it after fetching the root via the loader. Accepted first-paint flash in exchange for a single source of truth for tree data.

## Application glue

New file: `web/src/sidebar.js`. Replaces the current tree-related logic in `app.js`.

```js
import { TreeView } from './tree-view.js';

const initial = readEmbeddedInitialState(); // { selectedPath: string | null }

const tree = new TreeView(document.getElementById('sidebar-tree'), {
  loader: path => fetch('/api/tree/list?path=' + encodeURIComponent(path)).then(r => r.json()),
  initial,
  persistKey: 'notesview.tree',
});

tree.container.addEventListener('tree:select', e => {
  const { path, node } = e.detail;
  const href = node.isDir ? '/dir/' + encodePath(path) : '/view/' + encodePath(path);
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });
  history.pushState({ type: node.isDir ? 'dir' : 'note', href }, '', href);
});

const es = new EventSource('/events?watch=' + encodeURIComponent(currentNotePath || ''));
es.addEventListener('dir-changed', e => {
  tree.refresh(JSON.parse(e.data).path);
});
// existing 'change' handler for note-pane refresh is retained.

window.addEventListener('popstate', e => {
  const path = pathFromURL(location.pathname);
  const ancestors = ancestorsOf(path);
  Promise.all(ancestors.map(a => tree.expand(a)))
    .then(() => tree.select(path, { source: 'silent' }))
    .then(() => tree.scrollTo(path));
});
```

Tag clicks remain outside the tree component — tags are still a flat list rendered by the existing sidebar template.

## Current-code pointers

| File | Action |
|---|---|
| `web/src/app.js` | Remove: `selectDir` / `selectNote` sidebar branches, `toggleDir`, `ensureDirExpanded`, `ensureDirPathVisible`, `expandDirLocal`, `collapseDirLocal`, `findToggleButton`, `setChevronState`, `markSelected`, `pendingScrollToSelected`, sidebar click delegation for dirs/notes. Keep: HTMX bootstrap, highlight.js wiring, `pendingNoteScrollReset`, tag click delegation, section collapse/expand. |
| `web/templates/entry_list.html` | Remove the sidebar branch (chevron + group wrapper). Keep `entry_list` / `entry_list_rows` for the main-pane flat listings. |
| `web/templates/sidebar_tree.html` | Replace `<div id="files-content">` contents with an empty placeholder + initial-state `<script>`. Tags section stays. |
| `internal/server/handlers.go` | Remove: `GET /dir/{path}?children=1&depth=N` branch, sidebar partial branch in `handleDir`. Add: `/api/tree/list` handler. Simplify `handleDir`. |
| `internal/server/chrome.go` | Remove: `buildDirTree`, `buildTreeLevel`, `readDirEntriesAtDepth`. Keep: `readDirEntries`, `viewPath`, `tagPath`. |
| `internal/server/templates.go` | Remove: `IndexCard.Flat`, `renderEntryListRows`, `renderEntryList`. |
| `internal/server/sse.go` | Refactor: `SSEHub` → `EventHub`, subscription model, recursive dir watching, new dir-mutation broadcast path. Replace `handleSSE` with `handleEvents`. |
| `tests/sidebar-tree.spec.ts` | Rewrite Playwright suite for the new component. |
| New: `web/src/tree-view.js` | Component implementation (~500–700 lines incl. keyboard). |
| New: `web/src/sidebar.js` | App glue (~100 lines). |
| New: `web/src/tree-view.test.js` | Vitest unit tests with happy-dom. |
| New: `internal/server/tree_api.go` | `/api/tree/list` handler + dir-changed broadcast wiring. |
| New: `vitest.config.mjs` + dev deps | `vitest`, `happy-dom`. |

## Test plan

### Unit — Vitest + happy-dom

Small, fast, no browser. Cover:

- Construct wipes container; renders one `<ul>` with root children from the loader.
- `expand(path)`: loader called, rows inserted, `aria-expanded` flipped.
- `expand(path)` idempotent: no duplicate loader call, no duplicate rows.
- `expand(path)` while loading: in-flight promise returned, single loader call.
- `collapse(path)`: rows removed, model preserved.
- `select(path)`: `aria-selected` flipped, event emitted, focus moved.
- `refresh(path)` with changed children: adds new, removes deleted, preserves unchanged-and-expanded descendants.
- `refresh(path)` during load: `pendingRefresh` set, exactly one follow-up fetch after load resolves.
- Node type flip (dir → file): expansion state under old path dropped.
- Persistence round-trip: expand two dirs → destroy → reconstruct → same expansions.
- Persistence + `initial.selectedPath`: merged expansion; `initial.selectedPath` wins over persisted `selected`.
- Stale paths in storage: pruned silently and re-persisted.
- Loader rejection: `tree:error` emitted, state unchanged.
- Path with quotes/brackets: row found via `CSS.escape`-based selector.
- Loader receives correctly encoded path (round-trip through `encodeURIComponent`).

### Keyboard — Vitest + happy-dom

- Every key in the reference table, asserting focused `data-path`, `aria-expanded`, `aria-selected`.
- Typeahead prefix matching and idle-timeout buffer reset.
- Roving tabindex: exactly one node with `tabindex=0`.
- Tab leaves the tree.

### Integration — Playwright

- Open sidebar, expand dir via chevron, assert children visible.
- Chevron click on one dir does not collapse another (regression from #80).
- Select a note from main-pane listing: tree expands ancestors, highlights note, no scroll jump.
- Selecting a note from sidebar: URL updates, tree state unchanged.
- Reload preserves expanded + selected.
- Direct URL visit `/view/journal/day-one.md` from clean localStorage: tree expands `journal`, selects `day-one.md` after loader resolves.
- Dir-changed SSE: simulate via a test hook; `refresh` runs, new file appears.
- Back/forward: tree syncs to URL.

### Accessibility smoke

- Axe-core on the rendered tree: zero violations.
- `role="tree"` root, `role="treeitem"` per item, `aria-level` strictly increasing with depth.

## Edge cases

- Path with `#`, `?`, spaces, quotes, brackets: encoding handled in loader; DOM selectors use `CSS.escape`.
- Renamed dir: parent `dir-changed` → `refresh` removes old row, adds new one; if selection was a descendant, cleared.
- Deleted expanded dir: parent refresh → subtree gone; if selected, `tree:select { path: null }`.
- Node type flip: `refresh` reconciles in place; old expansion dropped.
- Loader returns empty array: node renders as expandable but empty.
- Very deep tree: unbounded; indentation via `--tv-depth`.
- Two rapid toggles of the same chevron: second is a no-op until first loader resolves (guarded by `loadingPaths`).
- Refresh during load: queued via `pendingRefresh`, at most one follow-up fetch.
- First-paint flash: accepted tradeoff; server does not embed root entries.

## Risks and tradeoffs

- **Two rendering systems in the app.** The rest of the app is server-rendered + HTMX; the sidebar is now client-rendered. Accepted because the sidebar's requirements (stateful, interactive, long-lived state) differ fundamentally from stateless main-pane swaps.
- **No-JS regression.** The current SSR tree renders without JS; the new one does not. Accepted — this is a notes-viewing app with JS as a hard dependency elsewhere.
- **fsnotify watcher count.** Recursive watching adds one watcher per dir. For notes trees with thousands of dirs, this approaches OS limits (`fs.inotify.max_user_watches` defaults to 8192 on Linux). Acceptable for the current use case; revisit if the notes tree grows that large.
- **First-paint flash for deep URLs.** Server embeds metadata only, so `/view/a/b/c/d.md` first paints with an empty sidebar. Usually invisible on localhost, possibly noticeable over network. Revisit by embedding ancestor-path data if it becomes a real problem — additive change to the glue, not a component API change.
- **`dir-changed` and `change` can both fire for the same `.md` write.** Not deduplicated. The tree refresh is cheap (one listing fetch per changed dir); overlap is acceptable.

## References

- Issue: #88
- Retrospective context: #80 (comment)
- Related prior work: #84, #85, #86, #87
- W3C WAI-ARIA Authoring Practices, Tree View pattern: https://www.w3.org/WAI/ARIA/apg/patterns/treeview/
