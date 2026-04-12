# Independent sidebar and note panels — design

## Problem

The current layout swaps both panels on every click. The root cause is
`hx-boost="true" hx-target="#content"` on `<body>`, combined with a single
`#content` element wrapping both the sidebar and the note card. Every boosted
navigation — clicking a note in the index, clicking a directory, toggling the
sidebar, wiki-links inside note content — replaces the whole `#content`
subtree.

This causes five user-visible problems:

1. Clicking a note in the sidebar re-renders the sidebar too, resetting its
   scroll position.
2. Clicking a directory in the sidebar re-renders the note, even though the
   note HTML is byte-identical (the note card gets re-parsed and re-swapped;
   text selection is lost).
3. Toggling the sidebar triggers a full server round-trip for a piece of
   state that is purely about visibility.
4. The sidebar and the note card share a single scroll container — neither
   can scroll independently of the other.
5. The topbar is not truly fixed relative to the panels; it's absolutely
   positioned but inside the same layout flow as everything else.

## Goals

- Click a note in the sidebar → only the note panel refreshes. Sidebar DOM,
  sidebar scroll position, and any future sidebar-internal state survive.
- Click a directory in the sidebar → only the sidebar refreshes. Note panel
  DOM and scroll position survive.
- Toggle sidebar visibility → no server round-trip, no note reload. Pure
  client-side state change.
- Sidebar is a 100vh scroll container, independent of the note panel.
- Note panel is a 100vh scroll container, independent of the sidebar.
- Topbar is fixed to the viewport, not to the document.
- SSE live-reload on the watched note swaps only the note panel; sidebar
  scroll position survives.
- External links inside note content open in the browser normally, not via
  boosted XHR.
- The URL remains the single source of truth for deep-linkable state: which
  note is open, which directory the sidebar is showing (when open).

## Non-goals

- **Mobile layout tuning.** Deferred, as in the previous pass. Desktop is the
  target; mobile degrades as it does today.
- **Sidebar refresh on directory-content changes.** SSE today only watches the
  current note. Adding, removing, or renaming files in the sidebar's shown
  directory does not auto-update the sidebar. This is a pre-existing gap and
  stays out of scope; see *Future work*.
- **Search and tag modes for the sidebar.** The current `IndexCard.Mode`
  discriminator is kept as an extensibility hook, but no new modes ship in
  this refactor.
- **Panel state memory across close/reopen.** Reopening the sidebar defaults
  to the current note's parent directory; no `localStorage` or cookie is used
  to remember a last-seen directory.
- **Scroll restoration on back/forward navigation.** With two independent
  scroll containers, the browser's native scroll restoration doesn't map
  to a single viewport scroll. Each pane would need per-history-entry
  scroll state. Out of scope for this refactor; see *Future work*.

## Architecture

### Layout: three fixed regions

```
<body>
  <header id="topbar" class="fixed top-0 left-0 right-0 h-12 ..."> ... </header>
  <aside  id="sidebar"   class="fixed top-12 left-0 bottom-0 w-[320px] overflow-y-auto ..."> ... </aside>
  <main   id="note-pane" class="fixed top-12 left-[320px] right-0 bottom-0 overflow-y-auto ..."> ... </main>
</body>
```

- **`#topbar`** is fixed across the full viewport width, `h-12`, `z-[100]`,
  contains the hamburger and the edit button. Never re-rendered by navigation
  (HTMX swaps target either `#sidebar` or `#note-pane`, never the topbar).
- **`#sidebar`** is fixed to the viewport, positioned below the topbar,
  320px wide, its own `overflow-y: auto` scroll container. When the sidebar
  is closed, body gets a `.sidebar-closed` class that sets `#sidebar`'s
  `display: none` (or `transform: translateX(-100%)` if we want a slide
  animation later) and moves `#note-pane`'s left edge to `0`.
- **`#note-pane`** is fixed to the viewport, positioned below the topbar and
  to the right of the sidebar, its own `overflow-y: auto` scroll container.

Each panel is a DOM element that HTMX can swap independently. Neither panel
is a descendant of the other; they are siblings. Scroll position is a
property of the DOM element, so as long as the element is not replaced, its
scroll survives.

### HTMX wiring: per-link explicit boost, nothing on `<body>`

`<body>` carries no HTMX attributes. There is no ancestor `hx-boost` to
catch link clicks. Instead, every link that needs HTMX behavior declares it
itself, inline:

