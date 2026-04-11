// notesview front-end bootstrap.
//
// The server renders all markup via Go html/template; this file exists
// only to:
//   1. load htmx and its SSE extension,
//   2. run syntax highlighting on every swap,
//   3. keep the sidebar toggle and active-item highlighting in sync
//      across hx-boost navigation.

import 'htmx.org';
import 'htmx-ext-sse';
import hljs from 'highlight.js/lib/common';

function highlightIn(root) {
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el);
  });
}

function currentFilePath() {
  var content = document.getElementById('content');
  return (content && content.dataset.filePath) || '';
}

function refreshSidebarActive() {
  var sidebar = document.getElementById('sidebar');
  if (!sidebar) return;
  sidebar.querySelectorAll('.sidebar-item.active').forEach(function (el) {
    el.classList.remove('active');
  });
  var path = currentFilePath();
  if (!path) return;
  // Comparing dataset.filePath directly avoids building a CSS selector
  // from a path that may contain ", \, ] or other characters that break
  // attribute selectors.
  var link = null;
  var links = sidebar.querySelectorAll('.sidebar-link');
  for (var i = 0; i < links.length; i++) {
    if (links[i].dataset.filePath === path) {
      link = links[i];
      break;
    }
  }
  if (!link) return;
  var item = link.closest('.sidebar-item');
  if (item) item.classList.add('active');
  // Open every ancestor <details> so deep-linked notes remain visible
  // inside collapsed folders.
  var details = link.closest('details');
  while (details) {
    details.open = true;
    var parent = details.parentElement;
    details = parent ? parent.closest('details') : null;
  }
}

// Event delegation for the sidebar toggle: the button is inside the header
// which hx-boost replaces on navigation, so a direct listener would be lost.
document.addEventListener('click', function (e) {
  if (e.target.closest('#sidebar-toggle')) {
    var sidebar = document.getElementById('sidebar');
    if (sidebar) sidebar.classList.toggle('sidebar-collapsed');
  }
});

document.addEventListener('DOMContentLoaded', function () {
  highlightIn(document);
  refreshSidebarActive();
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
  refreshSidebarActive();
});
