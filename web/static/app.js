(function () {
  'use strict';

  // ---------------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------------
  var currentPath = null;   // current file path (null when browsing a dir)
  var currentSSE = null;    // active EventSource

  // ---------------------------------------------------------------------------
  // Helpers
  // ---------------------------------------------------------------------------

  /**
   * Encode each path segment individually (preserve slashes).
   * @param {string} path
   * @returns {string}
   */
  function encodePathSegments(path) {
    return path.split('/').map(function (seg) {
      return encodeURIComponent(seg);
    }).join('/');
  }

  /**
   * Escape a string for safe HTML insertion.
   * @param {string} str
   * @returns {string}
   */
  function escapeHtml(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  /**
   * Escape a string for use inside an HTML attribute value.
   * @param {string} str
   * @returns {string}
   */
  function escapeAttr(str) {
    return escapeHtml(str);
  }

  // ---------------------------------------------------------------------------
  // Navigation
  // ---------------------------------------------------------------------------

  /**
   * Navigate to a path.  Paths starting with /view/ or /browse/ are handled
   * client-side; everything else falls through to the server.
   * @param {string} path      – URL path e.g. /view/notes/foo.md
   * @param {boolean} pushState – whether to push to history
   */
  function navigate(path, pushState) {
    if (pushState !== false) {
      history.pushState(null, '', path);
    }

    disconnectSSE();

    if (path.startsWith('/view/')) {
      var filePath = path.slice('/view/'.length);
      loadFile(filePath);
    } else if (path.startsWith('/browse/')) {
      var dirPath = path.slice('/browse/'.length);
      loadDir(dirPath);
    } else if (path === '/') {
      // redirect handled server-side; just try browse root
      loadDir('');
    } else {
      loadDir('');
    }
  }

  // ---------------------------------------------------------------------------
  // File view
  // ---------------------------------------------------------------------------

  /**
   * Fetch a file from the /view/ endpoint (JSON) and render it.
   * @param {string} filePath
   */
  function loadFile(filePath) {
    currentPath = filePath;

    var editBtn = document.getElementById('edit-btn');
    if (editBtn) editBtn.style.display = '';

    var content = document.getElementById('content');
    content.innerHTML = '<p class="loading">Loading…</p>';

    var url = '/view/' + encodePathSegments(filePath);
    fetch(url, {
      headers: { 'Accept': 'application/json' }
    })
      .then(function (res) {
        if (!res.ok) {
          return res.text().then(function (t) { throw new Error(t || res.statusText); });
        }
        return res.json();
      })
      .then(function (data) {
        renderFile(data);
        updateBreadcrumbs(filePath);
        highlightSidebarItem(filePath);
        connectSSE(filePath);
      })
      .catch(function (err) {
        content.innerHTML = '<p class="error">Error: ' + escapeHtml(err.message) + '</p>';
      });
  }

  /**
   * Render a ViewResponse object into the #content area.
   * @param {{ html: string, frontmatter: object|null, path: string }} data
   */
  function renderFile(data) {
    var fm = data.frontmatter || {};
    var html = data.html || '';

    // Strip redundant leading <h1> if it matches frontmatter title
    if (fm.title) {
      html = html.replace(
        /^(\s*<h1[^>]*>)\s*(.*?)\s*(<\/h1>)/i,
        function (match, open, text, close) {
          var plain = text.replace(/<[^>]+>/g, '');
          if (plain.trim() === fm.title.trim()) {
            return '';
          }
          return match;
        }
      );
    }

    var parts = [];

    // Frontmatter bar
    var hasFM = fm.title || fm.description || (fm.tags && fm.tags.length);
    if (hasFM) {
      parts.push('<div class="frontmatter-bar">');
      if (fm.title) {
        parts.push('<h1 class="fm-title">' + escapeHtml(fm.title) + '</h1>');
      }
      if (fm.description) {
        parts.push('<p class="fm-description">' + escapeHtml(fm.description) + '</p>');
      }
      if (fm.tags && fm.tags.length) {
        parts.push('<ul class="fm-tags">');
        fm.tags.forEach(function (tag) {
          parts.push('<li class="fm-tag">' + escapeHtml(tag) + '</li>');
        });
        parts.push('</ul>');
      }
      parts.push('</div>');
    }

    parts.push('<div class="markdown-body">' + html + '</div>');

    var content = document.getElementById('content');
    content.innerHTML = parts.join('');

    // Intercept internal /view/ links inside the rendered markdown
    content.querySelectorAll('.markdown-body a[href^="/view/"]').forEach(function (a) {
      a.addEventListener('click', function (e) {
        e.preventDefault();
        navigate(a.getAttribute('href'), true);
      });
    });
  }

  // ---------------------------------------------------------------------------
  // Directory browse
  // ---------------------------------------------------------------------------

  /**
   * Fetch a directory listing from /browse/ and render it.
   * @param {string} dirPath
   */
  function loadDir(dirPath) {
    currentPath = null;

    var editBtn = document.getElementById('edit-btn');
    if (editBtn) editBtn.style.display = 'none';

    var content = document.getElementById('content');
    content.innerHTML = '<p class="loading">Loading…</p>';

    var url = '/browse/' + encodePathSegments(dirPath);
    fetch(url, {
      headers: { 'Accept': 'application/json' }
    })
      .then(function (res) {
        if (!res.ok) {
          return res.text().then(function (t) { throw new Error(t || res.statusText); });
        }
        return res.json();
      })
      .then(function (entries) {
        renderDir(dirPath, entries);
        updateBreadcrumbs(dirPath);
      })
      .catch(function (err) {
        content.innerHTML = '<p class="error">Error: ' + escapeHtml(err.message) + '</p>';
      });
  }

  /**
   * Render an array of BrowseEntry objects.
   * @param {string} dirPath
   * @param {Array<{name:string,isDir:boolean,path:string}>} entries
   */
  function renderDir(dirPath, entries) {
    var parts = ['<div class="dir-listing">'];
    var label = dirPath ? escapeHtml(dirPath) : 'Root';
    parts.push('<h2 class="dir-title">' + label + '</h2>');
    if (!entries || entries.length === 0) {
      parts.push('<p class="empty">No files here.</p>');
    } else {
      parts.push('<ul class="dir-entries">');
      entries.forEach(function (entry) {
        if (entry.isDir) {
          var href = '/browse/' + encodePathSegments(entry.path);
          parts.push('<li class="entry entry-dir"><a href="' + escapeAttr(href) + '" data-link>' +
            '&#128193; ' + escapeHtml(entry.name) + '</a></li>');
        } else {
          var href = '/view/' + encodePathSegments(entry.path);
          parts.push('<li class="entry entry-file"><a href="' + escapeAttr(href) + '" data-link>' +
            '&#128196; ' + escapeHtml(entry.name) + '</a></li>');
        }
      });
      parts.push('</ul>');
    }
    parts.push('</div>');

    var content = document.getElementById('content');
    content.innerHTML = parts.join('');
  }

  // ---------------------------------------------------------------------------
  // Breadcrumbs
  // ---------------------------------------------------------------------------

  /**
   * Build and insert clickable breadcrumb navigation.
   * @param {string} path  – file or dir path relative to root
   */
  function updateBreadcrumbs(path) {
    var nav = document.getElementById('breadcrumbs');
    if (!nav) return;

    var parts = [];

    // Home icon
    parts.push(
      '<a class="breadcrumb" href="/browse/" data-link aria-label="Home">&#8962;</a>'
    );

    if (!path) {
      nav.innerHTML = parts.join('');
      return;
    }

    var segments = path.split('/').filter(Boolean);
    var accumulated = '';
    segments.forEach(function (seg, i) {
      accumulated += (accumulated ? '/' : '') + seg;
      var isLast = (i === segments.length - 1);
      parts.push('<span class="breadcrumb-sep">/</span>');
      if (isLast) {
        parts.push('<span class="breadcrumb breadcrumb-current">' + escapeHtml(seg) + '</span>');
      } else {
        var href = '/browse/' + encodePathSegments(accumulated);
        parts.push(
          '<a class="breadcrumb" href="' + escapeAttr(href) + '" data-link>' +
          escapeHtml(seg) + '</a>'
        );
      }
    });

    nav.innerHTML = parts.join('');
  }

  // ---------------------------------------------------------------------------
  // Sidebar
  // ---------------------------------------------------------------------------

  /**
   * Initialise the sidebar: attach toggle handler and load the root tree.
   */
  function loadSidebar() {
    var sidebar = document.getElementById('sidebar');
    var toggle = document.getElementById('sidebar-toggle');
    if (!sidebar || !toggle) return;

    toggle.addEventListener('click', function () {
      sidebar.classList.toggle('hidden');
    });

    sidebar.innerHTML = '<ul class="sidebar-tree"></ul>';
    var root = sidebar.querySelector('.sidebar-tree');
    loadSidebarDir('', root, 0);
  }

  /**
   * Fetch a directory and append tree items into `container`.
   * @param {string} dirPath
   * @param {Element} container  – <ul> element to append into
   * @param {number} depth
   */
  function loadSidebarDir(dirPath, container, depth) {
    var url = '/browse/' + encodePathSegments(dirPath);
    fetch(url, {
      headers: { 'Accept': 'application/json' }
    })
      .then(function (res) { return res.ok ? res.json() : []; })
      .then(function (entries) {
        if (!entries || !entries.length) return;
        entries.forEach(function (entry) {
          var li = document.createElement('li');
          li.className = entry.isDir ? 'sidebar-item sidebar-dir' : 'sidebar-item sidebar-file';
          li.dataset.path = entry.path;

          if (entry.isDir) {
            var arrow = document.createElement('span');
            arrow.className = 'sidebar-arrow';
            arrow.textContent = '▶';
            arrow.setAttribute('aria-hidden', 'true');

            var label = document.createElement('span');
            label.className = 'sidebar-label';
            label.textContent = entry.name;

            var subList = document.createElement('ul');
            subList.className = 'sidebar-subtree hidden';

            var loaded = false;

            function toggleDir() {
              var expanded = !subList.classList.contains('hidden');
              if (expanded) {
                subList.classList.add('hidden');
                arrow.textContent = '▶';
              } else {
                subList.classList.remove('hidden');
                arrow.textContent = '▼';
                if (!loaded) {
                  loaded = true;
                  loadSidebarDir(entry.path, subList, depth + 1);
                }
              }
            }

            arrow.addEventListener('click', function (e) {
              e.stopPropagation();
              toggleDir();
            });
            label.addEventListener('click', function (e) {
              e.stopPropagation();
              toggleDir();
              navigate('/browse/' + encodePathSegments(entry.path), true);
            });

            li.appendChild(arrow);
            li.appendChild(label);
            li.appendChild(subList);
          } else {
            var link = document.createElement('a');
            link.className = 'sidebar-link';
            link.href = '/view/' + encodePathSegments(entry.path);
            link.textContent = entry.name;
            link.dataset.filePath = entry.path;

            link.addEventListener('click', function (e) {
              e.preventDefault();
              navigate('/view/' + encodePathSegments(entry.path), true);
            });

            li.appendChild(link);
          }

          container.appendChild(li);
        });
      })
      .catch(function () { /* sidebar load failure is non-critical */ });
  }

  /**
   * Mark the sidebar item matching `filePath` as active.
   * @param {string} filePath
   */
  function highlightSidebarItem(filePath) {
    var sidebar = document.getElementById('sidebar');
    if (!sidebar) return;

    sidebar.querySelectorAll('.sidebar-item.active').forEach(function (el) {
      el.classList.remove('active');
    });
    sidebar.querySelectorAll('.sidebar-link').forEach(function (a) {
      if (a.dataset.filePath === filePath) {
        a.closest('.sidebar-item').classList.add('active');
      }
    });
  }

  // ---------------------------------------------------------------------------
  // SSE live reload
  // ---------------------------------------------------------------------------

  /**
   * Open an SSE connection watching `filePath` for changes.
   * On a `change` event, re-render the current file.
   * @param {string} filePath
   */
  function connectSSE(filePath) {
    disconnectSSE();
    var url = '/events?watch=' + encodeURIComponent(filePath);
    var es = new EventSource(url);
    currentSSE = es;

    es.addEventListener('message', function (e) {
      try {
        var msg = JSON.parse(e.data);
        if (msg.type === 'change' && currentPath === filePath) {
          loadFile(filePath);
        }
      } catch (err) {
        // ignore parse errors
      }
    });

    es.addEventListener('error', function () {
      // SSE errors are non-fatal; browser will auto-reconnect
    });
  }

  /**
   * Close and discard the current SSE connection (if any).
   */
  function disconnectSSE() {
    if (currentSSE) {
      currentSSE.close();
      currentSSE = null;
    }
  }

  // ---------------------------------------------------------------------------
  // Edit button
  // ---------------------------------------------------------------------------

  function initEditButton() {
    var editBtn = document.getElementById('edit-btn');
    if (!editBtn) return;

    editBtn.style.display = 'none';

    editBtn.addEventListener('click', function () {
      if (!currentPath) return;
      var url = '/api/edit/' + encodePathSegments(currentPath);
      fetch(url, { method: 'POST' })
        .catch(function () { /* best-effort */ });
    });
  }

  // ---------------------------------------------------------------------------
  // Global link interception
  // ---------------------------------------------------------------------------

  function initLinkInterception() {
    document.addEventListener('click', function (e) {
      var a = e.target.closest('a[data-link]');
      if (!a) return;
      var href = a.getAttribute('href');
      if (!href || href.startsWith('http://') || href.startsWith('https://') || href.startsWith('//')) {
        return;
      }
      e.preventDefault();
      navigate(href, true);
    });
  }

  // ---------------------------------------------------------------------------
  // popstate (back / forward)
  // ---------------------------------------------------------------------------

  function initPopState() {
    window.addEventListener('popstate', function () {
      navigate(window.location.pathname, false);
    });
  }

  // ---------------------------------------------------------------------------
  // Init
  // ---------------------------------------------------------------------------

  function init() {
    initEditButton();
    initLinkInterception();
    initPopState();
    loadSidebar();
    navigate(window.location.pathname, false);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }

})();