```html
<a href="/view/foo.md?dir=x/y" hx-boost="true" hx-target="#note-pane">foo</a>
```

Two HTMX attributes per internal link: `hx-boost="true"` + `hx-target`.
`href` stays real (right-click "open in new tab" works; no-JS fallback is
a full page load), and there's no URL duplication because `hx-boost` reuses
the `href` instead of taking a separate `hx-get`.

Internal links — the ones that need HTMX — come from two places:

- **Sidebar templates** (`web/templates/index_card.html`,
  `web/templates/breadcrumbs.html`). Each link declares its target:
  directory entries, breadcrumb segments, and the home icon use
  `hx-target="#sidebar"`; file entries use `hx-target="#note-pane"`.
- **Renderer** (`internal/renderer/noteext.go`). The goldmark extension's
  custom `NodeRenderer` for `*ast.Link` emits `hx-boost="true"
  hx-target="#note-pane"` on any link whose final destination is
  internal (starts with `/view/`). See *Renderer* below for the
  extension's structure.

### External links are plain HTML

Links inside note content that point outside the note system —
`https://...`, `http://...`, `mailto:...`, protocol-relative URLs,
`/static/...` assets, and anything else goldmark emits with an `href` the
renderer doesn't rewrite — stay as plain `<a href="...">`. No HTMX
attributes, no opt-out markers.

Because there is no `hx-boost` on `<body>` or any ancestor, HTMX simply
doesn't see these links. A click on `https://example.com` is a normal
browser navigation. This is the point of annotating the internal links
instead of the external ones: the specialness (HTMX behavior) lives with
the special elements, and everything else is ordinary HTML.

External links are not specially detected or rewritten anywhere; they
fall through the goldmark extension's link renderer as-is. See
*Renderer* for the mechanics. `target="_blank"` / `rel="noopener"` is
a separate product decision and is out of scope for this refactor.

### Server response shapes

Handlers check `HX-Request` and `HX-Target` headers and pick a response
shape. HTMX sends `HX-Target` as the raw id value, without the `#` prefix:

- **`HX-Request: true` + `HX-Target: note-pane`** → render `note-pane`
  partial (the `<main id="note-pane">` subtree only, no layout, no sidebar).
- **`HX-Request: true` + `HX-Target: sidebar`** → render `sidebar` partial
  (the `<aside id="sidebar">` subtree only).
- **No `HX-Request` header (initial load, refresh, direct URL)** → render
  the full page with both panels composed together.

This is efficient: a sidebar navigation click does not run the markdown
renderer, and a note click does not run the directory reader.

**404 for partial note requests.** When a note doesn't exist and the
request carries `HX-Target: note-pane`, the server responds with HTTP
`200 OK` and a body containing the empty-state fragment (`<main
id="note-pane">…"note not found"…</main>`). HTMX swaps it into the note
pane, leaving the sidebar untouched. Returning a non-2xx status here
would cause HTMX to skip the swap by default, leaving the user staring
at the previous note with no indication the click did anything.
Full-page requests (`/view/missing.md` typed directly in the URL bar)
still respond with HTTP `404`, as today.

**`dirQuery` on note-pane partial responses.** When the incoming
request has `?dir=x/y` set, the note-pane partial it returns must have
its wiki-links rewritten with `?dir=x/y` threaded through — same as a
full-page render. Otherwise a wiki-link click from within a partial
response would lose the sticky directory. `handleView` resolves the
sticky directory identically for partial and full-page response paths
and passes it to the renderer as `dirQuery`.

### Sidebar toggle: client-side visibility + opt-in refresh

The hamburger is no longer an `<a>`. It becomes a `<button>` with a small JS
handler. The button carries `aria-expanded` and `aria-controls="sidebar"`
so screen readers announce the state change.

```js
function toggleSidebar() {
  const open = document.body.classList.toggle('sidebar-open');
  hamburgerBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
  localStorage.setItem('notesview.sidebarOpen', open ? '1' : '0');
  if (open) {
    // Refresh sidebar for the current note to avoid staleness:
    // the sidebar DOM froze at whatever it rendered last, while the
    // user navigated notes via wiki-links with the sidebar hidden.
    htmx.ajax('GET', currentSidebarUrl(), { target: '#sidebar', swap: 'outerHTML' });
  } else {
    // Strip ?dir= from the URL so the closed state has no URL residue.
    const url = new URL(window.location.href);
    url.searchParams.delete('dir');
    history.replaceState(null, '', url.toString());
  }
}
```

