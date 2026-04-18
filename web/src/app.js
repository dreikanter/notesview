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
    el.classList.remove('selected', 'bg-blue-100', 'border-blue-300', 'text-blue-700');
  });
}

function markSelected(selector) {
  clearSelected();
  var el = document.querySelector(selector);
  if (el) {
    el.classList.add('selected', 'bg-blue-100', 'border-blue-300', 'text-blue-700');
  }
}

// Set by deliberate navigation (selectNote/Dir/Tag) so the next swap that
// finds the selected element scrolls it into view. Sidebar-only actions
// like toggleDir leave it unset, so they don't hijack the viewport.
var pendingScrollToSelected = false;

// --- Directory navigation ---

window.selectDir = function(href, skipPush, fromSidebar) {
  setLS('selected', href);
  if (!fromSidebar) pendingScrollToSelected = true;

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'dir', href: href }, '', href);

  // Load listing in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Sidebar tree manipulation is the chevron's job. A label click
  // originating inside the sidebar leaves the tree alone so the clicked
  // item stays exactly where the cursor is. Navigations from the main
  // pane (or popstate) still resync the sidebar to the new location.
  if (!fromSidebar) {
    var dirPath = href.replace(/^\/dir\//, '');
    setLS('filesDir', decodeURIComponent(dirPath));
    htmx.ajax('GET', href, {
      target: '#files-content',
      swap: 'innerHTML',
    });
  }

  // Ensure files section is visible
  var content = document.getElementById('files-content');
  var disclosure = document.getElementById('files-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('filesOpen', '1');
};

// Chevron click: toggle a directory's expanded state in the sidebar only.
// Does not touch the URL or the main pane — this is pure tree manipulation,
// independent from the label-click navigation handled by selectDir.
window.toggleDir = function(href, isExpanded) {
  var dirPath = decodeURIComponent(href.replace(/^\/dir\//, ''));
  var targetPath;
  if (isExpanded) {
    var slash = dirPath.lastIndexOf('/');
    targetPath = slash < 0 ? '' : dirPath.substring(0, slash);
  } else {
    targetPath = dirPath;
  }
  setLS('filesDir', targetPath);
  var targetHref = '/dir/' + encodePath(targetPath);
  htmx.ajax('GET', targetHref, {
    target: '#files-content',
    swap: 'innerHTML',
  });
};

// --- Tag navigation ---

window.selectTag = function(tag, skipPush, fromSidebar) {
  var href = '/tags/' + encodeURIComponent(tag);
  setLS('tagsTag', tag);
  setLS('selected', href);
  if (!fromSidebar) pendingScrollToSelected = true;

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'tag', tag: tag, href: href }, '', href);

  // Load tagged notes in main panel only — sidebar stays as flat tag list
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Ensure tags section is visible and highlight the selected tag
  var content = document.getElementById('tags-content');
  var disclosure = document.getElementById('tags-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('tagsOpen', '1');
};

// --- Note navigation ---

var pendingNoteScrollReset = false;

window.selectNote = function(href, skipPush, fromSidebar) {
  setLS('selected', href);
  if (!fromSidebar) pendingScrollToSelected = true;

  // Push browser URL
  if (!skipPush) history.pushState({ type: 'note', href: href }, '', href);

  // Flag a scroll reset for the upcoming #note-pane swap. The reset runs
  // in htmx:afterSwap so it lands in the same paint as the new content —
  // resetting before the swap causes a visible jump on the old note.
  // SSE-driven same-note swaps do not set the flag and preserve scroll.
  pendingNoteScrollReset = true;

  // Load note in main panel
  htmx.ajax('GET', href, {
    target: '#note-pane',
    swap: 'innerHTML',
    headers: { 'HX-Target': 'note-pane' },
  });

  // Expand the note's parent directory in the sidebar so the note
  // appears highlighted among its siblings.
  var notePath = href.replace(/^\/view\//, '');
  var parts = decodeURIComponent(notePath).split('/');
  if (parts.length > 1) {
    // Has a parent directory — expand it
    var parentDir = parts.slice(0, -1).join('/');
    setLS('filesDir', parentDir);
    var dirHref = '/dir/' + encodePath(parentDir);
    htmx.ajax('GET', dirHref, {
      target: '#files-content',
      swap: 'innerHTML',
    });
  }

  // Ensure files section is visible
  var content = document.getElementById('files-content');
  var disclosure = document.getElementById('files-disclosure');
  if (content) content.style.display = '';
  if (disclosure) disclosure.textContent = '\u25BE';
  setLS('filesOpen', '1');
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

// --- Event delegation for entry links ---
// Uses data-action attributes instead of inline onclick handlers,
// avoiding escaping issues with special characters in names/paths.

document.addEventListener('click', function(e) {
  var link = e.target.closest('[data-action]');
  if (!link) return;
  e.preventDefault();
  // Clicks originating inside the sidebar should not scroll the sidebar
  // to center the clicked item — the user already has it under the cursor.
  var fromSidebar = !!e.target.closest('#sidebar');
  var action = link.dataset.action;
  if (action === 'selectTag') {
    selectTag(link.dataset.entryName, false, fromSidebar);
  } else if (action === 'selectDir') {
    selectDir(link.dataset.entryHref, false, fromSidebar);
  } else if (action === 'selectNote') {
    selectNote(link.dataset.entryHref, false, fromSidebar);
  } else if (action === 'toggleDir') {
    toggleDir(link.dataset.entryHref, link.dataset.expanded === '1');
  }
});

// --- Selection highlight after HTMX swaps ---

document.body.addEventListener('htmx:afterSwap', function(e) {
  highlightIn(e.target);

  if (pendingNoteScrollReset && e.target && e.target.id === 'note-pane') {
    e.target.scrollTop = 0;
    pendingNoteScrollReset = false;
  }

  // Re-apply selection highlight after any swap. Scroll into view only
  // when a deliberate navigation flagged it — toggleDir swaps must not
  // yank the viewport away from where the user's cursor is.
  var selected = getLS('selected', '');
  if (selected) {
    setTimeout(function() {
      markSelected('[data-entry-href="' + selected + '"]');
      if (pendingScrollToSelected) {
        var el = document.querySelector('[data-entry-href="' + selected + '"]');
        if (el) {
          el.scrollIntoView({ block: 'center', inline: 'nearest' });
          pendingScrollToSelected = false;
        }
      }
    }, 0);
  }
});
