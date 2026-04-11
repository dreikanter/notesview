# Client-Side Syntax Highlighting

**Issue:** [#2](https://github.com/dreikanter/notes-view/issues/2) — code blocks currently have low contrast and are hard to read. The issue author suggests moving highlighting to the client (e.g. highlight.js) to reduce server-side complexity.

## Goals

- Code blocks render with readable, contrasted colors on the light background.
- Highlighting complexity leaves the Go server; server emits plain `<pre><code class="language-xxx">` markup.
- `go install github.com/dreikanter/notesview/cmd/notesview@latest` continues to work (no change to the install story).
- Tool remains fully offline-capable — nothing is fetched at runtime.

## Non-Goals

- Dark mode or `prefers-color-scheme` support. The rest of the app is light-only; dark mode is a separate concern that should land app-wide.
- Line numbers, copy buttons, or other code-block chrome.
- Moving to binary-release distribution (goreleaser et al.). That is a separate, larger decision.
- Inline code styling beyond `@tailwindcss/typography` defaults.

## Architecture

**Server:** The Go renderer stops highlighting. `internal/renderer/renderer.go` drops `goldmark-highlighting/v2` and, transitively via `go mod tidy`, `alecthomas/chroma/v2`. Goldmark's default fenced-code renderer emits `<pre><code class="language-xxx">…</code></pre>` when GFM is enabled, which is exactly what highlight.js consumes. The HTML contract between server and client is otherwise unchanged; `/view/` still returns `{html, frontmatter, path}`.

**Client:** `app.js` imports highlight.js as an ES module (common language bundle, ~35 languages, ~40 KB gzipped) and a theme stylesheet. After `renderFile()` inserts HTML into `#content`, it calls `hljs.highlightElement()` on each `pre > code` inside `.markdown-body`. The same path runs on SSE-triggered re-render, because SSE calls back into `loadFile()` → `renderFile()`.

**Build pipeline:** Vite replaces the current Tailwind CLI invocation. Vite handles both the JS bundle (with hljs tree-shaken in) and the CSS (Tailwind + hljs theme, concatenated into a single `style.css`). Output lands in `web/static/` with fixed filenames (no content hashing), and those build artifacts are committed so `go install` works without a Node toolchain.

## Source tree

```
web/
├── src/
│   ├── app.js         # ES module source, imports hljs
│   ├── input.css      # Tailwind entry, imports hljs theme CSS
│   └── index.html     # Vite entry point
├── static/            # Vite build output, committed, embedded by Go
│   ├── app.js
│   ├── style.css
│   └── index.html
└── embed.go           # unchanged: //go:embed static/*
```

`index.html` references `/static/style.css` and `/static/app.js` with those exact paths. Vite is configured with `base: '/static/'` and fixed output filenames, so no reference rewriting is required.

## Server changes

**`internal/renderer/renderer.go`**

- Remove the `goldmark-highlighting/v2` import.
- Remove the `highlighting.NewHighlighting(highlighting.WithStyle("github"))` option from `goldmark.New`.
- All other renderer behavior (frontmatter parsing, note-link processing, task-syntax processing) is unchanged.

**`go.mod`**

- After `go mod tidy`: `github.com/yuin/goldmark-highlighting/v2` removed (direct); `github.com/alecthomas/chroma/v2` removed (indirect).

**Tests**

- Existing `notelinks_test.go` and `tasks_test.go` are unaffected.
- Add one small test in `internal/renderer/` that feeds a fenced ```go block to `Render` and asserts the output contains `<code class="language-go">`. This locks in the language-class contract that highlight.js depends on.

## Client changes

**`web/src/app.js`** (moved from `web/static/app.js`)

- Top of file:
  ```js
  import hljs from 'highlight.js/lib/common';
  import 'highlight.js/styles/github.css';
  ```
- The existing IIFE wrapper is removed; Vite handles module scoping.
- In `renderFile()`, immediately after setting `content.innerHTML`, before wiring internal-link handlers:
  ```js
  content.querySelectorAll('.markdown-body pre > code').forEach(function (el) {
    hljs.highlightElement(el);
  });
  ```
- Fenced blocks with no language hint receive `<code>` with no class; `highlightElement` falls back to auto-detection. Blocks with an unknown language are left as-is (hljs logs a warning, no throw). Both cases render as plain monospace text inside the themed `<pre>`, which is the acceptable fallback the issue asks for.

**`web/src/input.css`**

- Delete the `.chroma` and `.chroma pre` rules.
- Add wrapper rules so that `<pre>` gets the look previously provided by `.chroma` (the hljs `github.css` theme only colors tokens, it does not style the surrounding `<pre>`):
  ```css
  .markdown-body pre {
    @apply bg-gray-50 border border-gray-200 rounded-md p-4 overflow-auto mb-4 text-[85%] leading-snug;
  }
  .markdown-body code {
    @apply font-mono;
  }
  ```
- Inline `<code>` inside prose continues to use `@tailwindcss/typography` defaults.

**`tailwind.config.js`**

- Content globs change from `./web/static/index.html`, `./web/static/app.js` to `./web/src/**/*.{html,js}`.
- Safelist (`broken-link`, `uid-link`, `task-*`) is unchanged.
- No hljs classes need safelisting; `github.css` ships its own rules and is imported through PostCSS, not generated by Tailwind.

## Build pipeline

**`package.json` devDependencies** (new set):

- `vite`
- `tailwindcss` (v3 major, unchanged)
- `@tailwindcss/typography` (unchanged)
- `postcss`
- `autoprefixer`
- `highlight.js`

**`vite.config.js`** (new):

- `root: 'web/src'`
- `base: '/static/'`
- `build.outDir: '../static'`
- `build.emptyOutDir: true`
- `build.minify: true`
- `build.sourcemap: false`
- Fixed output filenames (`app.js`, `style.css`) via `rollupOptions.output.entryFileNames` / `assetFileNames`; no content hashing.
- Single entry: `web/src/index.html`. Vite discovers `app.js` and the CSS link from the HTML.

**`postcss.config.js`** (new): PostCSS with `tailwindcss` and `autoprefixer` plugins.

**`Makefile`**

- `CSS_SRC` / `CSS_OUT` variables removed.
- `css` / `css-watch` targets replaced by:
  ```
  assets:       npx vite build
  assets-watch: npx vite build --watch
  ```
- `all: assets build` (was `all: css build`).
- `make build` continues to only run `go build`; full build goes through `make all`.

## Install and distribution

Unchanged. `web/static/app.js`, `web/static/style.css`, `web/static/index.html` are committed to the repository. `go install github.com/dreikanter/notesview/cmd/notesview@latest` continues to work because `go:embed static/*` finds the files in the fetched module source. The tradeoff — minified blobs churn in git history — is the same tradeoff the project accepts today for the Tailwind-built `style.css`.

## Migration order

The order matters because the committed-artifacts invariant must hold at every commit:

1. Add Vite + hljs to `package.json`. Create `vite.config.js`, `postcss.config.js`.
2. Move `web/static/app.js` → `web/src/app.js` (convert to ES module). Move `web/static/index.html` → `web/src/index.html`.
3. Update `tailwind.config.js` content globs.
4. Add hljs import and `highlightElement` call in `app.js`. Replace `.chroma` CSS with new `pre`/`code` rules in `input.css`.
5. Update `Makefile` (`assets`, `assets-watch`, `all`).
6. Run `npx vite build`. Commit the new `web/static/{app.js,style.css,index.html}`.
7. Update `internal/renderer/renderer.go` to drop `goldmark-highlighting`. Run `go mod tidy`. Add the language-class renderer test.
8. Update `README.md` Development section: `make assets` replaces `make css`.

## Verification

- `go install` from a clean checkout at the final commit produces a working binary.
- `make all` on a fresh clone succeeds after `npm install`.
- A `.md` fixture with ``` ```go, ```python, ```bash, ```json, ```yaml, and an unlabeled ``` block renders with readable colors consistent with the `github` hljs theme.
- SSE live-reload continues to re-highlight after a file edit.
- `go test ./...` passes, including the new renderer test.
- `golangci-lint run ./...` is clean.

## Risks

- **hljs bundle size:** adds roughly 40 KB gzipped to `app.js`. Irrelevant over localhost.
- **Auto-detect misfires:** unlabeled fences can occasionally be miscategorized. Worst-case output is a differently-tinted but still monospace block — acceptable, and it is strictly better than the current state.
- **Vite HTML processing:** Vite rewrites asset references in `index.html` by default. `base: '/static/'` plus fixed filenames keeps the final output compatible with the existing server routes.
- **Committed-artifact drift:** a contributor can forget to run `npx vite build` before committing. Mitigation: CI runs `make all` and fails if `git diff --exit-code web/static/` is non-empty. (Out of scope for this spec if no CI exists yet; worth a follow-up.)
