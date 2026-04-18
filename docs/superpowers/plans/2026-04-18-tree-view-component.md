# Tree View Component Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the server-rendered sidebar tree with a reusable, pure-JS client-side `TreeView` component driven by a JSON endpoint and a unified SSE event stream.

**Architecture:** Three layers with clean boundaries. (1) Server: `/api/tree/list?path=X` returns JSON children; unified `/events` emits `change` (per-note) and `dir-changed` (tree mutations). (2) Component: `TreeView` class with methods (`expand`, `collapse`, `select`, `refresh`, etc.) and prefixed DOM events (`tree:select`, `tree:toggle`, `tree:error`). State lives in memory Maps/Sets; DOM is a projection. (3) App glue: `web/src/sidebar.js` wires TreeView events to HTMX navigation and consumes SSE.

**Tech Stack:** Plain JS module (no framework). Vitest + happy-dom for unit tests. Tailwind utility classes for styling. Go `fsnotify` for recursive directory watching. Existing Playwright suite for integration.

**Spec:** `docs/superpowers/specs/2026-04-18-tree-view-component-design.md`

**Issue:** #88

---

## File structure

**New files:**
- `web/src/tree-view.js` — component class (state, DOM reconciliation, keyboard, persistence).
- `web/src/tree-view.test.js` — Vitest unit tests co-located with the module.
- `web/src/sidebar.js` — application glue (TreeView construction + navigation + SSE).
- `internal/server/tree_api.go` — `/api/tree/list` handler.
- `vitest.config.mjs` — Vitest config.

**Renamed:**
- `internal/server/sse.go` → `internal/server/events.go` (contents refactored: `SSEHub` → `EventHub`, new `Subscription`, new dir-changed broadcast path, recursive directory watching).
- `internal/server/sse_test.go` → `internal/server/events_test.go` (tests updated).

**Modified:**
- `web/src/app.js` — remove tree-related code; keep HTMX bootstrap, highlight.js, tag click delegation, section collapse/expand, `pendingNoteScrollReset`.
- `web/templates/sidebar_tree.html` — replace tree rendering with empty placeholder + `<script type="application/json" id="tv-initial">`.
- `web/templates/entry_list.html` — remove chevron-dir branch (flat-only).
- `internal/server/handlers.go` — remove `?children=1&depth=N` branch; remove sidebar partial branch from `handleDir`; add initial-state JSON to full-page responses; drop `buildDirTree` calls.
- `internal/server/chrome.go` — remove `buildDirTree`, `buildTreeLevel`, `readDirEntriesAtDepth`.
- `internal/server/templates.go` — remove `IndexCard.Flat`, `renderEntryListRows`, `renderEntryList`; add `InitialJSON` to `SidebarPartialData`, remove `Files`.
- `internal/server/server.go` — rename `sseHub` → `events`; rename routing to `/events` unchanged endpoint path, renamed handler `handleEvents`.
- `tests/sidebar-tree.spec.ts` — rewrite for the new component.
- `package.json` — add `vitest`, `happy-dom` dev deps; add `test:unit` script.
- `CHANGELOG.md` — entry.

---

## Testing strategy

- **Unit tests (fast, Vitest + happy-dom):** every `TreeView` public method, state transition, keyboard handler, persistence path. Co-located in `web/src/tree-view.test.js`.
- **Go tests (`go test ./...`):** `/api/tree/list` handler, `EventHub` refactor preserves existing per-note semantics, new `dir-changed` broadcast.
- **Playwright integration:** end-to-end flows (open sidebar, expand, select, reload persists, SSE refreshes, back/forward). No component-level behavior duplicated here.

---

## Task 1: Add Vitest + happy-dom scaffolding

**Files:**
- Modify: `package.json`
- Create: `vitest.config.mjs`

- [ ] **Step 1: Add dev dependencies**

Run: `npm install --save-dev vitest@^2 happy-dom@^15`

Expected: `package.json` gains `vitest` and `happy-dom` under `devDependencies`; `package-lock.json` updates.

- [ ] **Step 2: Add npm test script**

Edit `package.json` to change the `"scripts": {}` object to:

```json
  "scripts": {
    "test:unit": "vitest run",
    "test:unit:watch": "vitest"
  }
```

- [ ] **Step 3: Create Vitest config**

Create `vitest.config.mjs`:

```js
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'happy-dom',
    include: ['web/src/**/*.test.js'],
    globals: false,
  },
})
```

- [ ] **Step 4: Sanity test**

Create `web/src/smoke.test.js`:

```js
import { describe, it, expect } from 'vitest'

describe('smoke', () => {
  it('runs in a DOM env', () => {
    const el = document.createElement('div')
    el.textContent = 'ok'
    expect(el.textContent).toBe('ok')
  })
})
```

Run: `npm run test:unit`

Expected: 1 passing test.

- [ ] **Step 5: Remove the smoke test and commit**

```bash
rm web/src/smoke.test.js
git add package.json package-lock.json vitest.config.mjs
git commit -m "Add Vitest + happy-dom harness for unit tests"
```

---

## Task 2: TreeView — construction and root load

**Files:**
- Create: `web/src/tree-view.js`
- Create: `web/src/tree-view.test.js`

The component starts as the smallest working surface: constructor, root-level loader call, minimal DOM projection. Subsequent tasks add expand/collapse, selection, refresh, keyboard, persistence.

- [ ] **Step 1: Write the failing test**

Create `web/src/tree-view.test.js`:

```js
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { TreeView } from './tree-view.js'

function makeLoader(tree) {
  return vi.fn(async (path) => (tree[path] || []).slice())
}

const twoRoot = {
  '': [
    { name: 'a', path: 'a', isDir: true },
    { name: 'readme.md', path: 'readme.md', isDir: false },
  ],
}

describe('TreeView construction', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('wipes the container and mounts a tv-root with role="tree"', async () => {
    container.innerHTML = '<p>stale</p>'
    const tv = new TreeView(container, { loader: makeLoader(twoRoot) })
    await tv.ready
    expect(container.querySelector('p')).toBeNull()
    expect(container.querySelector('.tv-root')).toBeTruthy()
    expect(container.querySelector('.tv-root').getAttribute('role')).toBe('tree')
  })

  it('calls loader with rootPath on construction and renders returned children', async () => {
    const loader = makeLoader(twoRoot)
    const tv = new TreeView(container, { loader })
    await tv.ready
    expect(loader).toHaveBeenCalledWith('')
    const items = container.querySelectorAll('[role="treeitem"]')
    expect(items.length).toBe(2)
    expect(items[0].getAttribute('data-path')).toBe('a')
    expect(items[0].classList.contains('tv-item--dir')).toBe(true)
    expect(items[1].getAttribute('data-path')).toBe('readme.md')
    expect(items[1].classList.contains('tv-item--file')).toBe(true)
  })

  it('uses rootPath option when provided', async () => {
    const loader = makeLoader({ 'sub': [{ name: 'x.md', path: 'sub/x.md', isDir: false }] })
    const tv = new TreeView(container, { loader, rootPath: 'sub' })
    await tv.ready
    expect(loader).toHaveBeenCalledWith('sub')
    expect(container.querySelector('[data-path="sub/x.md"]')).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test:unit`

Expected: `tree-view.js` not found; or "TreeView is not a constructor". Failures on all three tests.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/tree-view.js`:

```js
// TreeView — reusable client-side tree view component.
//
// Public API:
//   const tv = new TreeView(container, { loader, rootPath, persistKey, initial, ... })
//   await tv.ready
//   tv.expand(path) / tv.collapse(path) / tv.toggle(path)
//   tv.select(path, { source })
//   tv.refresh(path)
//   tv.scrollTo(path, { block })
//   tv.destroy()
//
// Events dispatched on `container`:
//   tree:select { path, node, source }
//   tree:toggle { path, expanded }
//   tree:error  { path, error }
//
// See docs/superpowers/specs/2026-04-18-tree-view-component-design.md

const DIR_CLASS = 'tv-item--dir'
const FILE_CLASS = 'tv-item--file'

