// nview front-end bootstrap.
//
// Loads HTMX + SSE support, runs syntax highlighting on swaps, owns sidebar
// toggle/navigation, and keeps note updates flowing through the unified
// /events endpoint with per-view scope filtering.

import htmx from 'htmx.org'
import 'htmx-ext-sse'
import hljs from 'highlight.js/lib/common'

function highlightIn(root) {
  if (!root || !root.querySelectorAll) return
  root.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el)
  })
}

let es = null
let watchedNoteID = 0

function currentNoteID() {
  const card = document.getElementById('note-card')
  if (!card) return 0
  const v = parseInt(card.getAttribute('data-note-id') || '', 10)
  return Number.isFinite(v) && v > 0 ? v : 0
}

// openEventStream opens an SSE connection scoped to either the current
// note (id > 0) or the list view (id === 0). Idempotent for the same scope:
// reopens only when the desired scope changes.
function openEventStream(noteID) {
  if (es && watchedNoteID === noteID) return
  if (es) es.close()
  watchedNoteID = noteID
  const url = noteID > 0
    ? '/events?scope=note&id=' + encodeURIComponent(noteID)
    : '/events?scope=list'
  es = new EventSource(url)
  es.addEventListener('note', () => {
    const card = document.getElementById('note-card')
    const href = card?.getAttribute('data-view-href') || location.pathname
    htmx.ajax('GET', href, {
      target: '#note-pane',
      swap: 'innerHTML',
      headers: { 'HX-Target': 'note-pane' },
    })
  })
  es.addEventListener('list', () => {
    refreshSidebar()
    refreshListPane()
  })
}

function refreshSidebar() {
  const card = document.getElementById('note-card')
  const selectedPath = card?.getAttribute('data-note-path') || ''
  const qs = selectedPath ? '?selected=' + encodeURIComponent(selectedPath) : ''
  htmx.ajax('GET', '/sidebar' + qs, {
    target: '#sidebar',
    swap: 'innerHTML',
  })
}

// refreshListPane reloads the dir-listing partial in the note pane when the
// current view is a tag/type/date list (i.e., not a single note).
function refreshListPane() {
  if (currentNoteID() > 0) return
  const path = location.pathname
  if (!path.startsWith('/tags') && !path.startsWith('/types') && !path.startsWith('/dates')) return
  htmx.ajax('GET', path, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  })
}

// reconcileIndex POSTs /api/index/refresh and refreshes UI when the diff
// is non-empty. Throttled so rapid visibility flips (focus/blur, multi-
// monitor) don't hammer the store-walking endpoint.
const RECONCILE_MIN_INTERVAL_MS = 5000
let lastReconcileAt = 0

function reconcileIndex(force) {
  const now = Date.now()
  if (!force && now - lastReconcileAt < RECONCILE_MIN_INTERVAL_MS) return
  lastReconcileAt = now
  fetch('/api/index/refresh', { method: 'POST' })
    .then((r) => (r.ok ? r.json() : null))
    .then((diff) => {
      if (!diff) return
      if (diff.added || diff.updated || diff.deleted) {
        refreshSidebar()
        refreshListPane()
      }
    })
    .catch(() => {})
}

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document)
  wireSidebarToggle()
  wireThemeToggle()
  wireRefreshButton()
  openEventStream(currentNoteID())
})

document.addEventListener('visibilitychange', function () {
  if (document.visibilityState === 'visible') {
    reconcileIndex()
  }
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
    localStorage.setItem('nview.sidebarOpen', open ? '1' : '0')
  } catch (e) {}
}

function wireThemeToggle() {
  const btn = document.getElementById('theme-toggle')
  if (!btn) return
  const root = document.documentElement
  btn.setAttribute('aria-pressed', root.classList.contains('dark') ? 'true' : 'false')
  btn.addEventListener('click', () => {
    const isDark = root.classList.toggle('dark')
    btn.setAttribute('aria-pressed', isDark ? 'true' : 'false')
    try {
      localStorage.setItem('nview.theme', isDark ? 'dark' : 'light')
    } catch (e) {}
    const light = document.getElementById('hljs-light')
    const dark = document.getElementById('hljs-dark')
    if (light) light.disabled = isDark
    if (dark) dark.disabled = !isDark
  })
}

function wireRefreshButton() {
  const btn = document.getElementById('index-refresh')
  if (!btn) return
  btn.addEventListener('click', () => reconcileIndex(true))
}

function loadIntoPane(href, state) {
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  })
  if (state) history.pushState(state, '', href)
}

window.selectTag = function(tag, skipPush) {
  const href = '/tags/' + encodeURIComponent(tag)
  loadIntoPane(href, skipPush ? null : { type: 'tag', tag: tag, href: href })
  openEventStream(0)
}

let pendingNoteScrollReset = false

document.addEventListener('click', function(e) {
  const uidLink = e.target.closest('.uid-link')
  if (uidLink) {
    e.preventDefault()
    const uid = (uidLink.dataset.noteUid || uidLink.textContent || '').replace(/^#/, '').trim()
    if (uid) scrollSidebarToUID(uid)
    return
  }

  const link = e.target.closest('[data-action]')
  if (!link) return
  const action = link.dataset.action
  if (action === 'selectTag') {
    e.preventDefault()
    selectTag(link.dataset.entryName, false)
  } else if (action === 'selectDir' || action === 'selectNote' || action === 'selectIndex') {
    e.preventDefault()
    const href = link.dataset.entryHref || link.getAttribute('href')
    pendingNoteScrollReset = true
    const stateType = action === 'selectNote' ? 'note' : 'index'
    loadIntoPane(href, { type: stateType, href })
    // For note links, the htmx:afterSwap handler will read the new
    // data-note-id from the freshly-rendered partial. List links open
    // immediately on the list-scope stream.
    if (action !== 'selectNote') openEventStream(0)
  }
})

function scrollSidebarToUID(uid) {
  const link = document.querySelector('#sidebar [data-note-uid="' + cssEscape(uid) + '"]')
  if (!link) return
  link.scrollIntoView({ block: 'center', inline: 'nearest' })
  link.classList.add('bg-blue-50', 'dark:bg-blue-950')
  setTimeout(() => link.classList.remove('bg-blue-50', 'dark:bg-blue-950'), 1200)
}

function cssEscape(s) {
  if (typeof CSS !== 'undefined' && CSS.escape) return CSS.escape(s)
  return String(s).replace(/["\\[\]]/g, '\\$&')
}

window.addEventListener('popstate', function(e) {
  const state = e.state
  const href = state?.href || location.pathname
  loadIntoPane(href, null)
  // Note views will resubscribe on htmx:afterSwap once the partial loads;
  // non-note views can subscribe immediately.
  if (!href.startsWith('/n/')) openEventStream(0)
})

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target)
  if (e.target && e.target.id === 'note-pane') {
    openEventStream(currentNoteID())
  }
  if (pendingNoteScrollReset && e.target && e.target.id === 'note-pane') {
    e.target.scrollTop = 0
    pendingNoteScrollReset = false
  }
})
