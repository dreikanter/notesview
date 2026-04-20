// TreeView — reusable client-side tree view component.
//
// Public API:
//   const tv = new TreeView(container, { loader, rootPath, persistKey, initial, ... })
//   await tv.ready
//   tv.expand(path)  — resolves on success or loader error (see tree:error).
//                      No-op if path is unknown, a file, or already expanded.
//   tv.collapse(path)/tv.toggle(path) — no-op for unknown paths.
//   tv.select(path, { source })
//   tv.refresh(path)
//   tv.scrollTo(path, { block })
//   tv.destroy()
//
// Options:
//   rowHref(node) => string: returns a URL so icon+label are wrapped in
//     an <a href tabindex=-1>. Plain left-click is intercepted by the
//     component's click handler; modifier/middle/right clicks fall through
//     to the browser for new-tab / context-menu affordances.
//
// Events dispatched on `container`:
//   tree:select { path, node, source }
//   tree:toggle { path, expanded }
//   tree:error  { path, error }
//
// See docs/superpowers/specs/2026-04-18-tree-view-component-design.md

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
    this.rowHref = options.rowHref
    this.classPrefix = options.classPrefix ?? 'tv-'

    this.persistKey = options.persistKey
    this.initial = options.initial ?? null

    this.nodesByPath = new Map()
    this.childrenByPath = new Map()
    this.expandedPaths = new Set()
    this.selectedPath = null
    this.focusedPath = null
    this.loadingPaths = new Map()

    this.container.innerHTML = ''
    this.root = document.createElement('div')
    this.root.className = this._cls('root')
    this.root.setAttribute('role', 'tree')
    this.container.appendChild(this.root)

    this._onKeydown = (e) => this._handleKeydown(e)
    this.root.addEventListener('keydown', this._onKeydown)

    this._typeaheadBuffer = ''
    this._typeaheadTimer = null
    this._onClick = (e) => this._handleClick(e)
    this.root.addEventListener('click', this._onClick)

    this.ready = this._bootstrap()
  }

  _cls(suffix) {
    return `${this.classPrefix}${suffix}`
  }

  async _bootstrap() {
    await this._loadChildren(this.rootPath)
    this._renderChildren(this.rootPath, this.root, this._nodesAt(this.rootPath), 0)

    const stored = this._readStorage()
    const initialSelected = this.initial?.selectedPath ?? null
    const initialExpanded = initialSelected ? this._ancestors(initialSelected) : []
    const fromStorage = stored?.expanded ?? []
    const toExpand = Array.from(new Set([...fromStorage, ...initialExpanded]))
      .sort((a, b) => a.split('/').length - b.split('/').length)

    for (const p of toExpand) {
      await this.expand(p)
    }

    const staleFromStorage = fromStorage.filter((p) => !this.expandedPaths.has(p))
    if (this.persistKey && stored && staleFromStorage.length > 0) {
      this._writeStorage()
    }

    const selected = initialSelected ?? stored?.selected ?? null
    if (selected) {
      const li = this._findItem(selected)
      if (li) this.select(selected, { source: 'silent' })
    }
  }

  async _loadChildren(path) {
    const nodes = await this.loader(path)
    this.childrenByPath.set(path, nodes.map((n) => n.path))
    for (const n of nodes) this.nodesByPath.set(n.path, n)
    return nodes
  }

  _renderChildren(parentPath, parentEl, nodes, baseLevel) {
    const ul = document.createElement('ul')
    ul.className = `${this._cls('group')} list-none m-0 p-0`
    ul.setAttribute('role', 'group')
    for (const node of nodes) {
      ul.appendChild(this._buildItem(node, baseLevel + 1))
    }
    parentEl.appendChild(ul)
    this._updateRovingTabindex()
  }

  _buildItem(node, level) {
    const li = document.createElement('li')
    li.className = `${this._cls('item')} ${node.isDir ? this._cls('item--dir') : this._cls('item--file')}`
    li.setAttribute('role', 'treeitem')
    li.setAttribute('data-path', node.path)
    li.setAttribute('aria-level', String(level))
    li.setAttribute('aria-selected', 'false')
    li.setAttribute('tabindex', '-1')
    li.style.setProperty('--tv-depth', String(level - 1))
    if (node.isDir) li.setAttribute('aria-expanded', 'false')

    const row = document.createElement('div')
    row.className = `${this._cls('row')} flex items-center gap-2 px-4 py-2 text-sm`
    if (node.isDir) {
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = `${this._cls('toggle')} flex items-center justify-center w-4 flex-shrink-0 text-gray-400 dark:text-gray-500 cursor-pointer bg-transparent border-0 p-0`
      btn.setAttribute('tabindex', '-1')
      btn.setAttribute('aria-hidden', 'true')
      btn.textContent = '\u25B8'
      row.appendChild(btn)
    } else {
      const spacer = document.createElement('span')
      spacer.className = `${this._cls('toggle-spacer')} w-4 flex-shrink-0`
      row.appendChild(spacer)
    }

    const href = typeof this.rowHref === 'function' ? this.rowHref(node) : null
    const link = document.createElement(href ? 'a' : 'span')
    link.className = `${this._cls('link')} flex-1 min-w-0 flex items-center gap-2 no-underline text-blue-600 dark:text-blue-400`
    if (href) {
      link.setAttribute('href', href)
      link.setAttribute('tabindex', '-1')
    }

    const icon = document.createElement('span')
    icon.className = `${this._cls('icon')} flex-shrink-0 inline-flex items-center`
    if (typeof this.renderIcon === 'function') {
      const result = this.renderIcon(node)
      if (typeof result === 'string') icon.textContent = result
      else if (result instanceof Node) icon.appendChild(result)
    } else {
      icon.innerHTML = node.isDir
        ? '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>'
        : '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/></svg>'
    }
    link.appendChild(icon)

    const label = document.createElement('span')
    label.className = `${this._cls('label')} truncate min-w-0`
    if (typeof this.renderLabel === 'function') {
      const result = this.renderLabel(node)
      if (typeof result === 'string') label.textContent = result
      else if (result instanceof Node) label.appendChild(result)
    } else {
      label.textContent = node.name
    }
    link.appendChild(label)

    row.appendChild(link)

    li.appendChild(row)
    li.style.paddingLeft = `calc(var(--tv-depth) * 1rem)`
    return li
  }

  async expand(path) {
    if (this.expandedPaths.has(path)) return
    if (this.loadingPaths.has(path)) return this.loadingPaths.get(path).promise

    const li = this._findItem(path)
    if (!li || !li.classList.contains(this._cls('item--dir'))) return

    const entry = { pendingRefresh: false }
    const promise = this._doExpand(path, li, entry)
    entry.promise = promise
    this.loadingPaths.set(path, entry)

    try {
      await promise
    } catch (_) {
      // tree:error already emitted by _doExpand; do not rethrow.
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
    this._setChevron(li, true)
    this.expandedPaths.add(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: true } }))
    this._writeStorage()
  }

  async refresh(path) {
    if (path !== this.rootPath && !this.expandedPaths.has(path) && !this.loadingPaths.has(path)) return
    if (this.loadingPaths.has(path)) {
      this.loadingPaths.get(path).pendingRefresh = true
      return this.loadingPaths.get(path).promise
    }

    const entry = { pendingRefresh: false }
    const promise = this._doRefresh(path)
    entry.promise = promise
    this.loadingPaths.set(path, entry)

    try {
      await promise
    } finally {
      this.loadingPaths.delete(path)
      if (entry.pendingRefresh && (path === this.rootPath || this.expandedPaths.has(path))) {
        this.refresh(path)
      }
    }
  }

  async _doRefresh(path) {
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

    for (const p of prev) {
      if (!nextSet.has(p)) this._removeSubtree(p)
    }

    let selectionWasFlipped = false
    for (const n of nextNodes) {
      const existing = this.nodesByPath.get(n.path)
      if (existing && existing.isDir !== n.isDir) {
        if (this.selectedPath === n.path) {
          this.selectedPath = null
          selectionWasFlipped = true
        }
        this._removeSubtree(n.path)
      }
    }

    const parentUl = this._childUl(parentPath)
    if (!parentUl) return

    const level = parentPath === this.rootPath
      ? 1
      : Number(this._findItem(parentPath).getAttribute('aria-level')) + 1

    let cursor = null
    for (const node of nextNodes) {
      let li = this._findItem(node.path)
      const isNew = !li || !prevSet.has(node.path)
      if (isNew) {
        li = this._buildItem(node, level)
      }
      this.nodesByPath.set(node.path, node)
      if (cursor === null) {
        parentUl.insertBefore(li, parentUl.firstChild)
      } else {
        cursor.after(li)
      }
      cursor = li
    }

    this.childrenByPath.set(parentPath, next)

    if (this.focusedPath && !this._findItem(this.focusedPath)) {
      this.focusedPath = this._findItem(parentPath) ? parentPath : null
    }

    if ((this.selectedPath && !this._findItem(this.selectedPath)) || selectionWasFlipped) {
      this.selectedPath = null
      this.focusedPath = null
      this._updateRovingTabindex()
      this.container.dispatchEvent(new CustomEvent('tree:select', {
        detail: { path: null, node: null, source: 'api' },
      }))
    } else {
      this._updateRovingTabindex()
    }
    this._writeStorage()
  }

  _removeSubtree(path) {
    const descendants = this.childrenByPath.get(path) ?? []
    for (const d of descendants) this._removeSubtree(d)
    this.childrenByPath.delete(path)
    this.expandedPaths.delete(path)
    this.nodesByPath.delete(path)
    const li = this._findItem(path)
    if (li) li.remove()
  }

  // Returns the direct-child <ul class="<prefix>group"> for `path`, or null.
  // Implemented without `:scope` because happy-dom (our test environment)
  // does not support it.
  _childUl(path) {
    const parent = path === this.rootPath ? this.root : this._findItem(path)
    if (!parent) return null
    const groupCls = this._cls('group')
    for (const child of parent.children) {
      if (child.tagName === 'UL' && child.classList.contains(groupCls)) return child
    }
    return null
  }

  _setChevron(li, expanded) {
    const row = li.children[0]
    if (!row) return
    const toggleCls = this._cls('toggle')
    for (const child of row.children) {
      if (child.classList && child.classList.contains(toggleCls)) {
        child.textContent = expanded ? '\u25BE' : '\u25B8'
        return
      }
    }
  }

  collapse(path) {
    if (!this.expandedPaths.has(path)) return
    const li = this._findItem(path)
    if (!li) return
    const childUl = this._childUl(path)
    if (childUl) childUl.remove()
    li.setAttribute('aria-expanded', 'false')
    this._setChevron(li, false)
    this.expandedPaths.delete(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: false } }))
    this._writeStorage()
  }

  toggle(path) {
    return this.expandedPaths.has(path) ? this.collapse(path) : this.expand(path)
  }

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
      this._writeStorage()
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
    this._writeStorage()
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

  _findItem(path) {
    return this.root.querySelector(`[data-path="${escapeSelector(path)}"]`)
  }

  destroy() {
    this.root.removeEventListener('keydown', this._onKeydown)
    this.root.removeEventListener('click', this._onClick)
    if (this._typeaheadTimer) clearTimeout(this._typeaheadTimer)
    this.container.innerHTML = ''
  }

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

    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      e.preventDefault()
      this._typeahead(e.key.toLowerCase())
    }
  }

  _typeahead(ch) {
    const isNewBuffer = this._typeaheadBuffer === ''
    this._typeaheadBuffer += ch
    if (this._typeaheadTimer) clearTimeout(this._typeaheadTimer)
    this._typeaheadTimer = setTimeout(() => { this._typeaheadBuffer = '' }, 500)

    const items = this._visibleItems()
    const currentIdx = items.findIndex((it) => it.getAttribute('data-path') === this.focusedPath)
    // For a new single-char buffer, start from the item after the focused one (cycling).
    // For an accumulated buffer, start from the focused item itself.
    const startIdx = isNewBuffer
      ? (currentIdx === -1 ? 0 : (currentIdx + 1) % items.length)
      : Math.max(0, currentIdx)
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
    const toggle = e.target.closest(`.${this._cls('toggle')}`)
    if (toggle) {
      const li = toggle.closest('[role="treeitem"]')
      if (li) this.toggle(li.getAttribute('data-path'))
      return
    }
    const li = e.target.closest('[role="treeitem"]')
    if (!li) return
    // Let the browser handle non-primary clicks and modifier-clicks so
    // Cmd/Ctrl+click (new tab), middle-click (new tab), and modifier clicks
    // on an <a href> inside the row work natively.
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    e.preventDefault()
    this.select(li.getAttribute('data-path'), { source: 'click' })
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
    if (!li.classList.contains(this._cls('item--dir'))) return
    if (!this.expandedPaths.has(path)) {
      this.focusedPath = path
      this._updateRovingTabindex()
      this.expand(path)
      return
    }
    const childUl = this._childUl(path)
    const firstChild = childUl?.querySelector('[role="treeitem"]')
    if (firstChild) this._focusPath(firstChild.getAttribute('data-path'))
  }

  _arrowLeft(path, li) {
    if (li.classList.contains(this._cls('item--dir')) && this.expandedPaths.has(path)) {
      this.collapse(path)
      this.focusedPath = path
      this._updateRovingTabindex()
      return
    }
    const parent = li.parentElement?.closest('[role="treeitem"]')
    if (parent) this._focusPath(parent.getAttribute('data-path'))
  }

  _focusEdge(which) {
    const items = this._visibleItems()
    if (!items.length) return
    const target = which === 'first' ? items[0] : items[items.length - 1]
    this._focusPath(target.getAttribute('data-path'))
  }

  scrollTo(path, options = {}) {
    const li = this._findItem(path)
    if (!li) return
    const block = options.block ?? 'center'
    li.scrollIntoView({ block, inline: 'nearest' })
  }
}