function escapeSelector(s) {
  if (typeof CSS !== 'undefined' && CSS.escape) return CSS.escape(s)
  return String(s).replace(/["\\[\]]/g, '\\$&')
}

export class TreeView {
  constructor(container, options) {
    if (!container) throw new Error('TreeView: container is required')
    if (!options || typeof options.loader !== 'function') {
      throw new Error('TreeView: loader is required')
    }
    this.container = container
    this.loader = options.loader
    this.rootPath = options.rootPath ?? ''
    this.renderLabel = options.renderLabel
    this.renderIcon = options.renderIcon
    this.classPrefix = options.classPrefix ?? 'tv-'

    this.nodesByPath = new Map()
    this.childrenByPath = new Map()
    this.expandedPaths = new Set()
    this.selectedPath = null
    this.focusedPath = null
    this.loadingPaths = new Map()

    this.container.innerHTML = ''
    this.root = document.createElement('div')
    this.root.className = 'tv-root'
    this.root.setAttribute('role', 'tree')
    this.container.appendChild(this.root)

    this.ready = this._bootstrap()
  }

  async _bootstrap() {
    const children = await this._loadChildren(this.rootPath)
    this._renderChildren(this.rootPath, this.root, children, 0)
  }

  async _loadChildren(path) {
    const nodes = await this.loader(path)
    this.childrenByPath.set(path, nodes.map((n) => n.path))
    for (const n of nodes) this.nodesByPath.set(n.path, n)
    return nodes
  }

  _renderChildren(parentPath, parentEl, nodes, baseLevel) {
    const ul = document.createElement('ul')
    ul.className = 'tv-group list-none m-0 p-0'
    ul.setAttribute('role', 'group')
    for (const node of nodes) {
      ul.appendChild(this._buildItem(node, baseLevel + 1))
    }
    parentEl.appendChild(ul)
  }

  _buildItem(node, level) {
    const li = document.createElement('li')
    li.className = `tv-item ${node.isDir ? DIR_CLASS : FILE_CLASS}`
    li.setAttribute('role', 'treeitem')
    li.setAttribute('data-path', node.path)
    li.setAttribute('aria-level', String(level))
    li.setAttribute('aria-selected', 'false')
    li.setAttribute('tabindex', '-1')
    li.style.setProperty('--tv-depth', String(level - 1))
    if (node.isDir) li.setAttribute('aria-expanded', 'false')

    const row = document.createElement('div')
    row.className = 'tv-row flex items-center gap-2 px-4 py-2 text-sm'
    if (node.isDir) {
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = 'tv-toggle flex items-center justify-center w-8 flex-shrink-0 text-gray-400 cursor-pointer bg-transparent border-0 p-0'
      btn.setAttribute('tabindex', '-1')
      btn.setAttribute('aria-hidden', 'true')
      btn.textContent = '\u25B8'
      row.appendChild(btn)
    } else {
      const spacer = document.createElement('span')
      spacer.className = 'tv-toggle-spacer w-8 flex-shrink-0'
      row.appendChild(spacer)
    }

    const icon = document.createElement('span')
    icon.className = 'tv-icon flex-shrink-0'
    if (typeof this.renderIcon === 'function') {
      const result = this.renderIcon(node)
      if (typeof result === 'string') icon.textContent = result
      else if (result instanceof Node) icon.appendChild(result)
    } else {
      icon.textContent = node.isDir ? '\uD83D\uDCC1' : '\uD83D\uDCC4'
    }
    row.appendChild(icon)

    const label = document.createElement('span')
    label.className = 'tv-label truncate min-w-0 text-blue-600'
    if (typeof this.renderLabel === 'function') {
      const result = this.renderLabel(node)
      if (typeof result === 'string') label.textContent = result
      else if (result instanceof Node) label.appendChild(result)
    } else {
      label.textContent = node.name
    }
    row.appendChild(label)

    li.appendChild(row)
    li.style.paddingLeft = `calc(var(--tv-depth) * 1rem)`
    return li
  }

  _findItem(path) {
    return this.root.querySelector(`[data-path="${escapeSelector(path)}"]`)
  }

  destroy() {
    this.container.innerHTML = ''
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm run test:unit`

Expected: 3 passing tests.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "Add TreeView component: construction + root load"
```

---

## Task 3: TreeView — expand, collapse, toggle

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
const nested = {
  '': [
    { name: 'a', path: 'a', isDir: true },
    { name: 'b', path: 'b', isDir: true },
    { name: 'readme.md', path: 'readme.md', isDir: false },
  ],
  'a': [
    { name: 'inner.md', path: 'a/inner.md', isDir: false },
  ],
  'b': [
    { name: 'deep', path: 'b/deep', isDir: true },
  ],
  'b/deep': [
    { name: 'x.md', path: 'b/deep/x.md', isDir: false },
  ],
}

describe('TreeView expand/collapse', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('expand(path) loads children, inserts rows, flips aria-expanded', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    expect(loader).toHaveBeenCalledWith('a')
    const row = container.querySelector('[data-path="a"]')
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
  })

  it('expand(path) is idempotent — no duplicate loader call, no duplicate rows', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    await tv.expand('a')
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
    expect(container.querySelectorAll('[data-path="a/inner.md"]').length).toBe(1)
  })

  it('concurrent expand(path) calls share one loader call', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await Promise.all([tv.expand('a'), tv.expand('a')])
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
  })

  it('collapse(path) removes DOM subtree but preserves model', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    tv.collapse('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeNull()
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-expanded')).toBe('false')
    // Model retained: re-expand does not re-fetch
    await tv.expand('a')
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
  })

  it('toggle(path) expands if collapsed, collapses if expanded', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.toggle('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
    await tv.toggle('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeNull()
  })

  it('emits tree:toggle on expand and collapse', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    const events = []
    container.addEventListener('tree:toggle', (e) => events.push(e.detail))
    await tv.expand('a')
    tv.collapse('a')
    expect(events).toEqual([
      { path: 'a', expanded: true },
      { path: 'a', expanded: false },
    ])
  })

  it('expand(path) emits tree:error when loader rejects and leaves state unchanged', async () => {
    const loader = vi.fn(async (path) => {
      if (path === '') return nested['']
      throw new Error('nope')
    })
    const tv = new TreeView(container, { loader })
    await tv.ready
    const errors = []
    container.addEventListener('tree:error', (e) => errors.push(e.detail))
    await tv.expand('a').catch(() => {})
    expect(errors.length).toBe(1)
    expect(errors[0].path).toBe('a')
    expect(errors[0].error.message).toBe('nope')
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-expanded')).toBe('false')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: failures — `tv.expand is not a function`, etc.

- [ ] **Step 3: Implement expand/collapse/toggle**

Edit `web/src/tree-view.js`. Add these methods to the `TreeView` class, placed after `_loadChildren`:

```js
  async expand(path) {
    if (this.expandedPaths.has(path)) return
    if (this.loadingPaths.has(path)) return this.loadingPaths.get(path).promise

    const li = this._findItem(path)
    if (!li || !li.classList.contains(DIR_CLASS)) return

    const entry = { pendingRefresh: false }
    const promise = this._doExpand(path, li, entry)
    entry.promise = promise
    this.loadingPaths.set(path, entry)

    try {
      await promise
    } finally {
      this.loadingPaths.delete(path)
      if (entry.pendingRefresh && this.expandedPaths.has(path)) {
        this.refresh(path)
      }
    }
  }

  async _doExpand(path, li, entry) {
    let nodes
    try {
      nodes = this.childrenByPath.has(path)
        ? this.childrenByPath.get(path).map((p) => this.nodesByPath.get(p)).filter(Boolean)
        : await this._loadChildren(path)
    } catch (err) {
      this.container.dispatchEvent(new CustomEvent('tree:error', { detail: { path, error: err } }))
      throw err
    }
    const level = Number(li.getAttribute('aria-level'))
    this._renderChildren(path, li, nodes, level)
    li.setAttribute('aria-expanded', 'true')
    this.expandedPaths.add(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: true } }))
  }

  collapse(path) {
    if (!this.expandedPaths.has(path)) return
    const li = this._findItem(path)
    if (!li) return
    const childUl = li.querySelector(':scope > ul.tv-group')
    if (childUl) childUl.remove()
    li.setAttribute('aria-expanded', 'false')
    this.expandedPaths.delete(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: false } }))
  }

  toggle(path) {
    return this.expandedPaths.has(path) ? this.collapse(path) : this.expand(path)
  }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass (original 3 + 7 new).

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: expand, collapse, toggle"
```

---

## Task 4: TreeView — select and focus

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView select', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('select(path) flips aria-selected and emits tree:select', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    tv.select('readme.md', { source: 'api' })
    const li = container.querySelector('[data-path="readme.md"]')
    expect(li.getAttribute('aria-selected')).toBe('true')
    expect(events).toEqual([{ path: 'readme.md', node: expect.any(Object), source: 'api' }])
  })

  it('select(path) clears previous selection', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('a')
    tv.select('readme.md')
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-selected')).toBe('false')
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('select with source=silent does not emit tree:select', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    tv.select('readme.md', { source: 'silent' })
    expect(events.length).toBe(0)
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('select sets keyboard focus (tabindex=0 on selected, -1 elsewhere)', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('readme.md')
    const selected = container.querySelector('[data-path="readme.md"]')
    const others = container.querySelectorAll('[role="treeitem"]:not([data-path="readme.md"])')
    expect(selected.getAttribute('tabindex')).toBe('0')
    for (const o of others) expect(o.getAttribute('tabindex')).toBe('-1')
  })

  it('select(null) clears selection and tabindex rests on first node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('readme.md')
    tv.select(null)
    expect(container.querySelectorAll('[aria-selected="true"]').length).toBe(0)
    const first = container.querySelector('[role="treeitem"]')
    expect(first.getAttribute('tabindex')).toBe('0')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: `tv.select is not a function`.

- [ ] **Step 3: Implement select + roving tabindex**

Edit `web/src/tree-view.js`. In the constructor, after the root node setup but before `this.ready = ...`, add:

```js
    this._updateRovingTabindex()
```

(This line will be added when `_updateRovingTabindex` is defined below; on first render the root has no children yet, but the bootstrap `_renderChildren` will call it at the end.)

Update `_renderChildren` to end with:

```js
    parentEl.appendChild(ul)
    this._updateRovingTabindex()
```

Add these methods to the class (after `toggle`):

```js
  select(path, options = {}) {
    const source = options.source ?? 'api'
    const prev = this.selectedPath ? this._findItem(this.selectedPath) : null
    if (prev) prev.setAttribute('aria-selected', 'false')

    if (path == null) {
      this.selectedPath = null
      this.focusedPath = null
      this._updateRovingTabindex()
      if (source !== 'silent') {
        this.container.dispatchEvent(new CustomEvent('tree:select', {
          detail: { path: null, node: null, source },
        }))
      }
      return
    }

    const li = this._findItem(path)
    if (!li) return
    li.setAttribute('aria-selected', 'true')
    this.selectedPath = path
    this.focusedPath = path
    this._updateRovingTabindex()

    if (source !== 'silent') {
      const node = this.nodesByPath.get(path) ?? null
      this.container.dispatchEvent(new CustomEvent('tree:select', {
        detail: { path, node, source },
      }))
    }
  }

  _updateRovingTabindex() {
    const items = this.root.querySelectorAll('[role="treeitem"]')
    let target = null
    if (this.focusedPath) target = this._findItem(this.focusedPath)
    if (!target && this.selectedPath) target = this._findItem(this.selectedPath)
    if (!target && items.length) target = items[0]
    for (const it of items) {
      it.setAttribute('tabindex', it === target ? '0' : '-1')
    }
  }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: select + roving tabindex"
```

---

## Task 5: TreeView — refresh and reconciliation

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView refresh / reconciliation', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('refresh adds new children from loader', async () => {
    const state = { '': [{ name: 'a.md', path: 'a.md', isDir: false }] }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    state[''].push({ name: 'b.md', path: 'b.md', isDir: false })
    await tv.refresh('')
    expect(container.querySelector('[data-path="b.md"]')).toBeTruthy()
  })

  it('refresh removes deleted children and drops their model', async () => {
    const state = {
      '': [{ name: 'x.md', path: 'x.md', isDir: false }, { name: 'y.md', path: 'y.md', isDir: false }],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    state[''] = [{ name: 'x.md', path: 'x.md', isDir: false }]
    await tv.refresh('')
    expect(container.querySelector('[data-path="y.md"]')).toBeNull()
    expect(tv.nodesByPath.has('y.md')).toBe(false)
  })

  it('refresh preserves unchanged-and-expanded descendants', async () => {
    const state = {
      '': [
        { name: 'a', path: 'a', isDir: true },
        { name: 'b', path: 'b', isDir: true },
      ],
      'a': [{ name: 'a.md', path: 'a/a.md', isDir: false }],
      'b': [{ name: 'b.md', path: 'b/b.md', isDir: false }],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    await tv.expand('b')

    // Add a new sibling at root; a and b both survive, their subtrees untouched
    state[''].unshift({ name: 'c', path: 'c', isDir: true })
    state['c'] = []
    await tv.refresh('')

    expect(container.querySelector('[data-path="c"]')).toBeTruthy()
    expect(container.querySelector('[data-path="a/a.md"]')).toBeTruthy()
    expect(container.querySelector('[data-path="b/b.md"]')).toBeTruthy()
    expect(tv.expandedPaths.has('a')).toBe(true)
    expect(tv.expandedPaths.has('b')).toBe(true)
  })

  it('refresh clears selection if selected node is removed', async () => {
    const state = {
      '': [
        { name: 'x.md', path: 'x.md', isDir: false },
        { name: 'y.md', path: 'y.md', isDir: false },
      ],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    tv.select('y.md')
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    state[''] = [{ name: 'x.md', path: 'x.md', isDir: false }]
    await tv.refresh('')
    expect(tv.selectedPath).toBeNull()
    expect(events.some((e) => e.path === null)).toBe(true)
  })

  it('refresh replaces a node whose isDir flipped', async () => {
    const state = { '': [{ name: 'thing', path: 'thing', isDir: true }], 'thing': [] }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('thing')
    expect(tv.expandedPaths.has('thing')).toBe(true)

    state[''] = [{ name: 'thing', path: 'thing', isDir: false }]
    await tv.refresh('')
    const row = container.querySelector('[data-path="thing"]')
    expect(row.classList.contains(FILE_CLASS)).toBe(true)
    expect(tv.expandedPaths.has('thing')).toBe(false)
  })
})

// FILE_CLASS is not exported; re-declare for the test via literal:
const FILE_CLASS = 'tv-item--file'
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: `tv.refresh is not a function`.

- [ ] **Step 3: Implement refresh/reconciliation**

Edit `web/src/tree-view.js`. Add the `refresh` method and helpers (after `_doExpand`):

```js
  async refresh(path) {
    if (path !== this.rootPath && !this.expandedPaths.has(path)) return
    if (this.loadingPaths.has(path)) {
      this.loadingPaths.get(path).pendingRefresh = true
      return this.loadingPaths.get(path).promise
    }

    let nodes
    try {
      nodes = await this.loader(path)
    } catch (err) {
      this.container.dispatchEvent(new CustomEvent('tree:error', { detail: { path, error: err } }))
      return
    }

    this._reconcile(path, nodes)
  }

  _reconcile(parentPath, nextNodes) {
    const prev = this.childrenByPath.get(parentPath) ?? []
    const next = nextNodes.map((n) => n.path)
    const nextSet = new Set(next)
    const prevSet = new Set(prev)

    // Remove paths that disappeared
    for (const p of prev) {
      if (!nextSet.has(p)) this._removeSubtree(p)
    }

    // Handle isDir flip for surviving paths
    for (const n of nextNodes) {
      const existing = this.nodesByPath.get(n.path)
      if (existing && existing.isDir !== n.isDir) {
        this._removeSubtree(n.path, { keepEntry: false })
      }
    }

    // Find the parent UL
    const parentUl = this._childUl(parentPath)
    if (!parentUl) return

    // Build or reuse rows in `next` order
    const level = parentPath === this.rootPath ? 1 : Number(this._findItem(parentPath).getAttribute('aria-level')) + 1

    let cursor = null
    for (const node of nextNodes) {
      let li = this._findItem(node.path)
      const isNew = !li || !prevSet.has(node.path)
      if (isNew) {
        li = this._buildItem(node, level)
      }
      this.nodesByPath.set(node.path, node)
      // Insert in order
      if (cursor === null) {
        parentUl.insertBefore(li, parentUl.firstChild)
      } else {
        cursor.after(li)
      }
      cursor = li
    }

    this.childrenByPath.set(parentPath, next)

    // Selection cleanup
    if (this.selectedPath && !this._findItem(this.selectedPath)) {
      this.selectedPath = null
      this.focusedPath = null
      this._updateRovingTabindex()
      this.container.dispatchEvent(new CustomEvent('tree:select', {
        detail: { path: null, node: null, source: 'api' },
      }))
    } else {
      this._updateRovingTabindex()
    }
  }

  _childUl(path) {
    if (path === this.rootPath) {
      return this.root.querySelector(':scope > ul.tv-group')
    }
    const li = this._findItem(path)
    return li ? li.querySelector(':scope > ul.tv-group') : null
  }

  _removeSubtree(path, { keepEntry = false } = {}) {
    // Recursively drop model entries for `path` and its descendants
    const descendants = this.childrenByPath.get(path) ?? []
    for (const d of descendants) this._removeSubtree(d, { keepEntry: false })
    this.childrenByPath.delete(path)
    this.expandedPaths.delete(path)
    if (!keepEntry) this.nodesByPath.delete(path)
    // Drop DOM
    const li = this._findItem(path)
    if (li) li.remove()
  }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: refresh and reconciliation"
```

---

## Task 6: TreeView — refresh-during-load queue

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

The `expand` path already sets `pendingRefresh` on the loadingPaths entry. This task wires refresh to set the flag and ensures a follow-up fetch happens after the initial load completes.

- [ ] **Step 1: Write the failing test**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView refresh-during-load queue', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('refresh while expand is in flight fires exactly one follow-up fetch', async () => {
    let resolveFirst
    const firstPromise = new Promise((r) => { resolveFirst = r })
    const calls = []
    const loader = vi.fn(async (path) => {
      calls.push(path)
      if (path === '') return [{ name: 'a', path: 'a', isDir: true }]
      if (path === 'a' && calls.filter((c) => c === 'a').length === 1) {
        await firstPromise
        return [{ name: 'x.md', path: 'a/x.md', isDir: false }]
      }
      return [{ name: 'y.md', path: 'a/y.md', isDir: false }]
    })
    const tv = new TreeView(container, { loader })
    await tv.ready

    const expandP = tv.expand('a')
    // While the first load is blocked, ask for a refresh
    const refreshP = tv.refresh('a')
    resolveFirst()
    await Promise.all([expandP, refreshP])
    // Allow the queued follow-up to complete
    await new Promise((r) => setTimeout(r, 0))

    expect(calls.filter((c) => c === 'a').length).toBe(2)
    expect(container.querySelector('[data-path="a/y.md"]')).toBeTruthy()
  })

  it('multiple refreshes during a single in-flight load coalesce to one follow-up', async () => {
    let resolveFirst
    const firstPromise = new Promise((r) => { resolveFirst = r })
    const calls = []
    const loader = vi.fn(async (path) => {
      calls.push(path)
      if (path === '') return [{ name: 'a', path: 'a', isDir: true }]
      if (calls.filter((c) => c === 'a').length === 1) {
        await firstPromise
        return [{ name: 'x.md', path: 'a/x.md', isDir: false }]
      }
      return [{ name: 'z.md', path: 'a/z.md', isDir: false }]
    })
    const tv = new TreeView(container, { loader })
    await tv.ready
    const p = tv.expand('a')
    tv.refresh('a')
    tv.refresh('a')
    tv.refresh('a')
    resolveFirst()
    await p
    await new Promise((r) => setTimeout(r, 0))
    expect(calls.filter((c) => c === 'a').length).toBe(2)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: tests fail — loader for 'a' called only once because the current `refresh` bails when loadingPaths has the path (it returns the in-flight promise but doesn't queue).

Wait — looking at Task 5's `refresh`, it DOES set `pendingRefresh = true`. And Task 3's `expand` does check `entry.pendingRefresh` and call `refresh(path)`. So this should already work. Verify by running the tests.

Expected actual: likely passes. If so, skip to commit. If the tests fail, add the missing queue behavior below.

- [ ] **Step 3: (If needed) Fix the queue behavior**

If the tests fail, the cause is typically that `refresh` doesn't return the in-flight promise, or the follow-up isn't scheduled after `_doExpand`. The expected implementation is already in Task 3 and Task 5; if a bug exists, patch by ensuring the `finally` block in `expand` checks `pendingRefresh` AFTER `loadingPaths.delete(path)` and before returning.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: refresh-during-load queue (coverage)"
```

---

## Task 7: TreeView — persistence and initial.selectedPath

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView persistence', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
    localStorage.clear()
  })

  it('persists expanded + selected on change', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    await tv.expand('a')
    tv.select('readme.md')
    const saved = JSON.parse(localStorage.getItem('tv'))
    expect(saved.version).toBe(1)
    expect(saved.expanded).toEqual(['a'])
    expect(saved.selected).toBe('readme.md')
  })

  it('bootstrap restores expanded + selected from localStorage', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a'], selected: 'readme.md' }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('invalid JSON in storage is ignored', async () => {
    localStorage.setItem('tv', '{not json')
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(tv.expandedPaths.size).toBe(0)
  })

  it('stale expanded paths are pruned and re-persisted', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a', 'ghost'], selected: null }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    const saved = JSON.parse(localStorage.getItem('tv'))
    expect(saved.expanded).toEqual(['a'])
  })

  it('initial.selectedPath wins over persisted selected and merges expansion with ancestors', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a'], selected: 'readme.md' }))
    const tv = new TreeView(container, {
      loader: makeLoader(nested),
      persistKey: 'tv',
      initial: { selectedPath: 'b/deep/x.md' },
    })
    await tv.ready
    // b and b/deep are expanded because they are ancestors of the selected path;
    // a is also expanded because localStorage said so.
    expect(tv.expandedPaths.has('a')).toBe(true)
    expect(tv.expandedPaths.has('b')).toBe(true)
    expect(tv.expandedPaths.has('b/deep')).toBe(true)
    expect(tv.selectedPath).toBe('b/deep/x.md')
    expect(container.querySelector('[data-path="b/deep/x.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('initial.selectedPath applied silently does not emit tree:select', async () => {
    const tv = new TreeView(container, {
      loader: makeLoader(nested),
      initial: { selectedPath: 'readme.md' },
    })
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    await tv.ready
    expect(events.length).toBe(0)
    expect(tv.selectedPath).toBe('readme.md')
  })

  it('wrong version is dropped', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 99, expanded: ['a'], selected: 'x' }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(tv.expandedPaths.size).toBe(0)
    expect(tv.selectedPath).toBeNull()
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: persistence tests fail (no such behavior yet).

- [ ] **Step 3: Implement persistence + initial merging**

Edit `web/src/tree-view.js`:

Add to the constructor's stored options (before `this.ready = ...`):

```js
    this.persistKey = options.persistKey
    this.initial = options.initial ?? null
```

Replace `this.ready = this._bootstrap()` with the same line (no change; just the method itself changes).

Replace `_bootstrap` with:

```js
  async _bootstrap() {
    await this._loadChildren(this.rootPath)
    this._renderChildren(this.rootPath, this.root, this._nodesAt(this.rootPath), 0)

    const stored = this._readStorage()
    const initialSelected = this.initial?.selectedPath ?? null
    const initialExpanded = initialSelected ? this._ancestors(initialSelected) : []
    const fromStorage = stored?.expanded ?? []
    const toExpand = Array.from(new Set([...fromStorage, ...initialExpanded]))
      .sort((a, b) => a.split('/').length - b.split('/').length)

    const pruned = []
    for (const p of toExpand) {
      try {
        await this.expand(p)
        pruned.push(p)
      } catch (_) {
        // 404 / missing — drop silently
      }
    }

    const selected = initialSelected ?? stored?.selected ?? null
    if (selected) {
      const li = this._findItem(selected)
      if (li) this.select(selected, { source: 'silent' })
    }

    // If storage had stale paths, re-persist the cleaned set
    if (this.persistKey && stored && pruned.length !== fromStorage.length) {
      this._writeStorage()
    }
  }

  _nodesAt(parentPath) {
    const paths = this.childrenByPath.get(parentPath) ?? []
    return paths.map((p) => this.nodesByPath.get(p)).filter(Boolean)
  }

  _ancestors(path) {
    const parts = path.split('/')
    const out = []
    for (let i = 1; i < parts.length; i++) {
      out.push(parts.slice(0, i).join('/'))
    }
    return out
  }

  _readStorage() {
    if (!this.persistKey) return null
    try {
      const raw = localStorage.getItem(this.persistKey)
      if (!raw) return null
      const parsed = JSON.parse(raw)
      if (!parsed || parsed.version !== 1) return null
      return parsed
    } catch (_) {
      return null
    }
  }

  _writeStorage() {
    if (!this.persistKey) return
    const payload = {
      version: 1,
      expanded: Array.from(this.expandedPaths),
      selected: this.selectedPath,
    }
    try {
      localStorage.setItem(this.persistKey, JSON.stringify(payload))
    } catch (_) {}
  }
```

Then wire `_writeStorage` to state transitions. Add a single line `this._writeStorage()` at the end of:
- `_doExpand` (after `dispatchEvent tree:toggle`)
- `collapse` (after `dispatchEvent`)
- `select` (after `dispatchEvent`, inside the normal path AND the `path == null` branch)
- `_reconcile` (end of method)

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: persistence and initial.selectedPath bootstrap"
```

---

## Task 8: TreeView — keyboard navigation (arrows, Home, End)

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView keyboard — arrows, Home, End', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  function press(el, key) {
    const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    el.dispatchEvent(ev)
    return ev
  }

  it('ArrowDown moves focus to next visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const first = container.querySelector('[data-path="a"]')
    first.focus()
    press(first, 'ArrowDown')
    expect(tv.focusedPath).toBe('b')
    expect(container.querySelector('[data-path="b"]').getAttribute('tabindex')).toBe('0')
  })

  it('ArrowUp moves focus to previous visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const readme = container.querySelector('[data-path="readme.md"]')
    readme.focus()
    press(readme, 'ArrowUp')
    expect(tv.focusedPath).toBe('b')
  })

  it('ArrowRight on collapsed dir expands it', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowRight')
    await new Promise((r) => setTimeout(r, 0))
    expect(tv.expandedPaths.has('a')).toBe(true)
    // Focus stays on a
    expect(tv.focusedPath).toBe('a')
  })

  it('ArrowRight on expanded dir moves focus to first child', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowRight')
    expect(tv.focusedPath).toBe('a/inner.md')
  })

  it('ArrowLeft on expanded dir collapses it', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowLeft')
    expect(tv.expandedPaths.has('a')).toBe(false)
    expect(tv.focusedPath).toBe('a')
  })

  it('ArrowLeft on child moves focus to parent', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const inner = container.querySelector('[data-path="a/inner.md"]')
    inner.focus()
    press(inner, 'ArrowLeft')
    expect(tv.focusedPath).toBe('a')
  })

  it('Home focuses the first visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const readme = container.querySelector('[data-path="readme.md"]')
    readme.focus()
    press(readme, 'Home')
    expect(tv.focusedPath).toBe('a')
  })

  it('End focuses the last visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'End')
    expect(tv.focusedPath).toBe('readme.md')
  })

  it('arrow keys preventDefault to stop page scroll', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    const ev = press(a, 'ArrowDown')
    expect(ev.defaultPrevented).toBe(true)
  })

  it('exactly one treeitem has tabindex=0 at all times', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const first = container.querySelector('[data-path="a"]')
    first.focus()
    press(first, 'ArrowDown')
    const zeros = container.querySelectorAll('[role="treeitem"][tabindex="0"]')
    expect(zeros.length).toBe(1)
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: keyboard tests fail — no keydown handler yet.

- [ ] **Step 3: Implement keyboard navigation**

Edit `web/src/tree-view.js`. In the constructor, after `this.container.appendChild(this.root)` and before `this.ready = ...`, add:

```js
    this._onKeydown = (e) => this._handleKeydown(e)
    this.root.addEventListener('keydown', this._onKeydown)
```

In `destroy()`, remove the listener:

```js
  destroy() {
    this.root.removeEventListener('keydown', this._onKeydown)
    this.container.innerHTML = ''
  }
```

Add these methods to the class:

```js
  _handleKeydown(e) {
    const li = e.target.closest('[role="treeitem"]')
    if (!li) return
    const path = li.getAttribute('data-path')

    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); this._focusRelative(path, 1); return
      case 'ArrowUp':   e.preventDefault(); this._focusRelative(path, -1); return
      case 'ArrowRight': e.preventDefault(); this._arrowRight(path, li); return
      case 'ArrowLeft':  e.preventDefault(); this._arrowLeft(path, li); return
      case 'Home': e.preventDefault(); this._focusEdge('first'); return
      case 'End':  e.preventDefault(); this._focusEdge('last'); return
    }
  }

  _visibleItems() {
    return Array.from(this.root.querySelectorAll('[role="treeitem"]'))
  }

  _focusPath(path) {
    if (!path) return
    const li = this._findItem(path)
    if (!li) return
    this.focusedPath = path
    this._updateRovingTabindex()
    li.focus()
  }

  _focusRelative(fromPath, delta) {
    const items = this._visibleItems()
    const idx = items.findIndex((it) => it.getAttribute('data-path') === fromPath)
    if (idx === -1) return
    const next = items[idx + delta]
    if (next) this._focusPath(next.getAttribute('data-path'))
  }

  _arrowRight(path, li) {
    if (!li.classList.contains(DIR_CLASS)) return
    if (!this.expandedPaths.has(path)) {
      this.expand(path)
      return
    }
    // Already expanded: focus first child
    const childUl = li.querySelector(':scope > ul.tv-group')
    const firstChild = childUl?.querySelector('[role="treeitem"]')
    if (firstChild) this._focusPath(firstChild.getAttribute('data-path'))
  }

  _arrowLeft(path, li) {
    if (li.classList.contains(DIR_CLASS) && this.expandedPaths.has(path)) {
      this.collapse(path)
      return
    }
    // Focus parent
    const parent = li.parentElement?.closest('[role="treeitem"]')
    if (parent) this._focusPath(parent.getAttribute('data-path'))
  }

  _focusEdge(which) {
    const items = this._visibleItems()
    if (!items.length) return
    const target = which === 'first' ? items[0] : items[items.length - 1]
    this._focusPath(target.getAttribute('data-path'))
  }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: keyboard navigation (arrows, Home, End)"
```

---

## Task 9: TreeView — Enter/Space, typeahead, click handlers

**Files:**
- Modify: `web/src/tree-view.js`
- Modify: `web/src/tree-view.test.js`

- [ ] **Step 1: Write the failing tests**

Append to `web/src/tree-view.test.js`:

```js
describe('TreeView keyboard — activation and typeahead', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  function press(el, key) {
    const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    el.dispatchEvent(ev)
    return ev
  }

  it('Enter on focused node emits tree:select with source=keyboard', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    const li = container.querySelector('[data-path="readme.md"]')
    li.focus()
    press(li, 'Enter')
    expect(events.length).toBe(1)
    expect(events[0].path).toBe('readme.md')
    expect(events[0].source).toBe('keyboard')
  })

  it('Space selects like Enter and preventDefaults', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const li = container.querySelector('[data-path="readme.md"]')
    li.focus()
    const ev = press(li, ' ')
    expect(ev.defaultPrevented).toBe(true)
    expect(tv.selectedPath).toBe('readme.md')
  })

  it('typeahead focuses first matching visible node by name prefix', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const first = container.querySelector('[data-path="a"]')
    first.focus()
    press(first, 'r')  // matches readme.md
    expect(tv.focusedPath).toBe('readme.md')
  })

  it('typeahead buffer accumulates and resets after idle', async () => {
    const state = {
      '': [
        { name: 'alpha.md', path: 'alpha.md', isDir: false },
        { name: 'apple.md', path: 'apple.md', isDir: false },
      ],
    }
    const tv = new TreeView(container, { loader: makeLoader(state) })
    await tv.ready
    const first = container.querySelector('[data-path="alpha.md"]')
    first.focus()
    press(first, 'a')
    press(first, 'p')  // buffer is "ap" → matches apple.md
    expect(tv.focusedPath).toBe('apple.md')
    // After idle timeout, buffer resets
    vi.advanceTimersByTime(600)
    press(first, 'a')  // buffer = "a" → matches alpha.md (first "a...")
    expect(tv.focusedPath).toBe('alpha.md')
  })
})

describe('TreeView mouse clicks', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('clicking the row label selects the node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    const label = container.querySelector('[data-path="readme.md"] .tv-label')
    label.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(events.length).toBe(1)
    expect(events[0].path).toBe('readme.md')
    expect(events[0].source).toBe('click')
  })

  it('clicking the chevron toggles without selecting', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const selectEvents = []
    container.addEventListener('tree:select', (e) => selectEvents.push(e.detail))
    const chev = container.querySelector('[data-path="a"] .tv-toggle')
    chev.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    await new Promise((r) => setTimeout(r, 0))
    expect(tv.expandedPaths.has('a')).toBe(true)
    expect(selectEvents.length).toBe(0)
  })

  it('clicking the row label sets focus to the clicked node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const label = container.querySelector('[data-path="readme.md"] .tv-label')
    label.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    expect(tv.focusedPath).toBe('readme.md')
  })

  it('clicking the chevron does not move focus', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const firstItem = container.querySelector('[data-path="a"]')
    firstItem.focus()
    tv.focusedPath = 'a'
    const chevB = container.querySelector('[data-path="b"] .tv-toggle')
    chevB.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
    await new Promise((r) => setTimeout(r, 0))
    expect(tv.focusedPath).toBe('a')
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test:unit`

Expected: all new tests fail.

- [ ] **Step 3: Implement Enter/Space, typeahead, click**

Edit `web/src/tree-view.js`.

In the constructor, after the `keydown` listener:

```js
    this._typeaheadBuffer = ''
    this._typeaheadTimer = null
    this._onClick = (e) => this._handleClick(e)
    this.root.addEventListener('click', this._onClick)
```

Update `destroy`:

```js
  destroy() {
    this.root.removeEventListener('keydown', this._onKeydown)
    this.root.removeEventListener('click', this._onClick)
    this.container.innerHTML = ''
  }
```

Extend `_handleKeydown`'s switch with Enter/Space cases and add a default branch for typeahead:

```js
  _handleKeydown(e) {
    const li = e.target.closest('[role="treeitem"]')
    if (!li) return
    const path = li.getAttribute('data-path')

    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); this._focusRelative(path, 1); return
      case 'ArrowUp':   e.preventDefault(); this._focusRelative(path, -1); return
      case 'ArrowRight': e.preventDefault(); this._arrowRight(path, li); return
      case 'ArrowLeft':  e.preventDefault(); this._arrowLeft(path, li); return
      case 'Home': e.preventDefault(); this._focusEdge('first'); return
      case 'End':  e.preventDefault(); this._focusEdge('last'); return
      case 'Enter':
      case ' ':
        e.preventDefault()
        this.select(path, { source: 'keyboard' })
        return
    }

    // Typeahead: single printable character, no modifier keys
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault()
      this._typeahead(e.key.toLowerCase())
    }
  }

  _typeahead(ch) {
    this._typeaheadBuffer += ch
    if (this._typeaheadTimer) clearTimeout(this._typeaheadTimer)
    this._typeaheadTimer = setTimeout(() => { this._typeaheadBuffer = '' }, 500)

    const items = this._visibleItems()
    const startIdx = Math.max(0, items.findIndex((it) => it.getAttribute('data-path') === this.focusedPath))
    const ordered = items.slice(startIdx).concat(items.slice(0, startIdx))
    for (const it of ordered) {
      const p = it.getAttribute('data-path')
      const node = this.nodesByPath.get(p)
      if (node && node.name.toLowerCase().startsWith(this._typeaheadBuffer)) {
        this._focusPath(p)
        return
      }
    }
  }

  _handleClick(e) {
    const toggle = e.target.closest('.tv-toggle')
    if (toggle) {
      const li = toggle.closest('[role="treeitem"]')
      if (li) this.toggle(li.getAttribute('data-path'))
      return
    }
    const li = e.target.closest('[role="treeitem"]')
    if (li) this.select(li.getAttribute('data-path'), { source: 'click' })
  }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm run test:unit`

Expected: all tests pass.

- [ ] **Step 5: Add scrollTo method for completeness**

Append to the class:

```js
  scrollTo(path, options = {}) {
    const li = this._findItem(path)
    if (!li) return
    const block = options.block ?? 'center'
    li.scrollIntoView({ block, inline: 'nearest' })
  }
```

- [ ] **Step 6: Verify tests still pass, then commit**

Run: `npm run test:unit`

Expected: all tests pass.

```bash
git add web/src/tree-view.js web/src/tree-view.test.js
git commit -m "TreeView: Enter/Space, typeahead, click handlers, scrollTo"
```

---

## Task 10: Backend — `/api/tree/list` endpoint

**Files:**
- Create: `internal/server/tree_api.go`
- Create: `internal/server/tree_api_test.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/tree_api_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTreeListRoot(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	var nodes []struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &nodes); err != nil {
		t.Fatalf("invalid JSON: %v, body: %s", err, w.Body.String())
	}
	// Fixture root has "2026" dir and "README.md" file
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	// Dirs first, then files
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(nodes))
	}
	if !nodes[0].IsDir {
		t.Errorf("first entry should be a directory, got %+v", nodes[0])
	}
	if nodes[len(nodes)-1].IsDir {
		t.Errorf("last entry should be a file, got %+v", nodes[len(nodes)-1])
	}
	if nodes[0].Path != nodes[0].Name {
		t.Errorf("root-level path should equal name, got path=%q name=%q", nodes[0].Path, nodes[0].Name)
	}
}

func TestTreeListSubdir(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var nodes []struct {
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) == 0 {
		t.Fatal("expected entries under 2026")
	}
	for _, n := range nodes {
		if !strings.HasPrefix(n.Path, "2026/") {
			t.Errorf("child path should be prefixed with parent: %q", n.Path)
		}
	}
}

func TestTreeListPathEscaped(t *testing.T) {
	// Path with slashes encoded as %2F
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=2026%2F03", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var nodes []struct{ Path string `json:"path"` }
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) == 0 {
		t.Fatal("expected entries under 2026/03")
	}
	for _, n := range nodes {
		if !strings.HasPrefix(n.Path, "2026/03/") {
			t.Errorf("child path should be 2026/03/..., got %q", n.Path)
		}
	}
}

func TestTreeListNonexistent(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=nope", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestTreeListPathTraversal(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	// `..` is stripped by the rejectDirtyPaths middleware on the URL path,
	// but the query param path is its own surface — SafePath rejects `..`.
	req := httptest.NewRequest("GET", "/api/tree/list?path=../secrets", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTreeListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
	os.WriteFile(filepath.Join(dir, "empty", ".gitkeep"), nil, 0o644)
	srv, _ := NewServer(dir, "", nil)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/api/tree/list?path=empty", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("empty dir should return [], got %q", w.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestTreeList`

Expected: 404 on the route (`/api/tree/list` not registered).

- [ ] **Step 3: Implement the handler**

Create `internal/server/tree_api.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"os"
)

// treeNode is the JSON shape returned by /api/tree/list.
// Kept minimal and stable so the TreeView component's loader contract
// does not depend on IndexEntry internals.
type treeNode struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

func (s *Server) handleTreeList(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")

	absPath, err := SafePath(s.root, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries, err := readDirEntries(absPath, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nodes := make([]treeNode, 0, len(entries))
	for _, e := range entries {
		// IndexEntry.Href is /dir/X or /view/X; we want the relative path.
		// Reconstruct: href minus prefix, then URL-decoded. Simpler: compute
		// from Name + relPath.
		p := e.Name
		if relPath != "" {
			p = relPath + "/" + e.Name
		}
		nodes = append(nodes, treeNode{Name: e.Name, Path: p, IsDir: e.IsDir})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(nodes); err != nil {
		s.logger.Warn("tree list encode failed", "err", err)
	}
}
```

Modify `internal/server/server.go` `Routes()` to register the new handler. Add this line in the `mux.HandleFunc` block (after `/events`):

```go
	mux.HandleFunc("GET /api/tree/list", s.handleTreeList)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/`

Expected: all tests pass (existing + new).

- [ ] **Step 5: Commit**

```bash
git add internal/server/tree_api.go internal/server/tree_api_test.go internal/server/server.go
git commit -m "Add /api/tree/list JSON endpoint"
```

---

## Task 11: Backend — SSEHub → EventHub rename (pure refactor)

**Files:**
- Rename: `internal/server/sse.go` → `internal/server/events.go`
- Rename: `internal/server/sse_test.go` → `internal/server/events_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers_test.go` (if it references SSEHub directly)

This task is behaviorally a no-op. It renames types and the file but keeps the wire contract intact (`/events?watch=X` still emits `event: change` on writes). Tests stay green.

- [ ] **Step 1: Rename the files via git**

```bash
git mv internal/server/sse.go internal/server/events.go
git mv internal/server/sse_test.go internal/server/events_test.go
```

- [ ] **Step 2: Rename types inside `events.go`**

In the new `internal/server/events.go`, apply these renames:
- `SSEHub` → `EventHub` (struct + all method receivers)
- `NewSSEHub` → `NewEventHub`
- `sseClient` → `Subscription`
- `clients` field name kept (still a set)
- `watchPath` field kept (still per-subscription)
- `events` field kept (still a `chan string`; Task 13 will generalize it)
- `handleSSE` (in `events.go` too — it's the HTTP handler) → `handleEvents`

The `*Server.handleSSE` method in `events.go` should also be renamed to `handleEvents`.

- [ ] **Step 3: Update `server.go`**

Rename the struct field and constructor call:

```go
type Server struct {
	// ...
	events    *EventHub
	// ...
}
```

And in `NewServer`:

```go
	events:    NewEventHub(root, logger, idx),
```

Update the routing line from:

```go
	mux.HandleFunc("GET /events", s.handleSSE)
```

to:

```go
	mux.HandleFunc("GET /events", s.handleEvents)
```

And update `StartWatcher`/`Shutdown` to use the new field name:

```go
func (s *Server) StartWatcher() error {
	return s.events.Start()
}

func (s *Server) Shutdown() {
	s.events.Stop()
}
```

- [ ] **Step 4: Update `events_test.go`**

Apply the renames across the test file: `NewSSEHub` → `NewEventHub`, `SSEHub` → `EventHub`, `sseClient` → `Subscription`, `handleSSE` → `handleEvents`. The test bodies otherwise stay identical.

- [ ] **Step 5: Run all tests**

Run: `go test ./...`

Expected: all tests pass, no behavioral change.

- [ ] **Step 6: Commit**

```bash
git add internal/server/events.go internal/server/events_test.go internal/server/server.go
git commit -m "Rename SSEHub to EventHub (pure refactor)"
```

---

## Task 12: Backend — dir-changed broadcast + recursive dir watching

**Files:**
- Modify: `internal/server/events.go`
- Modify: `internal/server/events_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/server/events_test.go`:

```go
func TestDirChangedBroadcast(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)

	hub := NewEventHub(dir, nil, nil)
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	sub := &Subscription{watchPath: "", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	// Create a new file inside a watched dir.
	if err := os.WriteFile(filepath.Join(dir, "a", "new.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-sub.events:
		if msg.kind != "dir-changed" {
			t.Errorf("kind = %q, want dir-changed", msg.kind)
		}
		if msg.path != "a" {
			t.Errorf("path = %q, want a", msg.path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for dir-changed")
	}
}

func TestDirChangedOnNewSubdir(t *testing.T) {
	dir := t.TempDir()

	hub := NewEventHub(dir, nil, nil)
	hub.Start()
	defer hub.Stop()

	sub := &Subscription{watchPath: "", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	// Create a new subdir at root; should fire dir-changed for "".
	if err := os.Mkdir(filepath.Join(dir, "newdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-sub.events:
			if msg.kind == "dir-changed" && msg.path == "" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for dir-changed at root")
		}
	}
}

func TestDirChangedEndpointEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)

	hub := NewEventHub(dir, nil, nil)
	hub.Start()
	defer hub.Stop()

	srv := &Server{root: dir, events: hub}

	req := httptest.NewRequest("GET", "/events", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.handleEvents(w, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "a", "n.md"), []byte("x"), 0o644)

	<-done
	body := w.Body.String()
	if !strings.Contains(body, "event: dir-changed") {
		t.Errorf("expected event: dir-changed in body, got:\n%s", body)
	}
}

func TestChangeEventStillDelivered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.md"), []byte("a"), 0o644)

	hub := NewEventHub(dir, nil, nil)
	hub.Start()
	defer hub.Stop()

	sub := &Subscription{watchPath: "x.md", events: make(chan eventMsg, 4)}
	hub.addClient(sub)
	defer hub.removeClient(sub)

	os.WriteFile(filepath.Join(dir, "x.md"), []byte("b"), 0o644)

	deadline := time.After(3 * time.Second)
	gotChange := false
	for !gotChange {
		select {
		case msg := <-sub.events:
			if msg.kind == "change" && msg.path == "x.md" {
				gotChange = true
			}
		case <-deadline:
			t.Fatal("change event not delivered")
		}
	}
}
```

Also, replace the existing test bodies in `events_test.go` that create a `Subscription` with string channels — they need updating because the channel type changes from `chan string` to `chan eventMsg`. Specifically:

In `TestSSEMultiClientBroadcast`, `TestSSESelectiveBroadcast`, `TestSSEPerPathDebounce`, `TestSSENonBlockingSend` (now renamed to reflect the new type names — keep the names for continuity with git history), change:

```go
c := &Subscription{watchPath: "x.md", events: make(chan string, 1)}
```

to:

```go
c := &Subscription{watchPath: "x.md", events: make(chan eventMsg, 1)}
```

and assertions that receive from the channel:

```go
case p := <-c1.events:
    if p != "shared.md" { ... }
```

to:

```go
case msg := <-c1.events:
    if msg.path != "shared.md" || msg.kind != "change" { ... }
```

Rename for accuracy: `TestSSE...` → `TestEvents...` (since they no longer test SSE-specific types; the test file is `events_test.go`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/`

Expected: compilation errors and/or test failures.

- [ ] **Step 3: Implement the event-type union and dir-changed broadcaster**

Replace `internal/server/events.go` contents with:

```go
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dreikanter/notes-view/internal/index"
	"github.com/dreikanter/notes-view/internal/logging"
	"github.com/fsnotify/fsnotify"
)

// eventMsg is the internal envelope passed to subscribers.
// kind is "change" (file content) or "dir-changed" (tree mutation).
type eventMsg struct {
	kind string
	path string
}

type EventHub struct {
	root    string
	logger  *slog.Logger
	index   *index.NoteIndex
	mu      sync.RWMutex
	clients map[*Subscription]struct{}
	watcher *fsnotify.Watcher
	done    chan struct{}
}

type Subscription struct {
	watchPath string // "" means no file-change subscription
	events    chan eventMsg
}

func NewEventHub(root string, logger *slog.Logger, idx *index.NoteIndex) *EventHub {
	if logger == nil {
		logger = logging.Discard()
	}
	return &EventHub{
		root:    root,
		logger:  logger,
		index:   idx,
		clients: make(map[*Subscription]struct{}),
		done:    make(chan struct{}),
	}
}

func (h *EventHub) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	h.watcher = watcher
	// Recursively watch every existing directory under root so we see
	// adds/removes at any depth. New dirs created later are added in
	// eventLoop as they appear.
	if err := h.watchRecursive(h.root); err != nil {
		h.logger.Warn("initial recursive watch failed", "err", err)
	}
	go h.eventLoop()
	return nil
}

func (h *EventHub) watchRecursive(absDir string) error {
	return filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if name := d.Name(); strings.HasPrefix(name, ".") && p != absDir {
				return filepath.SkipDir
			}
			if err := h.watcher.Add(p); err != nil {
				h.logger.Warn("watcher add failed", "dir", p, "err", err)
			}
		}
		return nil
	})
}

