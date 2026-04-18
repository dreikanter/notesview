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
    this.classPrefix = options.classPrefix ?? 'tv-'

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

    this.ready = this._bootstrap()
  }

  _cls(suffix) {
    return `${this.classPrefix}${suffix}`
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
      btn.className = `${this._cls('toggle')} flex items-center justify-center w-8 flex-shrink-0 text-gray-400 cursor-pointer bg-transparent border-0 p-0`
      btn.setAttribute('tabindex', '-1')
      btn.setAttribute('aria-hidden', 'true')
      btn.textContent = '\u25B8'
      row.appendChild(btn)
    } else {
      const spacer = document.createElement('span')
      spacer.className = `${this._cls('toggle-spacer')} w-8 flex-shrink-0`
      row.appendChild(spacer)
    }

    const icon = document.createElement('span')
    icon.className = `${this._cls('icon')} flex-shrink-0`
    if (typeof this.renderIcon === 'function') {
      const result = this.renderIcon(node)
      if (typeof result === 'string') icon.textContent = result
      else if (result instanceof Node) icon.appendChild(result)
    } else {
      icon.textContent = node.isDir ? '\uD83D\uDCC1' : '\uD83D\uDCC4'
    }
    row.appendChild(icon)

    const label = document.createElement('span')
    label.className = `${this._cls('label')} truncate min-w-0 text-blue-600`
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
    this.expandedPaths.add(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: true } }))
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

  collapse(path) {
    if (!this.expandedPaths.has(path)) return
    const li = this._findItem(path)
    if (!li) return
    const childUl = this._childUl(path)
    if (childUl) childUl.remove()
    li.setAttribute('aria-expanded', 'false')
    this.expandedPaths.delete(path)
    this.container.dispatchEvent(new CustomEvent('tree:toggle', { detail: { path, expanded: false } }))
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

  _findItem(path) {
    return this.root.querySelector(`[data-path="${escapeSelector(path)}"]`)
  }

  destroy() {
    this.container.innerHTML = ''
  }
}
