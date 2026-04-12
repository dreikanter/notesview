// notesview front-end bootstrap.
//
// Loads HTMX + SSE, runs syntax highlighting on every swap, and owns
// the sidebar toggle and sidebar tree state.

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

// --- Sidebar tree state ---

function getLS(key, fallback) {
  try { return localStorage.getItem('notesview.' + key) || fallback; } catch (e) { return fallback; }
}

function setLS(key, value) {
  try { localStorage.setItem('notesview.' + key, value); } catch (e) {}
}

function encodePath(p) {
  if (!p) return '';
  return p.split('/').map(encodeURIComponent).join('/');
}

// --- Section collapse/expand ---

window.toggleSection = function(name) {
  var content = document.getElementById(name + '-content');
  var disclosure = document.getElementById(name + '-disclosure');
  var btn = document.querySelector('[aria-controls="' + name + '-content"]');
  if (!content) return;
  var isOpen = content.style.display !== 'none';
  content.style.display = isOpen ? 'none' : '';
  content.setAttribute('aria-hidden', isOpen ? 'true' : 'false');
  if (btn) btn.setAttribute('aria-expanded', isOpen ? 'false' : 'true');
  if (disclosure) disclosure.textContent = isOpen ? '\u25B8' : '\u25BE';
  setLS(name + 'Open', isOpen ? '0' : '1');
};

function restoreSectionState(name) {
  var open = getLS(name + 'Open', '1');
  var content = document.getElementById(name + '-content');
  var disclosure = document.getElementById(name + '-disclosure');
  var btn = document.querySelector('[aria-controls="' + name + '-content"]');
  if (!content) return;
  if (open === '0') {
    content.style.display = 'none';
    content.setAttribute('aria-hidden', 'true');
    if (btn) btn.setAttribute('aria-expanded', 'false');
    if (disclosure) disclosure.textContent = '\u25B8';
  }
}

// --- Selection highlight ---

function clearSelected() {
  document.querySelectorAll('.entry-link.selected').forEach(function(el) {
    el.classList.remove('selected', 'bg-blue-50', 'border-blue-200');
  });
}

function markSelected(selector) {
  clearSelected();
  var el = document.querySelector(selector);
  if (el) el.classList.add('selected', 'bg-blue-50', 'border-blue-200');
}

// --- Directory navigation ---

window.selectDir = function(href, skipPush) {
  var dirPath = href.replace(/^\/dir\//, '');
  setLS('filesDir', decodeURIComponent(dirPath));
  setLS('selected', href);

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'dir', href: href }, '', href);

  // Load listing in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Load tree in sidebar files section
  htmx.ajax('GET', href, {
    target: '#files-content',
    swap: 'innerHTML',
  });

  // Ensure files section is visible
  var content = document.getElementById('files-content');
  var disclosure = document.getElementById('files-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('filesOpen', '1');
};

// --- Tag navigation ---

window.selectTag = function(tag, skipPush) {
  var href = '/tags/' + encodeURIComponent(tag);
  setLS('tagsTag', tag);
  setLS('selected', href);

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'tag', tag: tag, href: href }, '', href);

  // Load listing in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Load tree in sidebar tags section
  htmx.ajax('GET', href, {
    target: '#tags-content',
    swap: 'innerHTML',
  });

  // Ensure tags section is visible
  var content = document.getElementById('tags-content');
  var disclosure = document.getElementById('tags-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('tagsOpen', '1');
};

// --- Note navigation ---

window.selectNote = function(href, skipPush) {
  setLS('selected', href);

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'note', href: href }, '', href);

  // Load note in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });
};

// --- Browser back/forward ---

window.addEventListener('popstate', function(e) {
  var state = e.state;
  if (state && state.type === 'dir') {
    selectDir(state.href, true);
  } else if (state && state.type === 'tag') {
    selectTag(state.tag, true);
  } else if (state && state.type === 'note') {
    selectNote(state.href, true);
  } else {
    // Fallback: parse the current URL
    var path = location.pathname;
    if (path.indexOf('/view/') === 0) {
      selectNote(path, true);
    } else if (path.indexOf('/dir/') === 0) {
      selectDir(path, true);
    } else if (path.indexOf('/tags/') === 0) {
      var tag = decodeURIComponent(path.replace(/^\/tags\//, ''));
      selectTag(tag, true);
    }
  }
});

// --- Restore state ---

function refreshSidebar() {
  var filesDir = getLS('filesDir', '');
  var filesUrl = '/dir/' + encodePath(filesDir);
  htmx.ajax('GET', filesUrl, {
    target: '#files-content',
    swap: 'innerHTML',
  });

  var tagsTag = getLS('tagsTag', '');
  var tagsUrl = tagsTag ? '/tags/' + encodeURIComponent(tagsTag) : '/tags';
  htmx.ajax('GET', tagsUrl, {
    target: '#tags-content',
    swap: 'innerHTML',
  });
}

function restoreSidebarState() {
  restoreSectionState('files');
  restoreSectionState('tags');
}

// --- Selection highlight after HTMX swaps ---

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target);

  // Re-apply selection highlight after any swap
  var selected = getLS('selected', '');
  if (selected) {
    setTimeout(function() {
      markSelected('[data-entry-href="' + selected + '"]');
    }, 0);
  }
});