func (h *EventHub) Stop() {
	close(h.done)
	if h.watcher != nil {
		h.watcher.Close()
	}
}

func (h *EventHub) eventLoop() {
	changeTimers := make(map[string]*time.Timer)
	dirTimers := make(map[string]*time.Timer)

	for {
		select {
		case <-h.done:
			for _, t := range changeTimers {
				t.Stop()
			}
			for _, t := range dirTimers {
				t.Stop()
			}
			return
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			h.handleFSEvent(event, changeTimers, dirTimers)
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			h.logger.Warn("file watcher error", "err", err)
		}
	}
}

func (h *EventHub) handleFSEvent(event fsnotify.Event, changeTimers, dirTimers map[string]*time.Timer) {
	// A new subdir means we must watch it too.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if name := filepath.Base(event.Name); !strings.HasPrefix(name, ".") {
				if err := h.watcher.Add(event.Name); err != nil {
					h.logger.Warn("watcher add (new dir) failed", "dir", event.Name, "err", err)
				}
			}
		}
	}

	// File-content change: Write/Create on a .md file → 'change' broadcast.
	if event.Op&(fsnotify.Write|fsnotify.Create) != 0 &&
		strings.HasSuffix(strings.ToLower(event.Name), ".md") {
		p := event.Name
		if t, ok := changeTimers[p]; ok {
			t.Stop()
		}
		changeTimers[p] = time.AfterFunc(100*time.Millisecond, func() {
			if h.index != nil {
				<-h.index.Rebuild()
			}
			h.broadcastChange(p)
		})
	}

	// Dir mutation: Create/Remove/Rename inside a watched dir → 'dir-changed'
	// broadcast for the parent dir. Skip non-visible names (dotfiles, non-md
	// files) so we don't wake clients for editor swap files.
	if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		base := filepath.Base(event.Name)
		visible := !strings.HasPrefix(base, ".")
		if visible {
			// For files, only .md counts as "visible" per the tree contract.
			if info, err := os.Stat(event.Name); err == nil && !info.IsDir() {
				visible = strings.HasSuffix(strings.ToLower(base), ".md")
			}
		}
		if visible {
			parentAbs := filepath.Dir(event.Name)
			parentRel, err := filepath.Rel(h.root, parentAbs)
			if err == nil {
				if parentRel == "." {
					parentRel = ""
				}
				if t, ok := dirTimers[parentRel]; ok {
					t.Stop()
				}
				pr := parentRel
				dirTimers[pr] = time.AfterFunc(200*time.Millisecond, func() {
					h.broadcastDirChanged(pr)
				})
			}
		}
	}
}

