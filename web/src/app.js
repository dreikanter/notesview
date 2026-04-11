// notesview front-end bootstrap.
//
// The server renders all markup via Go html/template; this file exists
// only to load htmx and its SSE extension, and to run syntax highlighting
// on every swap. Index-panel state lives entirely in the URL (?index=dir)
// so navigation via hx-boost carries it without any client-side storage.

import 'htmx.org';
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
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
});
