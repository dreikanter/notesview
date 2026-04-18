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
    rowHref: (node) => (node.isDir ? '/dir/' : '/view/') + encodePath(node.path),
  })

  // One EventSource per page, reopened whenever the watched note path changes.
  // Emits 'dir-changed' for tree mutations and 'change' for the current note's
  // content updates.
  let es = null
  let watchedNote = null
  const openEventSource = (notePath) => {
    if (es) es.close()
    watchedNote = notePath || null
    window.__tvWatchedNote = watchedNote || ''
    const esURL = '/events' + (notePath ? '?watch=' + encodeURIComponent(notePath) : '')
    es = new EventSource(esURL)
    es.addEventListener('dir-changed', (e) => {
      try {
        const { path } = JSON.parse(e.data)
        tree.refresh(path)
      } catch {}
    })
    es.addEventListener('change', () => {
      if (!watchedNote) return
      htmx.ajax('GET', '/view/' + encodePath(watchedNote), {
        target: '#note-pane',
        swap: 'innerHTML',
        headers: { 'HX-Target': 'note-pane' },
      })
    })
  }
  openEventSource(document.body.getAttribute('data-note-path') || '')

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
    openEventSource(node.isDir ? '' : path)
    if (node.isDir) tree.expand(path)
  })

  window.addEventListener('popstate', async () => {
    const path = pathFromURL(location.pathname)
    if (!path) return
    htmx.ajax('GET', location.pathname, {
      target: '#note-pane',
      swap: 'innerHTML',
      headers: { 'HX-Target': 'note-pane' },
    })
    for (const a of ancestorsOf(path)) {
      try { await tree.expand(a) } catch {}
    }
    tree.select(path, { source: 'silent' })
    tree.scrollTo(path)
    const isNote = location.pathname.startsWith('/view/')
    openEventSource(isNote ? path : '')
  })

  return {
    tree,
    setWatchedNote: (notePath) => openEventSource(notePath || ''),
  }
}