func (h *EventHub) broadcastChange(absPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.clients {
		if sub.watchPath == "" {
			continue
		}
		safePath, err := SafePath(h.root, sub.watchPath)
		if err != nil || safePath != absPath {
			continue
		}
		select {
		case sub.events <- eventMsg{kind: "change", path: sub.watchPath}:
		default:
		}
	}
}

func (h *EventHub) broadcastDirChanged(relPath string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.clients {
		select {
		case sub.events <- eventMsg{kind: "dir-changed", path: relPath}:
		default:
		}
	}
}

func (h *EventHub) addClient(c *Subscription) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *EventHub) removeClient(c *Subscription) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	watchPath := r.URL.Query().Get("watch")
	if watchPath != "" {
		if _, err := SafePath(s.root, watchPath); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := &Subscription{
		watchPath: watchPath,
		events:    make(chan eventMsg, 8),
	}
	s.events.addClient(sub)
	defer s.events.removeClient(sub)

	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", toJSON(map[string]string{"type": "connected"}))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sub.events:
			switch msg.kind {
			case "change":
				fmt.Fprintf(w, "event: change\ndata: %s\n\n", toJSON(map[string]string{
					"type": "change",
					"path": msg.path,
				}))
			case "dir-changed":
				fmt.Fprintf(w, "event: dir-changed\ndata: %s\n\n", toJSON(map[string]string{
					"path": msg.path,
				}))
			}
			flusher.Flush()
		}
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/server/ -run TestDirChanged -v`
Run: `go test ./internal/server/ -run TestEvents -v`
Run: `go test ./internal/server/`

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/server/events.go internal/server/events_test.go
git commit -m "EventHub: recursive dir watching + dir-changed broadcast"
```

