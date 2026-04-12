// notesview front-end bootstrap.
//
// Loads HTMX + SSE, runs syntax highlighting on every swap, and owns
// the sidebar toggle (client-side visibility with localStorage +
// on-open sidebar refresh).

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
  wireSidebarToggle();
});

document.body.addEventListener('htmx:afterSwap', function (e) {
  highlightIn(e.target);
});

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
    // Refresh the sidebar for the current note: while hidden, the
    // sidebar's DOM froze at its last render, but the user may have
    // clicked wiki-links and moved to a different note.
    window.htmx && window.htmx.ajax('GET', currentSidebarUrl(), {
      target: '#sidebar',
      swap: 'innerHTML',
    });
  } else {
    // Closing strips ?dir= from the URL (intentional, per spec). No
    // pushState — this is a UI preference, not a navigation event.
    const url = new URL(window.location.href);
    url.searchParams.delete('dir');
    history.replaceState(null, '', url.toString());
  }
}

// currentSidebarUrl builds the URL for refreshing the sidebar for the
// current note. The note path is stashed on <body> by the layout
// template (data-note-path) and re-stashed on #note-card for resilience
// across note-pane swaps.
function currentSidebarUrl() {
  const notePath = (document.body.dataset.notePath || '').replace(/^\/+/, '');
  const parent = notePath ? notePath.replace(/[^/]*$/, '').replace(/\/$/, '') : '';
  const base = notePath ? `/view/${notePath}` : '/';
  return `${base}?dir=${encodeURIComponent(parent)}`;
}
