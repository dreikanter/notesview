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

let sidebar = null

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document)
  wireSidebarToggle()
  wireThemeToggle()
  restoreSidebarState()
  sidebar = mountSidebar()
})

function decodeHref(href) {
  if (href.startsWith('/view/')) return decodeURIComponent(href.slice('/view/'.length))
  return ''
}

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

function wireThemeToggle() {
  const btn = document.getElementById('theme-toggle')
  if (!btn) return
  const root = document.documentElement
  btn.setAttribute('aria-pressed', root.classList.contains('dark') ? 'true' : 'false')
  btn.addEventListener('click', () => {
    const isDark = root.classList.toggle('dark')
    btn.setAttribute('aria-pressed', isDark ? 'true' : 'false')
    try {
      localStorage.setItem('notesview.theme', isDark ? 'dark' : 'light')
    } catch (e) {}
    const light = document.getElementById('hljs-light')
    const dark = document.getElementById('hljs-dark')
    if (light) light.disabled = isDark
    if (dark) dark.disabled = !isDark
  })
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
  if (sidebar) sidebar.setWatchedNote('')
  const content = document.getElementById('tags-content')
  const disclosure = document.getElementById('tags-disclosure')
  if (content) content.style.display = ''
  if (disclosure) disclosure.textContent = '\u25BE'
  setLS('tagsOpen', '1')
}

// Click delegation for links OUTSIDE the sidebar tree component.
// - Tags in the sidebar flat list.
// - Dir/note links rendered in the main pane by dir_listing.html.
// The tree handles its own clicks internally.
let pendingNoteScrollReset = false

document.addEventListener('click', function(e) {
  const link = e.target.closest('[data-action]')
  if (!link) return
  if (e.target.closest('#sidebar-tree')) return
  const action = link.dataset.action
  if (action === 'selectTag') {
    e.preventDefault()
    selectTag(link.dataset.entryName, false)
  } else if (action === 'selectDir' || action === 'selectNote') {
    e.preventDefault()
    const href = link.dataset.entryHref
    pendingNoteScrollReset = true
    htmx.ajax('GET', href, {
      target: '#note-pane',
      swap: 'innerHTML',
      headers: { 'HX-Target': 'note-pane' },
    })
    history.pushState({ type: action === 'selectDir' ? 'dir' : 'note', href }, '', href)
    if (sidebar) sidebar.setWatchedNote(action === 'selectNote' ? decodeHref(href) : '')
  }
})

window.addEventListener('popstate', function(e) {
  const state = e.state
  if (state && state.type === 'tag') {
    selectTag(state.tag, true)
  }
  // note/dir popstate is handled in sidebar.js, which calls tree APIs.
})

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target)
  if (pendingNoteScrollReset && e.target && e.target.id === 'note-pane') {
    e.target.scrollTop = 0
    pendingNoteScrollReset = false
  }
})