---

## Task 13: Backend — initial-state JSON in full-page responses

**Files:**
- Modify: `internal/server/templates.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/handlers_test.go`

The sidebar template will eventually become a placeholder with a `<script type="application/json" id="tv-initial">` containing the pre-selected path. This task wires the data pipeline (server emits the JSON, asserted via tests) while leaving the tree-rendering branch in the template for now. The template itself is updated in Task 15.

- [ ] **Step 1: Write failing tests**

Append to `internal/server/handlers_test.go`:

```go
func TestViewEmbedsInitialSelectedPath(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/view/README.md", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `id="tv-initial"`) {
		t.Errorf("expected tv-initial script tag, got: %s", body)
	}
	if !strings.Contains(body, `"selectedPath":"README.md"`) {
		t.Errorf("expected selectedPath=README.md in initial JSON, got: %s", body)
	}
}

func TestDirEmbedsInitialSelectedPath(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/dir/2026", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"selectedPath":"2026"`) {
		t.Errorf("expected selectedPath=2026 in initial JSON, got: %s", body)
	}
}

func TestRootEmbedsNullSelectedPath(t *testing.T) {
	// Server root without a README redirects; to test the empty-state branch,
	// use a directory with no README.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note.md"), []byte("x"), 0o644)
	srv, _ := NewServer(dir, "", nil)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"selectedPath":null`) {
		t.Errorf("expected selectedPath=null at empty root, got: %s", body)
	}
}

func TestTagsEmbedsNullSelectedPath(t *testing.T) {
	srv, _ := setupTestServer(t)
	handler := srv.Routes()

	req := httptest.NewRequest("GET", "/tags", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"selectedPath":null`) {
		t.Errorf("expected selectedPath=null on tags page, got: %s", body)
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./internal/server/ -run "TestViewEmbedsInitialSelectedPath|TestDirEmbedsInitialSelectedPath|TestRootEmbedsNullSelectedPath|TestTagsEmbedsNullSelectedPath"`

Expected: failures — the template doesn't emit `id="tv-initial"` yet.

- [ ] **Step 3: Update `templates.go` data shape**

Edit `internal/server/templates.go`. Update `SidebarPartialData` to carry the initial-state JSON:

```go
// SidebarPartialData is the render context for the sidebar tree.
type SidebarPartialData struct {
	Files       *IndexCard    // FILES section entries (rendered empty for now)
	Tags        *IndexCard    // TAGS section entries
	InitialJSON template.JS   // {"selectedPath": "<path>" | null} — consumed by TreeView
}
```

- [ ] **Step 4: Build the initial JSON in handlers**

In `internal/server/handlers.go`, add a helper near the top (after imports) that builds the JSON:

```go
// buildInitialJSON returns a pre-encoded JSON object that the TreeView
// component reads from the <script id="tv-initial"> on first render. The
// selectedPath drives ancestor pre-expansion and selection. Empty string
// means no selection (e.g., empty root, tags page).
func buildInitialJSON(selectedPath string) template.JS {
	payload := struct {
		SelectedPath *string `json:"selectedPath"`
	}{}
	if selectedPath != "" {
		payload.SelectedPath = &selectedPath
	}
	b, _ := json.Marshal(payload)
	return template.JS(b)
}
```

In the same file, update every place that constructs `SidebarPartialData` to set `InitialJSON`:

1. In `handleRoot` (the empty-state branch):

```go
	view := ViewData{
		layoutFields: lf,
		NotePath:     "",
		HTML:         template.HTML(`<p class="text-gray-500 text-center py-8">No note selected.</p>`),
		ViewHref:     "/",
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
	}
```

2. In `handleView` (full-page branch):

```go
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(reqPath),
		},