On page load, the script reads `localStorage` and applies the class before
first paint (inline script in `<head>` to avoid a flash). Initial sidebar
content is always rendered server-side so "sidebar was open on last visit"
works without a second request.

**Why the opt-in refresh.** When the sidebar is hidden, it stays in the DOM
with whatever it was last rendered with. Meanwhile the user clicks wiki-links
in the note, moving through files. If they then open the sidebar, the
sidebar is showing the directory of the note they *started* with, not the
note they're *currently* reading. The refresh on open fixes this. Closing
stays pure client-side (no server involvement).

**`currentSidebarUrl()` definition.** The URL is built from the current
note path plus `?dir=<note's-parent>`. When the current page has no note
in view (the `/` empty-state case), it returns `/?dir=` (root). Reopening
always defaults to the note's parent directory regardless of any prior
sticky state, per the "no hidden state" decision (see *URL model*).

```js
function currentSidebarUrl() {
  const notePath = document.body.dataset.notePath; // "" when no note
  const parent = notePath ? notePath.replace(/[^/]*$/, '').replace(/\/$/, '') : '';
  return (notePath ? `/view/${notePath}` : '/') + `?dir=${encodeURIComponent(parent)}`;
}
```

`document.body.dataset.notePath` is set by the layout template on
full-page renders and preserved across note-pane swaps (the note path is
a data attribute on `#note-pane` and the toggle reads it from there;
pick whichever element survives the swap, as long as it's consistent).

### Browser back/forward semantics

Every sidebar navigation click and every note click goes through
`hx-boost`, which calls `history.pushState` with the request URL. This
means:

1. User opens `/view/a.md` (initial load).
2. User clicks directory `x/y` in the sidebar → `history.pushState` to
   `/view/a.md?dir=x/y`. Sidebar swap only.
3. User clicks file `x/y/b.md` in the sidebar → `history.pushState` to
   `/view/x/y/b.md?dir=x/y`. Note-pane swap only.
4. User closes sidebar → `history.replaceState` to `/view/x/y/b.md`
   (strips `?dir=`). No new history entry.
5. User presses Back → browser navigates to `/view/a.md?dir=x/y`. HTMX's
   `htmx:historyRestore` fires. We rely on the fact that `?dir=x/y` +
   sidebar-closed (from `localStorage`) means: initial-load render,
   sidebar DOM populated from URL but visually hidden by the
   `.sidebar-closed` class. This is consistent with step 2's original
   render minus the animation.
6. Back again → `/view/a.md` → same, no `?dir=`, default sidebar on
   reopen would be `/` (the note's parent).

Closing the sidebar uses `replaceState` rather than `pushState` because
visibility is a UI preference, not a navigational event — pressing Back
after closing should take the user to the previous note, not to a
"sidebar was open" state of the current note.

The note pane does not currently persist its own scroll position across
back/forward; see *Non-goals*.

### URL model

- `/view/<file>.md` — note alone, sidebar may be visible or hidden per
  client-side state. This URL is complete on its own.
- `/view/<file>.md?dir=<dir>` — note plus an explicit sticky directory for
  the sidebar. The server always builds the sidebar DOM for this directory
  on initial load regardless of whether the client will show it; the
  `.sidebar-closed` class applied by the pre-paint script determines
  visibility. Sidebar navigation inside the page (dir clicks, breadcrumb
  clicks) updates this `?dir=` param in the browser URL via `hx-boost`'s
  history push.
- `/?dir=<dir>` — no note in view. Renders the two-pane layout with an
  empty-state placeholder in `#note-pane` ("No note selected"). The sidebar
  shows `<dir>`.
- `/` — redirects to `/view/README.md` if one exists; otherwise serves the
  two-pane layout with the empty state and the sidebar at root.

The `?index=dir` parameter from the current implementation goes away.
Sidebar visibility is no longer a URL concept. This changes the deep-link
surface: "here's a link with the sidebar open" is no longer expressible.
Users can still link directly to a directory view via `?dir=`, but whether
the sidebar is visible when the link is opened depends on the recipient's
`localStorage`. This is an intentional tradeoff — visibility is a UI
preference, not a piece of shareable state.

**`?dir=` is stripped when the sidebar closes.** There is no hidden state
outside the URL. Reopening the sidebar defaults to the current note's
parent directory.

**`dirQuery` threading in the renderer stays.** When the sidebar is open
and showing directory `x/y`, in-note wiki-links must be rewritten to include
`?dir=x/y` so that clicking a wiki-link preserves the sticky directory in
the URL. Otherwise refreshing after a wiki-link click would reset the
sidebar to the new note's parent, silently losing the user's sticky
position. The goldmark extension's `ASTTransformer` reads the current
`dirQuery` from `parser.Context` and appends it to every rewritten
internal-link destination.

### SSE live-reload

SSE moves from `#content` to `#note-pane`:

```html
<main id="note-pane"
      hx-ext="sse"
      sse-connect="/events?watch={{ .FilePath }}"
      hx-trigger="sse:change"
      hx-get="/view/{{ .FilePath }}{{ .DirQuery }}"
      hx-target="#note-pane"
      hx-swap="outerHTML">
```

On file change, HTMX fetches the note URL (with `HX-Request` + `HX-Target:
note-pane`) and the server returns a `note-pane` partial. `#sidebar` is a
sibling DOM node and is not touched — its scroll position, highlighted
entry, and any future in-panel state all survive.

## Component boundaries

### Server

- **`handleView`** — resolves the note, parses `?dir=` for the sidebar
  position, renders either the full two-pane page or a `note-pane` partial
  depending on `HX-Request` / `HX-Target`. Does NOT build the sidebar when
  the response is a `note-pane` partial.
- **`handleSidebar` (new)** — builds the `IndexCard` for a given directory
  plus an optional current-note path (for "keep the note visible" sticky
  links). Returns the `<aside id="sidebar">` partial. Called via
  `hx-target="#sidebar"` on dir-navigation links and via the hamburger's
  open-refresh `htmx.ajax` call.
- **`handleRoot`** — same redirect logic as today for `/`, but the
  "standalone index" case becomes a two-pane render with an empty note
  placeholder, not a sidebar-only page.

Endpoint shape for sidebar requests: `GET /view/<file>.md?dir=<dir>` with
`HX-Target: sidebar` returns the sidebar partial. `GET /?dir=<dir>` with
`HX-Target: sidebar` returns the sidebar partial without a note context. No
new URL is needed — the response shape is determined by the `HX-Target`
header, not the route. (Alternative: a dedicated `GET /api/sidebar?...`
route. We go with header-driven dispatch because it keeps the URL surface
stable and lets the same URLs serve both full pages and partials.)

### Templates

- **`layout.html`** — contains topbar, sidebar container, note-pane
  container. The sidebar container receives `{{ template "sidebar_body" . }}`
  and the note-pane container receives `{{ template "note_pane_body" . }}`.
- **`sidebar_body`** (new partial) — the inner HTML of `#sidebar`: the
  breadcrumbs + entry list + empty state. This is what `handleSidebar`
  returns as a partial.
- **`note_pane_body`** (new partial) — the inner HTML of `#note-pane`: the
  note card plus SSE wiring. This is what `handleView` returns as a partial
  when the request targets `#note-pane`.
- **`index_card.html`** — dir entries carry `hx-boost="true"
  hx-target="#sidebar"`; file entries carry `hx-boost="true"
  hx-target="#note-pane"`.
- **`breadcrumbs.html`** — all links (home icon + each segment) carry
  `hx-boost="true" hx-target="#sidebar"`.
- **`view.html` / `browse.html`** — both collapse into the single
  `layout.html` composition. `browse.html` can be removed entirely.

### Frontend

- **`web/src/app.js`** — gains the sidebar toggle handler and the on-load
  `localStorage` read. The topbar gets a pre-paint inline `<script>` to set
  the initial `.sidebar-open` class before first render (to avoid a flash
  of wrong state). Existing htmx + SSE imports and syntax highlighting stay.

### Renderer

The regex-over-rendered-HTML approach in `internal/renderer/notelinks.go`
is replaced with a goldmark extension that operates on the AST. The
existing three passes become two well-defined extension points, and no
HTML string manipulation happens at all.

**`internal/renderer/noteext.go`** (new file, replacing `notelinks.go`)
defines a goldmark extension with two pieces:

1. **`parser.ASTTransformer`** — runs after parsing, before rendering.
   Walks the AST with per-request state (index, `currentDir`, `dirQuery`)
   pulled from `parser.Context`:
   - For each `*ast.Link` whose `Destination` begins with `note://`:
     look up the UID in the index. On hit, rewrite `Destination` to
     `/view/<resolved>.md` + `dirQuery` suffix. On miss, set
     `Destination` to `#`, and attach a `broken` flag via
     `node.SetAttributeString("data-broken", true)` so the renderer can
     emit `class="broken-link"` and the not-found title.
   - For each `*ast.Link` whose `Destination` is relative and ends in
     `.md`: resolve against `currentDir` with `path.Clean(path.Join(...))`,
     rewrite `Destination` to `/view/<resolved>` + `dirQuery`. Skip if
     the destination already contains `://` or starts with `/`.
   - For each `*ast.Text` node: regex-match bare UIDs in the text's
     raw byte content (no HTML in sight — Text node values are plain).
     Where a match lands and the UID resolves, split the Text node into
     `Text + Link + Text` and insert a new `*ast.Link` with
     `Destination = "/view/<resolved>.md" + dirQuery` and a `uid` flag
     attribute so the renderer can emit `class="uid-link"`.

2. **Custom `NodeRenderer` for `ast.KindLink`** — registered with a
   priority that overrides goldmark's default HTML link renderer. Emits
   the opening `<a>` tag and decides whether to attach HTMX attributes
   based on the final `Destination`:
   - If `Destination` starts with `/view/` → internal link → emit
     `hx-boost="true" hx-target="#note-pane"` alongside `href`.
   - Otherwise → external link → emit plain `<a href>` with no HTMX
     attributes. External links are handled by this same renderer
     because goldmark sends all `*ast.Link` nodes through it; they
     simply fall into the "not internal" branch.
   - Honor `data-broken`: emit `class="broken-link"` and the title.
   - Honor `uid`: emit `class="uid-link"`.
   - Still emits the standard `title` attribute if `n.Title` is set.

**Per-request state flow.** The renderer wrapper in
`internal/renderer/renderer.go` stashes state in `parser.Context` before
calling `md.Convert`:

```go
type noteLinkState struct {
    idx        *index.Index
    currentDir string
    dirQuery   string // "?dir=a/b" or ""
}

var noteLinkStateKey = parser.NewContextKey()

func (r *Renderer) Render(src []byte, currentDir, dirQuery string) ([]byte, *Frontmatter, error) {
    pc := parser.NewContext()
    pc.Set(noteLinkStateKey, &noteLinkState{r.idx, currentDir, dirQuery})
    var buf bytes.Buffer
    if err := r.md.Convert(src, &buf, parser.WithContext(pc)); err != nil {
        return nil, nil, err
    }
    // frontmatter handling unchanged
    ...
}
```

One shared `goldmark.Markdown` instance is constructed at startup with
the extension registered; no per-request allocation of parsers or
renderers.

**Parameter rename.** The parameter previously called `linkQuery` in
`processNoteLinks` becomes `dirQuery` throughout to match the URL param
name. Same semantics.

**External-link handling is implicit.** The custom renderer sees every
`*ast.Link`, including those pointing at `https://...`, `mailto:...`,
`/static/foo.png`, etc. It simply emits plain `<a href>` for them —
the "internal vs external" distinction is one `bytes.HasPrefix` check
on `Destination`. No separate detection pass needed.

## Error handling

- **Sidebar build failures** (vanished directory, permissions error) — log
  a warning and return the sidebar partial with an empty-state message,
  same spirit as today's `s.logger.Warn("index card build failed", ...)`.
  On full-page requests, the note still renders even if the sidebar can't
  be built.
- **Note not found** — 404, same as today. For `HX-Request` note-pane
  requests, the response body is an empty-state fragment so HTMX swaps
  "note not found" into `#note-pane` instead of the browser showing an
  error page. Sidebar is untouched.
- **External URL in sidebar or breadcrumbs** — should never happen; all
  links there are constructed by `chrome.go` from filesystem paths. If
  somehow a malformed entry slips through, the existing `SafePath` guard
  catches it before the handler renders anything.

## Testing

- **`TestViewHandlerSidebarPartial`** — request `/view/foo.md?dir=a/b` with
  `HX-Request: true` and `HX-Target: sidebar`, assert response is the
  sidebar partial and contains no `<main id="note-pane">`.
- **`TestViewHandlerNotePanePartial`** — same URL with `HX-Target:
  note-pane`, assert response is the note partial and contains no
  `<aside id="sidebar">`.
- **`TestViewHandlerFullPage`** — no `HX-Request` header, assert response
  is the full two-pane layout.
- **`TestStickyDirectoryOnFileClick`** — verify that file entries in the
  sidebar emit URLs with the current `?dir=` preserved.
- **`TestStickyNoteOnDirectoryClick`** — verify that directory entries in
  the sidebar emit URLs with the current note path preserved.
- **`TestInternalLinkBoostAttributes`** — goldmark-convert a note
  containing `[wiki](./rel.md)`, `[proto](note://20260101_1)`, and
  bare-UID text `20260101_0001`, assert each emitted `<a>` has
  `hx-boost="true"` and `hx-target="#note-pane"` alongside its rewritten
  `href`.
- **`TestExternalLinksStayPlain`** — goldmark-convert a note containing
  `[a](https://x)`, `[b](mailto:x@y)`, `[d](/static/foo.png)`, assert each
  emitted `<a>` has NO `hx-boost` or `hx-target` attribute and no other
  HTMX attributes.
- **`TestASTTransformerRewritesLinks`** — parse a note to an AST, run
  the extension's transformer, walk the AST and assert `*ast.Link`
  destinations have been rewritten (`note://` resolved, relative `.md`
  resolved, bare UIDs auto-linked). Tests the transformer in isolation
  from the renderer.
- **`TestBrokenNoteLink`** — `[x](note://99999999_99999)` (non-existent
  UID), assert the rendered `<a>` has `href="#"`, `class="broken-link"`,
  and a "Note ... not found" title. No HTMX attributes.
- **`TestLiveReloadTargetsNotePaneOnly`** — verify the SSE wiring on
  `#note-pane` emits `hx-target="#note-pane"` with the current `?dir=`
  preserved in the reload URL.
- **`TestRootRedirectsToReadme`** and **`TestRootEmptyStateWhenNoReadme`**
  — both existing behaviors, adapted to the new two-pane layout.
- **Existing sticky-path tests** (`TestViewHandlerStickyPath`,
  `TestViewHandlerPathSurvivesSelfLinks`) continue to apply with minor
  adjustments for the new `?dir=` param name and absence of `?index=`.

## Migration notes

- `?index=dir` is removed entirely. There's no compat shim (the project has
  not had a stable release).
- `web/templates/browse.html` is deleted.
- `web/templates/sidebar.html` (already removed in the previous pass) stays
  removed.
- `internal/server/chrome.go`'s `indexQuery`, `toggleHref`, and
  `buildLayoutFields` functions are rewritten or removed: sidebar
  visibility is no longer a server concern, so `ToggleHref`, `IndexOpen`,
  and `ShowToggle` drop from `layoutFields`. The field and helper
  previously called `IndexQuery` / `indexQuery` is renamed to
  `DirQuery` / `dirQuery` throughout (server, template field, and the
  goldmark extension parameter) to match the URL param name.
- `dirLinkHref` and `fileLinkHref` become simpler: they no longer need to
  preserve the sidebar-open state because the sidebar-open state isn't in
  the URL anymore. `dirLinkHref` becomes `/view/<note>?dir=<new-dir>` and
  `fileLinkHref` becomes `/view/<new-file>?dir=<sidebar-dir>`.

## Future work

- **Sidebar SSE.** Wire a second SSE connection on `#sidebar` that watches
  the currently-shown directory. Would update the sidebar in place when
  files are added, removed, or renamed. Purely additive — the swap target
  and fragment endpoint already exist after this refactor.
- **Mobile drawer.** Narrow-viewport sidebar becomes a slide-out drawer
  over the note pane, with a tap-outside-to-close backdrop. Shares the
  client-side toggle and fragment endpoint from this refactor.
- **Search and tag modes.** `IndexCard.Mode` discriminator already exists;
  add new branches to the sidebar template and new input parsing to the
  sidebar handler.
- **Last-seen directory memory.** If the "reopen defaults to note's parent"
  behavior proves annoying, persist last-shown dir in `localStorage` and
  restore on reopen. Intentionally not done in this pass.
- **Per-pane scroll restoration.** Browser-native scroll restoration
  assumes one scroll container per history entry; with two independent
  scroll containers, back/forward nav restores neither. A future pass
  can stash `(#sidebar scrollTop, #note-pane scrollTop)` per history
  entry in `history.state` and restore on `popstate`.
