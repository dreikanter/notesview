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
  var link = sidebar.querySelector('.sidebar-link[data-file-path="' + path + '"]');
  if (link) {
    var item = link.closest('.sidebar-item');
    if (item) item.classList.add('active');
  }
}

// Event delegation for the sidebar toggle: the button is inside the header
// which hx-boost replaces on navigation, so a direct listener would be lost.
document.addEventListener('click', function (e) {
  if (e.target.closest('#sidebar-toggle')) {
    var sidebar = document.getElementById('sidebar');
    if (sidebar) sidebar.classList.toggle('hidden');
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