```

3. In `handleDir` (full-page branch):

```go
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(dirPath),
		},
```

4. In `handleTags` (full-page branch):

```go
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        card,
			InitialJSON: buildInitialJSON(""),
		},
```

5. In `handleTagNotes` (full-page branch):

```go
		Sidebar: SidebarPartialData{
			Files:       filesCard,
			Tags:        tagsCard,
			InitialJSON: buildInitialJSON(""),
		},
```

- [ ] **Step 5: Update `sidebar_tree.html` to emit the script tag**

Edit `web/templates/sidebar_tree.html`. Insert the `<script>` tag inside the files section, above the files content `<div>`:

```html
{{ define "sidebar_tree" }}
<script type="application/json" id="tv-initial">{{ .InitialJSON }}</script>
<section id="files-section">
  <button type="button" onclick="toggleSection('files')" aria-expanded="true" aria-controls="files-content" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="files-disclosure" class="text-[10px] leading-none">&#9662;</span>
    FILES
  </button>
  <div id="files-content" aria-hidden="false">
    {{ with .Files }}{{ template "entry_list" . }}{{ end }}
  </div>
</section>

<hr class="border-gray-200 m-0" />

<section id="tags-section">
  <button type="button" onclick="toggleSection('tags')" aria-expanded="true" aria-controls="tags-content" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="tags-disclosure" class="text-[10px] leading-none">&#9662;</span>
    TAGS
  </button>
  <div id="tags-content" aria-hidden="false">
    {{ with .Tags }}{{ template "entry_list" . }}{{ end }}
  </div>
</section>
{{ end }}
```

(The only change is the first line after `{{ define }}` — a new `<script>` tag.)

- [ ] **Step 6: Run all Go tests**

Run: `go test ./internal/server/`

Expected: all pass, including the four new tests.

- [ ] **Step 7: Commit**

```bash
git add internal/server/templates.go internal/server/handlers.go internal/server/handlers_test.go web/templates/sidebar_tree.html
git commit -m "Embed tv-initial JSON with selectedPath in full-page responses"
```

---

## Task 14: Client — `sidebar.js` application glue + template placeholder + `app.js` cleanup

**Files:**
- Create: `web/src/sidebar.js`
- Modify: `web/templates/sidebar_tree.html`
- Modify: `web/src/app.js`
- Modify: `web/src/index.html` (for Vite entry if needed)
- Modify: `web/templates/layout.html` (if needed for script loading)
- Modify: `tests/sidebar-tree.spec.ts`

This is the biggest task. It switches the sidebar to client-driven in one coherent change: the template becomes an empty placeholder with an initial-state script, the new `sidebar.js` instantiates `TreeView` and wires events, `app.js` loses its sidebar tree code, and the Playwright suite is rewritten for the new DOM. After this task, the sidebar renders and behaves correctly; subsequent tasks only remove dead code.

- [ ] **Step 1: Create `web/src/sidebar.js`**

```js
// Sidebar glue: wires the reusable TreeView component to notes-view
// navigation. Reads the server-embedded initial state, constructs the
// component with a fetch-based loader, and translates tree events into
// HTMX + history navigation. SSE from the unified /events endpoint
// keeps the tree in sync with filesystem mutations.

import { TreeView } from './tree-view.js'
import htmx from 'htmx.org'

function encodePath(p) {
  if (!p) return ''
  return p.split('/').map(encodeURIComponent).join('/')
}

function readInitial() {
  const el = document.getElementById('tv-initial')
  if (!el) return { selectedPath: null }
  try {
    return JSON.parse(el.textContent || '{}')
  } catch {
    return { selectedPath: null }
  }
}

function pathFromURL(pathname) {
  if (pathname.startsWith('/view/')) return decodeURIComponent(pathname.slice('/view/'.length))
  if (pathname.startsWith('/dir/')) return decodeURIComponent(pathname.slice('/dir/'.length))
  return null
}

function ancestorsOf(path) {
  if (!path) return []
  const parts = path.split('/')
  const out = []
  for (let i = 1; i < parts.length; i++) out.push(parts.slice(0, i).join('/'))
  return out
}

export function mountSidebar() {
  const container = document.getElementById('sidebar-tree')
  if (!container) return null

  const initial = readInitial()

  const tree = new TreeView(container, {
    loader: (p) => fetch('/api/tree/list?path=' + encodeURIComponent(p)).then((r) => {
      if (!r.ok) throw new Error('tree list failed: ' + r.status)
      return r.json()
    }),
    initial,
    persistKey: 'notesview.tree',
  })

  container.addEventListener('tree:select', (e) => {
    const { path, node } = e.detail
    if (!path || !node) return
    const href = node.isDir ? '/dir/' + encodePath(path) : '/view/' + encodePath(path)
    htmx.ajax('GET', href, {
      target: '#note-pane',
      swap: 'innerHTML',
      headers: { 'HX-Target': 'note-pane' },
    })
    history.pushState({ type: node.isDir ? 'dir' : 'note', href }, '', href)
  })

  const notePath = document.body.getAttribute('data-note-path') || ''
  const esURL = '/events' + (notePath ? '?watch=' + encodeURIComponent(notePath) : '')
  const es = new EventSource(esURL)
  es.addEventListener('dir-changed', (e) => {
    try {
      const { path } = JSON.parse(e.data)
      tree.refresh(path)
    } catch {}
  })

  window.addEventListener('popstate', async () => {
    const path = pathFromURL(location.pathname)
    if (!path) return
    for (const a of ancestorsOf(path)) {
      try { await tree.expand(a) } catch {}
    }
    tree.select(path, { source: 'silent' })
    tree.scrollTo(path)
  })

  return tree
}
```

- [ ] **Step 2: Replace the files section in `sidebar_tree.html` with an empty placeholder**

Edit `web/templates/sidebar_tree.html`. Replace the entire file contents with:

```html
{{ define "sidebar_tree" }}
<script type="application/json" id="tv-initial">{{ .InitialJSON }}</script>
<section id="files-section">
  <button type="button" onclick="toggleSection('files')" aria-expanded="true" aria-controls="files-content" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="files-disclosure" class="text-[10px] leading-none">&#9662;</span>
    FILES
  </button>
  <div id="files-content" aria-hidden="false">
    <div id="sidebar-tree"></div>
  </div>
</section>

<hr class="border-gray-200 m-0" />

<section id="tags-section">
  <button type="button" onclick="toggleSection('tags')" aria-expanded="true" aria-controls="tags-content" class="w-full flex items-center gap-1.5 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-gray-500 bg-gray-50 border-b border-gray-200 cursor-pointer hover:bg-gray-100 transition-colors">
    <span id="tags-disclosure" class="text-[10px] leading-none">&#9662;</span>
    TAGS
  </button>
  <div id="tags-content" aria-hidden="false">
    {{ with .Tags }}{{ template "entry_list" . }}{{ end }}
  </div>
