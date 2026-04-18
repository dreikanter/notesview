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
