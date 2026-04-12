// notesview front-end bootstrap.
//
// Loads HTMX + SSE, runs syntax highlighting on every swap, and owns
// the sidebar toggle and sidebar mode state (files/tags/tag).

import htmx from 'htmx.org';
import 'htmx-ext-sse';
import hljs from 'highlight.js/lib/common';

function highlightIn(root) {
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el);
  });
}

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document);
  wireSidebarToggle();
  restoreSidebarState();
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
});

// --- Sidebar toggle ---

function wireSidebarToggle() {
  const btn = document.getElementById('sidebar-toggle');
  if (!btn) return;
  const initiallyOpen = document.documentElement.classList.contains('sidebar-open');
  btn.setAttribute('aria-expanded', initiallyOpen ? 'true' : 'false');
  btn.addEventListener('click', toggleSidebar);
}

function toggleSidebar() {
  const root = document.documentElement;
  const btn = document.getElementById('sidebar-toggle');
  const open = root.classList.toggle('sidebar-open');
  if (btn) btn.setAttribute('aria-expanded', open ? 'true' : 'false');
  try {
    localStorage.setItem('notesview.sidebarOpen', open ? '1' : '0');
  } catch (e) {}

  if (open) {
    refreshSidebar();
  }
}

// --- Sidebar mode state ---

function refreshSidebar() {
  const mode = getSidebarMode();
  let url;
  if (mode === 'tags') {
    url = '/tags';
  } else if (mode === 'tag') {
    const tag = getSidebarTag();
    url = tag ? `/tags/${encodeURIComponent(tag)}` : '/tags';
  } else {
    const dir = getSidebarDir();
    url = `/dir/${encodePath(dir)}`;
  }
  htmx.ajax('GET', url, {
    target: '#sidebar',
    swap: 'innerHTML',
  });
}

function restoreSidebarState() {
  const mode = getSidebarMode();
  if (mode === 'files') return; // Server already rendered files mode
  refreshSidebar();
}

function getSidebarMode() {
  try { return localStorage.getItem('notesview.sidebarMode') || 'files'; } catch (e) { return 'files'; }
}

function getSidebarTag() {
  try { return localStorage.getItem('notesview.sidebarTag') || ''; } catch (e) { return ''; }
}

function getSidebarDir() {
  try { return localStorage.getItem('notesview.sidebarDir') || ''; } catch (e) { return ''; }
}

// Encode a directory path for use in URLs, encoding each segment
// individually while preserving literal / separators.
function encodePath(p) {
  if (!p) return '';
  return p.split('/').map(encodeURIComponent).join('/');
}

// Global functions called from template onclick handlers.
// These update localStorage before HTMX fires the request.

// switchToFiles navigates the sidebar to the root directory.
// The breadcrumb trail handles navigation within the directory tree.
window.switchToFiles = function() {
  try {
    localStorage.setItem('notesview.sidebarMode', 'files');
    localStorage.setItem('notesview.sidebarDir', '');
  } catch (e) {}
  htmx.ajax('GET', '/dir/', {
    target: '#sidebar',
    swap: 'innerHTML',
  });
};

window.switchToTags = function() {
  try {
    localStorage.setItem('notesview.sidebarMode', 'tags');
  } catch (e) {}
  htmx.ajax('GET', '/tags', {
    target: '#sidebar',
    swap: 'innerHTML',
  });
};

window.setSidebarTag = function(tag) {
  try {
    localStorage.setItem('notesview.sidebarMode', 'tag');
    localStorage.setItem('notesview.sidebarTag', tag);
  } catch (e) {}
};

window.setSidebarDir = function(href) {
  try {
    // Extract dir path from /dir/... href
    const dir = href.replace(/^\/dir\//, '');
    localStorage.setItem('notesview.sidebarMode', 'files');
    localStorage.setItem('notesview.sidebarDir', decodeURIComponent(dir));
  } catch (e) {}
};