</section>
{{ end }}
```

- [ ] **Step 3: Rewrite `web/src/app.js` to drop the tree logic and mount the sidebar**

Replace the entire contents of `web/src/app.js` with:

```js
// notesview front-end bootstrap.
//
// Loads HTMX + SSE support, runs syntax highlighting on every swap, owns
// sidebar toggle + section collapse, and delegates the sidebar tree to
// web/src/sidebar.js (a thin glue around the reusable TreeView module).

import htmx from 'htmx.org'
import 'htmx-ext-sse'
import hljs from 'highlight.js/lib/common'
import { mountSidebar } from './sidebar.js'

function highlightIn(root) {
  if (!root || !root.querySelectorAll) return
  root.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el)
  })
}

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document)
  wireSidebarToggle()
  restoreSidebarState()
  mountSidebar()
})

function wireSidebarToggle() {
  const btn = document.getElementById('sidebar-toggle')
  if (!btn) return
  const initiallyOpen = document.documentElement.classList.contains('sidebar-open')
  btn.setAttribute('aria-expanded', initiallyOpen ? 'true' : 'false')
  btn.addEventListener('click', toggleSidebar)
}

function toggleSidebar() {
  const root = document.documentElement
  const btn = document.getElementById('sidebar-toggle')
  const open = root.classList.toggle('sidebar-open')
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false')
  try {
    localStorage.setItem('notesview.sidebarOpen', open ? '1' : '0')
  } catch (e) {}
}

function getLS(key, fallback) {
  try { return localStorage.getItem('notesview.' + key) || fallback } catch (e) { return fallback }
}

function setLS(key, value) {
  try { localStorage.setItem('notesview.' + key, value) } catch (e) {}
}

// --- Section collapse/expand (FILES and TAGS headers) ---

window.toggleSection = function(name) {
  const content = document.getElementById(name + '-content')
  const disclosure = document.getElementById(name + '-disclosure')
  const btn = document.querySelector('[aria-controls="' + name + '-content"]')
  if (!content) return
  const isOpen = content.style.display !== 'none'
  content.style.display = isOpen ? 'none' : ''
  content.setAttribute('aria-hidden', isOpen ? 'true' : 'false')
  if (btn) btn.setAttribute('aria-expanded', isOpen ? 'false' : 'true')
  if (disclosure) disclosure.textContent = isOpen ? '\u25B8' : '\u25BE'
  setLS(name + 'Open', isOpen ? '0' : '1')
}

function restoreSectionState(name) {
  const open = getLS(name + 'Open', '1')
  const content = document.getElementById(name + '-content')
  const disclosure = document.getElementById(name + '-disclosure')
  const btn = document.querySelector('[aria-controls="' + name + '-content"]')
  if (!content) return
  if (open === '0') {
    content.style.display = 'none'
    content.setAttribute('aria-hidden', 'true')
    if (btn) btn.setAttribute('aria-expanded', 'false')
    if (disclosure) disclosure.textContent = '\u25B8'
  }
}

function restoreSidebarState() {
  restoreSectionState('files')
  restoreSectionState('tags')
}

// --- Tag navigation (flat list; not part of the tree component) ---

window.selectTag = function(tag, skipPush) {
  const href = '/tags/' + encodeURIComponent(tag)
  if (!skipPush) history.pushState({ type: 'tag', tag: tag, href: href }, '', href)
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  })
  const content = document.getElementById('tags-content')
  const disclosure = document.getElementById('tags-disclosure')
  if (content) content.style.display = ''
  if (disclosure) disclosure.textContent = '\u25BE'
  setLS('tagsOpen', '1')
}

// Tag click delegation (tree's own clicks are handled inside the component).
document.addEventListener('click', function(e) {
  const link = e.target.closest('[data-action="selectTag"]')
  if (!link) return
  e.preventDefault()
  selectTag(link.dataset.entryName, false)
})

// --- Browser back/forward for tag and unmatched routes ---

window.addEventListener('popstate', function(e) {
  const state = e.state
  if (state && state.type === 'tag') {
    selectTag(state.tag, true)
  }
  // Note: 'note' and 'dir' popstate handling lives in sidebar.js so the
  // tree expansion/selection is driven through the component's public API.
})

// --- Post-swap work ---

let pendingNoteScrollReset = false

document.body.addEventListener('htmx:beforeRequest', function(e) {
  const hdrs = e.detail && e.detail.requestConfig && e.detail.requestConfig.headers
  if (hdrs && hdrs['HX-Target'] === 'note-pane') {
    pendingNoteScrollReset = true
  }
})

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target)
  if (pendingNoteScrollReset && e.target && e.target.id === 'note-pane') {
    e.target.scrollTop = 0
    pendingNoteScrollReset = false
  }
})
```

- [ ] **Step 4: Confirm `index.html` and `layout.html` still reference a single entry**

The Vite build treats `web/src/index.html` as the HTML entry and bundles `./app.js`. `sidebar.js` is imported statically from `app.js`, so Vite will bundle it automatically. No config change needed.

Verify `web/templates/layout.html` still loads `/static/app.js` — no change required.

- [ ] **Step 5: Rewrite the Playwright tests**

Replace the entire contents of `tests/sidebar-tree.spec.ts` with:

```ts
import { test, expect } from '@playwright/test'

test.describe('Sidebar Tree (client-side TreeView)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
  })

  test('FILES and TAGS sections render', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#files-section')).toBeVisible()
    await expect(page.locator('#tags-section')).toBeVisible()
  })

  test('root tree populates from /api/tree/list', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await expect(files.locator('[data-path="journal"]')).toBeVisible()
    await expect(files.locator('[data-path="projects"]')).toBeVisible()
    await expect(files.locator('[data-path="README.md"]')).toBeVisible()
  })

  test('initial selectedPath reveals and selects the current note on reload', async ({ page }) => {
    await page.goto('/view/journal/day-one.md')
    await page.click('#sidebar-toggle')
    const target = page.locator('#sidebar-tree [data-path="journal/day-one.md"]')
    await expect(target).toBeVisible()
    await expect(target).toHaveAttribute('aria-selected', 'true')
    // Ancestor dir is expanded
    await expect(page.locator('#sidebar-tree [data-path="journal"]')).toHaveAttribute('aria-expanded', 'true')
  })

  test('chevron click expands and collapses without changing URL', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toBeVisible()
    await expect(page).toHaveURL(/\/view\/README\.md/)
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toHaveCount(0)
    await expect(page).toHaveURL(/\/view\/README\.md/)
  })

  test('chevron on one dir does not collapse another expanded dir', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await files.locator('[data-path="projects"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toBeVisible()
    await expect(files.locator('[data-path="projects/alpha.md"]')).toBeVisible()
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toHaveCount(0)
    await expect(files.locator('[data-path="projects/alpha.md"]')).toBeVisible()
  })

  test('clicking a note row opens it in the main panel and updates URL', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-toggle').click()
    await page.locator('#sidebar-tree [data-path="journal/day-one.md"] .tv-label').click()
    await expect(page.locator('#note-card')).toContainText('Day One')
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/)
  })

  test('clicking a directory row loads its listing', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-label').click()
    const listing = page.locator('#dir-listing')
    await expect(listing).toBeVisible()
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible()
  })

  test('tags section continues to work as a flat list', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#tags-content a', { hasText: 'daily' }).click()
    const listing = page.locator('#dir-listing')
    await expect(listing).toBeVisible()
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible()
  })

  test('reload preserves expanded and selected state', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-toggle').click()
    await page.locator('#sidebar-tree [data-path="journal/day-one.md"] .tv-label').click()
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/)
    await page.reload()
    const target = page.locator('#sidebar-tree [data-path="journal/day-one.md"]')
    await expect(target).toBeVisible()
    await expect(target).toHaveAttribute('aria-selected', 'true')
  })

  test('keyboard: ArrowDown moves focus between visible items', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const tree = page.locator('#sidebar-tree')
    await tree.locator('[data-path="journal"]').focus()
    await page.keyboard.press('ArrowDown')
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('data-path'))
    expect(focused).toBe('projects')
  })

  test('keyboard: ArrowRight expands a collapsed dir', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const journal = page.locator('#sidebar-tree [data-path="journal"]')
    await journal.focus()
    await page.keyboard.press('ArrowRight')
    await expect(journal).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('#sidebar-tree [data-path="journal/day-one.md"]')).toBeVisible()
  })

  test('tree root has role=tree and items have role=treeitem', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#sidebar-tree .tv-root')).toHaveAttribute('role', 'tree')
    const items = page.locator('#sidebar-tree [role="treeitem"]')
    await expect(items.first()).toBeVisible()
  })

  test('section collapse/expand still works', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#files-content')).toBeVisible()
    await page.locator('#files-section > button').click()
    await expect(page.locator('#files-content')).toBeHidden()
    await page.locator('#files-section > button').click()
    await expect(page.locator('#files-content')).toBeVisible()
  })
})
```

- [ ] **Step 6: Build the assets and run integration tests**

Run: `make assets`

Expected: Vite build succeeds; `web/static/app.js` emitted.

Run: `npx playwright test tests/sidebar-tree.spec.ts`

Expected: all new tests pass. If any fail, fix the component/glue per the test's expectation.

- [ ] **Step 7: Commit**

```bash
git add web/src/sidebar.js web/src/app.js web/templates/sidebar_tree.html tests/sidebar-tree.spec.ts
git commit -m "Switch sidebar to client-driven TreeView component (#88)"
```

---

## Task 15: Remove dead server code and simplify `entry_list.html`

**Files:**
- Modify: `internal/server/chrome.go`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/templates.go`
- Modify: `internal/server/chrome_test.go`
- Modify: `web/templates/entry_list.html`
- Modify: `internal/server/handlers_test.go` (tests that asserted old tree HTML)

After Task 14, the sidebar is client-driven and the old tree-building code paths have no users. This task removes them.

- [ ] **Step 1: Remove dead functions from `chrome.go`**

Edit `internal/server/chrome.go`. Delete:
- `buildDirTree`
- `buildTreeLevel`
- `readDirEntriesAtDepth`

Keep: `viewPath`, `tagPath`, `readDirEntries`.

The remaining file should read:

```go
package server

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func viewPath(relPath string) string {
	segments := strings.Split(relPath, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

func tagPath(tag string) string {
	return url.PathEscape(tag)
}

func readDirEntries(absPath, relPath string) ([]IndexEntry, error) {
	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	entries := make([]IndexEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryRel := name
		if relPath != "" {
			entryRel = filepath.ToSlash(filepath.Join(relPath, name))
		}
		var href string
		if de.IsDir() {
			href = "/dir/" + viewPath(entryRel)
		} else {
			href = "/view/" + viewPath(entryRel)
		}
		entries = append(entries, IndexEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Href:  href,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
```

- [ ] **Step 2: Remove dead branches from `handleDir` and tree calls from other handlers**

Edit `internal/server/handlers.go`:

In `handleRoot`, remove the `buildDirTree` call. Replace:

```go
	tree, err := buildDirTree(s.root, "")
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", "", "err", err)
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
```

with:

```go
	filesCard := &IndexCard{Empty: "No files here."}
```

In `handleView`, remove the `buildDirTree` call inside the full-page branch. Replace:

```go
	sidebarDir := currentDir
	lf := s.buildLayoutFields(title, editPath)

	tree, err := buildDirTree(s.root, sidebarDir)
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", sidebarDir, "err", err)
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
```

with:

```go
	lf := s.buildLayoutFields(title, editPath)
	filesCard := &IndexCard{Empty: "No files here."}
```

Delete the unused `currentDir` variable if it is no longer referenced (it is still used for `s.renderer.Render(data, currentDir)`, so keep that line). Only the `sidebarDir` / `tree` / `buildDirTree` usage is removed.

In `handleDir`, remove:
- The `?children=1&depth=N` branch (the block beginning `if r.URL.Query().Get("children") == "1" {`).
- The sidebar partial branch (`if r.Header.Get("HX-Request") == "true" {` that calls `buildDirTree`).
- The `buildDirTree` call in the full-page branch. Replace:

```go
	tree, err := buildDirTree(s.root, dirPath)
	if err != nil {
		s.logger.Warn("sidebar tree build failed", "dir", dirPath, "err", err)
	}
	title := dirPath
	if title == "" {
		title = "/"
	}
	filesCard := &IndexCard{Entries: tree, Empty: "No files here."}
```

with:

```go
	title := dirPath
	if title == "" {
		title = "/"
	}
	filesCard := &IndexCard{Empty: "No files here."}
```

The `strconv` import becomes unused (only the children-branch used it) — remove the `"strconv"` line from the import block if so.

In `handleTags` and `handleTagNotes`, the full-page branches call `s.buildDirIndex("")` to produce the sidebar's files card. With the new empty placeholder template, the `filesCard` is unused at render time (the template no longer iterates entries from `Files`). Either leave the calls in (harmless) or drop them. Drop them:

In `handleTags` replace:

```go
	filesCard, _ := s.buildDirIndex("")
	if filesCard == nil {
		filesCard = &IndexCard{Empty: "No files here."}
	}
```

with:

```go
	filesCard := &IndexCard{Empty: "No files here."}
```

Same substitution in `handleTagNotes`.

- [ ] **Step 3: Remove `IndexCard.Flat`, `renderEntryListRows`, `renderEntryList`, `Files` from types**

Edit `internal/server/templates.go`. Remove:
- The `Flat` field on `IndexCard`.
- The `Expanded` and `Depth` fields on `IndexEntry`.
- The `renderEntryListRows` method.
- The `renderEntryList` method.

The `IndexEntry` struct shrinks to:

```go
type IndexEntry struct {
	Name  string
	IsDir bool
	IsTag bool
	Href  string
}
```

The `IndexCard` struct shrinks to:

```go
type IndexCard struct {
	Entries []IndexEntry
	Empty   string
}
```

Note: the `Files` field on `SidebarPartialData` is still referenced by the new `sidebar_tree.html` in the `{{ with .Files }}` block — but after Task 14 that block was replaced by `<div id="sidebar-tree"></div>`, so `Files` is no longer read. Remove it too:

```go
type SidebarPartialData struct {
	Tags        *IndexCard
	InitialJSON template.JS
}
```

Then remove the `Files: filesCard,` field from every `SidebarPartialData{...}` literal in `handlers.go`. The `filesCard` local variables become unused; delete them too.

- [ ] **Step 4: Simplify `entry_list.html` to the flat-only branch**

Edit `web/templates/entry_list.html`. Since `IndexCard.Flat` is gone and only the flat path remains, collapse the template to:

```html
{{ define "entry_list_rows" }}
{{ range .Entries }}
{{ if .IsTag }}
<li class="border-b border-gray-100 last:border-b-0">
  <a
    href="{{ .Href }}"
    title="{{ .Name }}"
    data-action="selectTag"
    data-entry-type="tag"
    data-entry-name="{{ .Name }}"
    data-entry-href="{{ .Href }}"
    class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/></svg><span class="truncate min-w-0">{{ .Name }}</span></a>
</li>
{{ else if .IsDir }}
<li class="border-b border-gray-100 last:border-b-0">
  <a
    href="{{ .Href }}"
    title="{{ .Name }}"
    data-action="selectDir"
    data-entry-type="dir"
    data-entry-href="{{ .Href }}"
    class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 font-medium no-underline transition-colors duration-100 border border-transparent hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg><span class="truncate min-w-0">{{ .Name }}</span></a>
</li>
{{ else }}
<li class="border-b border-gray-100 last:border-b-0">
  <a
    href="{{ .Href }}"
    title="{{ .Name }}"
    data-action="selectNote"
    data-entry-type="note"
    data-entry-href="{{ .Href }}"
    class="entry-link flex items-center gap-2 px-4 py-2 text-sm text-blue-600 no-underline transition-colors duration-100 border border-transparent hover:border-blue-300"><svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="flex-shrink-0"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 13H8"/><path d="M16 13h-2"/><path d="M10 17H8"/><path d="M16 17h-2"/></svg><span class="truncate min-w-0">{{ .Name }}</span></a>
</li>
{{ end }}
{{ end }}
{{ end }}

{{ define "entry_list" }}
{{ if .Entries }}
<ul class="list-none m-0 p-0">
{{ template "entry_list_rows" . }}
</ul>
{{ else }}
<p class="px-4 py-6 text-gray-500 text-center">{{ .Empty }}</p>
{{ end }}
{{ end }}
```

Note: `selectNote` and `selectDir` handlers in `app.js` were removed in Task 14, but the flat main-pane listing (e.g. `/dir/journal` full-page, or `/tags/daily` filter) still renders `<a>` links with those `data-action` attributes. They now rely on the `<a href>` acting as a regular link — clicking lets the browser navigate to `/view/...` or `/dir/...` naturally. That is acceptable because the main-pane listing is not driven by HTMX after this refactor.

Wait — this changes main-pane navigation behavior. Re-check: `dir_listing.html` is rendered inside `#note-pane` on HTMX-targeted requests. When a user clicks a `selectNote` link in the main pane today, `app.js` intercepts via delegation and routes via `htmx.ajax`. After Task 14 removed the delegation, clicks fall through to the default `<a href>` behavior — full-page reload.

A full-page reload is correct (it hits the HTMX full-page handler, renders the two-pane layout). The sidebar will re-initialize and the TreeView will restore state from localStorage + the new URL. Usable, but slower than an HTMX partial swap.

Keeping the HTMX partial swap for main-pane listings: re-add a narrow click delegation in `app.js` that handles `data-action="selectNote"` and `data-action="selectDir"` clicks anywhere OUTSIDE `#sidebar-tree` (so the tree's internal clicks aren't intercepted). The sidebar's tree clicks go through the component's own handlers.

Add this block to `app.js` (back-fill from Step 3 above):

```js
document.addEventListener('click', function(e) {
  const link = e.target.closest('[data-action]')
  if (!link) return
  if (e.target.closest('#sidebar-tree')) return  // tree handles its own clicks
  const action = link.dataset.action
  if (action === 'selectTag') {
    e.preventDefault()
    selectTag(link.dataset.entryName, false)
  } else if (action === 'selectDir' || action === 'selectNote') {
    e.preventDefault()
    const href = link.dataset.entryHref
    htmx.ajax('GET', href, {
      target: '#note-pane',
      swap: 'innerHTML',
      headers: { 'HX-Target': 'note-pane' },
    })
    history.pushState({ type: action === 'selectDir' ? 'dir' : 'note', href }, '', href)
  }
})
```

Delete the narrower `selectTag`-only delegation added earlier in Task 14. (This consolidated handler supersedes it.)

On popstate for `dir`/`note`, the sidebar glue (in `sidebar.js`) handles ancestor expansion and selection via `tree.select(...)`. The `app.js` popstate handler remains focused on tags (and unrecognized routes).

- [ ] **Step 5: Update `chrome_test.go` if it referenced removed functions**

Inspect `internal/server/chrome_test.go`. The existing tests target `readDirEntries` only (per the snippet read earlier), so no change is needed.

- [ ] **Step 6: Inspect `handlers_test.go` for now-invalid assertions**

Search for tests that asserted the old tree-rendered HTML (e.g., `data-depth`, `data-expanded`, chevron `<button>` inside sidebar). Delete or adjust such assertions. The new assertions from Task 13 (`tv-initial`, `selectedPath`) are the canonical coverage.

Run: `grep -n 'data-depth\|data-expanded\|tv-item\|children=1' internal/server/handlers_test.go`

For each hit, delete the assertion if it targets the old sidebar tree. Keep assertions that target the main-pane dir listing (`#dir-listing` descendant markup is still flat).

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`

Expected: all pass. Fix any compilation errors from the struct shrinkage (e.g., a test constructing `IndexEntry{Depth: N}` needs updating to drop the field).

Run: `make assets && npx playwright test`

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add internal/server/chrome.go internal/server/handlers.go internal/server/templates.go internal/server/handlers_test.go web/templates/entry_list.html web/src/app.js
git commit -m "Remove dead tree-rendering code; simplify entry_list template"
```

---

## Task 16: CHANGELOG entry and final verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Read the existing CHANGELOG format**

Run: `head -40 CHANGELOG.md`

- [ ] **Step 2: Add an entry at the top under the appropriate heading**

Add to `CHANGELOG.md` following the existing style (keep wording terse):

```markdown
- Refactor sidebar into a reusable client-side `TreeView` component. Tree state (expanded, selected, focus) lives in the browser; the server exposes `/api/tree/list` for children and a unified `/events` SSE stream for file-change and tree-mutation notifications. Closes #88.
```

- [ ] **Step 3: Final verification**

Run each in parallel where possible:

```bash
go test ./...
npm run test:unit
make assets
npx playwright test
go tool golangci-lint run
```

Expected: everything green.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "CHANGELOG: client-side TreeView component (#88)"
```

- [ ] **Step 5: Summary self-check**

Confirm the working tree matches the spec:
- `web/src/tree-view.js` + `.test.js` exist, tests pass.
- `web/src/sidebar.js` exists and is imported by `app.js`.
- `internal/server/tree_api.go` exists; `/api/tree/list` returns JSON.
- `internal/server/events.go` (renamed from `sse.go`) emits both `change` and `dir-changed` events on one endpoint.
- `web/templates/sidebar_tree.html` contains `<script id="tv-initial">` and `<div id="sidebar-tree">`.
- `web/templates/entry_list.html` is flat-only.
- `internal/server/chrome.go` no longer contains `buildDirTree` et al.
- Playwright suite passes.

---

## Self-review

**Spec coverage check:**

- Component API (construction, methods, events) — Task 2–9.
- Node shape + globally-unique path contract — Tasks 2 and 10.
- Container ownership (wipe innerHTML) — Task 2 test asserts this.
- Internal state model (Maps, Sets, loadingPaths with pendingRefresh) — Tasks 2, 3, 6.
- DOM structure (tv-root role=tree, tv-item treeitem, tv-row, tv-toggle aria-hidden, tv-icon, tv-label, --tv-depth) — Task 2.
- Tailwind-only styling — Task 2 row/label/icon classes.
- Reconciliation — Task 5, includes isDir-flip case.
- Refresh during load — Task 6.
- Persistence + initial.selectedPath merge — Task 7.
- Keyboard (arrows, Home, End) — Task 8.
- Enter/Space, typeahead, click, scrollTo — Task 9.
- `/api/tree/list` — Task 10.
- EventHub rename — Task 11.
- dir-changed broadcast + recursive watching — Task 12.
- Unified `/events` endpoint — Task 12 (handleEvents emits both types).
- Initial-state JSON embed — Task 13.
- Application glue + template placeholder + app.js cleanup — Task 14.
- Dead-code removal — Task 15.
- CHANGELOG + verification — Task 16.

All spec sections covered.

**Placeholder scan:** No TBDs, TODOs, or "similar to" handwaves. Each task shows the actual code or specific removals to make.

**Type consistency:** Method names (`expand`, `collapse`, `toggle`, `select`, `refresh`, `scrollTo`, `destroy`) are consistent across tasks. Internal helpers (`_doExpand`, `_reconcile`, `_removeSubtree`, `_findItem`, `_childUl`, `_nodesAt`, `_ancestors`, `_readStorage`, `_writeStorage`, `_updateRovingTabindex`, `_handleKeydown`, `_handleClick`, `_typeahead`, `_focusPath`, `_focusRelative`, `_focusEdge`, `_visibleItems`, `_arrowRight`, `_arrowLeft`) appear consistently in the tasks that reference them. Go: `EventHub`, `Subscription`, `eventMsg`, `handleEvents`, `handleTreeList` are consistent.

**Edge case coverage:** isDir flip (Task 5), refresh-during-load (Task 6), stale storage (Task 7), 404 loader rejection (Tasks 3, 7), path traversal (Task 10), path-escaped query (Task 10).
