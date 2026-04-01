# Tailwind CSS Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hand-written `web/static/style.css` with a Tailwind-based stylesheet using `@layer components` + `@apply`, and `@tailwindcss/typography` for markdown content.

**Architecture:** All styling lives in `web/src/input.css` using `@apply` with Tailwind tokens — no inline utility classes in HTML or JS. The typography plugin (`prose`) handles all markdown-rendered HTML. Go-renderer classes (`.broken-link`, `.chroma`, etc.) are defined in `@layer components` with `@apply` since they are emitted by the server and cannot use inline utilities. The generated `web/static/style.css` is committed so `go build` works without Tailwind installed.

**Tech Stack:** Tailwind CSS v3 (npm), `@tailwindcss/typography` v0.5, Node.js (dev-only), Go embed

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `package.json` | Create | npm dev dependencies (tailwindcss, typography plugin) |
| `tailwind.config.js` | Create | Content scanning, custom breakpoint, typography plugin |
| `web/src/input.css` | Create | Tailwind source — all component definitions |
| `web/static/style.css` | Overwrite | Generated output — committed |
| `web/static/app.js` | Modify | Add `prose max-w-none` to markdown wrapper div |
| `Makefile` | Modify | Add `css`, `css-watch`, `all` targets; update `.PHONY` |
| `.gitignore` | Modify | Add `node_modules/` |

---

## Task 1: npm setup and Tailwind config

**Files:**
- Create: `package.json`
- Create: `tailwind.config.js`
- Modify: `.gitignore`

- [ ] **Step 1: Create `package.json`**

```json
{
  "name": "notesview",
  "version": "0.1.0",
  "private": true,
  "devDependencies": {
    "@tailwindcss/typography": "^0.5.0",
    "tailwindcss": "^3.4.0"
  }
}
```

- [ ] **Step 2: Install dependencies**

```bash
npm install
```

Expected: `node_modules/` created, `package-lock.json` generated.

- [ ] **Step 3: Create `tailwind.config.js`**

```js
/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './web/static/index.html',
    './web/static/app.js',
  ],
  theme: {
    extend: {
      screens: {
        sidebar: '900px',
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
}
```

- [ ] **Step 4: Add `node_modules/` to `.gitignore`**

Add to `.gitignore`:
```
node_modules/
```

- [ ] **Step 5: Commit**

```bash
git add package.json package-lock.json tailwind.config.js .gitignore
git commit -m "build: add Tailwind npm setup and config"
```

---

## Task 2: Makefile CSS targets

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update `Makefile`**

```makefile
BIN := notesview
BUILD_DIR := bin
CSS_SRC := web/src/input.css
CSS_OUT := web/static/style.css

.PHONY: build test lint clean css css-watch all

all: css build

build:
	go build -o $(BUILD_DIR)/$(BIN) ./cmd/$(BIN)

test:
	go test ./...

lint:
	golangci-lint run ./...

css:
	npx tailwindcss -i $(CSS_SRC) -o $(CSS_OUT) --minify

css-watch:
	npx tailwindcss -i $(CSS_SRC) -o $(CSS_OUT) --watch

clean:
	rm -rf $(BUILD_DIR)
```

- [ ] **Step 2: Create source directory**

```bash
mkdir -p web/src
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: add css and css-watch Makefile targets"
```

---

## Task 3: Create `input.css` — base + app chrome

**Files:**
- Create: `web/src/input.css`
- Overwrite: `web/static/style.css`

- [ ] **Step 1: Create `web/src/input.css` with directives and app chrome**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer components {
  /* =====================================================
     App Chrome
     ===================================================== */

  #topbar {
    @apply fixed top-0 left-0 right-0 h-12 bg-white border-b border-gray-200
           flex items-center px-3 gap-2 z-[100];
  }

  #sidebar-toggle {
    @apply bg-transparent border-0 cursor-pointer px-2 py-1.5 text-lg
           text-gray-500 rounded-md leading-none flex-shrink-0
           hover:bg-gray-50 hover:text-gray-900;
  }

  #edit-btn {
    @apply bg-transparent border border-gray-300 rounded-md cursor-pointer
           px-3 py-1 text-[13px] text-gray-900 font-sans flex-shrink-0
           hover:bg-gray-50 hover:border-gray-200;
  }

  #sidebar {
    @apply fixed top-12 left-0 bottom-0 w-[280px] bg-white border-r border-gray-200
           overflow-y-auto overflow-x-hidden transition-transform duration-200
           ease-in-out z-[90] py-2;
  }

  #sidebar.hidden {
    @apply -translate-x-full;
  }

  #content {
    @apply mt-12 px-6 py-8 max-w-[900px] mx-auto min-h-[calc(100vh-3rem)];
  }

  @media (max-width: 768px) {
    #content {
      @apply px-4 py-5;
    }
  }

  @media (max-width: 480px) {
    #content {
      @apply px-3 py-4;
    }
  }

  @media (min-width: 900px) {
    #sidebar:not(.hidden) ~ #content {
      margin-left: 280px;
    }
  }
}
```

- [ ] **Step 2: Generate CSS**

```bash
make css
```

Expected: `web/static/style.css` written, no errors.

- [ ] **Step 3: Verify Go build embeds it**

```bash
make build
```

Expected: `bin/notesview` built successfully.

- [ ] **Step 4: Commit**

```bash
git add web/src/input.css web/static/style.css
git commit -m "style: add Tailwind input.css with app chrome styles"
```

---

## Task 4: Breadcrumbs + sidebar tree

**Files:**
- Modify: `web/src/input.css`

- [ ] **Step 1: Add breadcrumb and sidebar tree styles to `@layer components` in `input.css`**

Append inside the `@layer components { }` block, after the app chrome section:

```css
  /* =====================================================
     Breadcrumbs
     ===================================================== */

  #breadcrumbs {
    @apply flex-1 flex items-center gap-1 text-sm text-gray-500
           overflow-hidden whitespace-nowrap;
  }

  .breadcrumb {
    @apply text-blue-600 no-underline hover:underline;
  }

  .breadcrumb-sep {
    @apply text-gray-400 select-none;
  }

  .breadcrumb-current {
    @apply text-gray-900 overflow-hidden text-ellipsis;
  }

  /* =====================================================
     Sidebar Tree
     ===================================================== */

  .sidebar-tree {
    @apply list-none m-0 p-0;
  }

  .sidebar-item {
    @apply flex items-center gap-1 px-2 py-0.5 text-[13px] cursor-pointer
           text-gray-900 whitespace-nowrap overflow-hidden text-ellipsis
           rounded-md mx-1 my-px hover:bg-gray-50;
  }

  .sidebar-item.active {
    @apply bg-gray-50 font-semibold;
  }

  .sidebar-arrow {
    @apply flex-shrink-0 w-4 text-[10px] text-gray-500 select-none
           cursor-pointer text-center hover:text-gray-900;
  }

  .sidebar-label {
    @apply overflow-hidden text-ellipsis cursor-pointer;
  }

  .sidebar-link {
    @apply text-gray-900 no-underline overflow-hidden text-ellipsis
           hover:text-blue-600;
  }

  .sidebar-subtree {
    @apply list-none m-0 p-0 pl-4;
  }

  .sidebar-subtree.hidden {
    display: none;
  }
```

- [ ] **Step 2: Regenerate CSS**

```bash
make css
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/input.css web/static/style.css
git commit -m "style: add breadcrumb and sidebar tree styles"
```

---

## Task 5: Frontmatter bar + directory listing

**Files:**
- Modify: `web/src/input.css`

- [ ] **Step 1: Append to `@layer components` in `input.css`**

```css
  /* =====================================================
     Frontmatter Bar
     ===================================================== */

  .frontmatter-bar {
    @apply pb-4 mb-6 border-b border-gray-200;
  }

  .fm-title {
    @apply text-[28px] font-semibold leading-tight text-gray-900 mt-0 mb-2;
  }

  .fm-description {
    @apply text-[15px] text-gray-500 mt-0 mb-3;
  }

  .fm-tags {
    @apply flex flex-wrap gap-1.5 m-0 p-0 list-none;
  }

  .fm-tag {
    @apply inline-flex items-center bg-blue-100 text-blue-600 text-xs
           font-medium px-2 py-0.5 rounded-full no-underline leading-relaxed
           hover:bg-blue-200;
  }

  /* =====================================================
     Directory Listing
     ===================================================== */

  .dir-listing {
    @apply border border-gray-200 rounded-md overflow-hidden mb-4;
  }

  .dir-title {
    @apply bg-gray-50 px-4 py-2 text-base font-semibold text-gray-900
           border-b border-gray-200 m-0;
  }

  .dir-entries {
    @apply list-none m-0 p-0;
  }

  .entry {
    @apply border-b border-gray-100 last:border-b-0;
  }

  .entry a {
    @apply flex items-center gap-2 px-4 py-2 text-sm text-blue-600
           no-underline transition-colors duration-100 hover:bg-gray-50;
  }

  .entry-dir a {
    @apply font-medium;
  }

  .empty {
    @apply px-4 py-6 text-gray-500 text-center;
  }
```

- [ ] **Step 2: Regenerate CSS**

```bash
make css
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/input.css web/static/style.css
git commit -m "style: add frontmatter bar and directory listing styles"
```

---

## Task 6: Loading/error states

**Files:**
- Modify: `web/src/input.css`

- [ ] **Step 1: Append to `@layer components` in `input.css`**

```css
  /* =====================================================
     Loading / Error States
     ===================================================== */

  .loading {
    @apply flex items-center justify-center py-12 px-6 text-sm text-gray-500;
  }

  .error {
    @apply flex flex-col items-center justify-center py-16 px-6
           text-center text-red-600;
  }

  .error-page {
    @apply flex flex-col items-center justify-center py-16 px-6 text-center;
  }

  .error-code {
    @apply text-5xl font-semibold text-gray-900 mb-2;
  }

  .error-message {
    @apply text-lg text-gray-500 mb-6;
  }

  .error-page a {
    @apply text-blue-600 no-underline hover:underline;
  }
```

- [ ] **Step 2: Regenerate CSS**

```bash
make css
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/input.css web/static/style.css
git commit -m "style: add loading and error state styles"
```

---

## Task 7: Go-renderer classes

These classes are emitted by the Go server into rendered HTML. They cannot use inline utilities and must be defined here.

**Files:**
- Modify: `web/src/input.css`

- [ ] **Step 1: Append to `@layer components` in `input.css`**

```css
  /* =====================================================
     Go-Renderer Classes
     (emitted by server — cannot use inline utilities)
     ===================================================== */

  /* Broken wiki links */
  .broken-link {
    @apply text-red-600 line-through;
  }

  .broken-link:hover {
    @apply underline;
  }

  /* UID auto-links */
  .uid-link {
    @apply text-blue-600 font-mono text-[0.9em];
  }

  /* Task checkboxes */
  .task-checked {
    @apply inline-flex items-center justify-center w-4 h-4 bg-green-700
           text-white rounded-sm text-[11px] font-bold align-middle
           mr-0.5 flex-shrink-0;
  }

  .task-unchecked {
    @apply inline-block w-4 h-4 border-2 border-gray-300 rounded-sm
           align-middle mr-0.5 bg-white flex-shrink-0;
  }

  .task-tag {
    @apply inline-flex items-center bg-gray-50 text-gray-500 border
           border-gray-200 text-[11px] font-medium px-1.5 py-px
           rounded-full align-middle font-sans;
  }

  /* Goldmark chroma syntax highlighting wrapper */
  .chroma {
    @apply bg-gray-50 border border-gray-200 rounded-md p-4 overflow-auto
           mb-4 font-mono text-[85%] leading-snug;
  }

  .chroma pre {
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    overflow: visible;
  }

  /* Override prose link styles for special renderer links */
  .prose .broken-link {
    @apply text-red-600 line-through no-underline;
  }

  .prose .broken-link:hover {
    @apply underline;
  }

  .prose .uid-link {
    @apply text-blue-600 font-mono text-[0.9em] no-underline;
  }
```

- [ ] **Step 2: Regenerate CSS**

```bash
make css
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/input.css web/static/style.css
git commit -m "style: add Go-renderer class styles (broken-link, chroma, task-*)"
```

---

## Task 8: Update `app.js` — add prose class to markdown wrapper

**Files:**
- Modify: `web/static/app.js`

- [ ] **Step 1: Find the markdown wrapper in `app.js`**

Search for the line that creates the markdown body div (around line 161):
```js
parts.push('<div class="markdown-body">' + html + '</div>');
```

- [ ] **Step 2: Add `prose max-w-none` to the wrapper**

```js
parts.push('<div class="markdown-body prose max-w-none">' + html + '</div>');
```

- [ ] **Step 3: Regenerate CSS**

`prose` and `max-w-none` are now in `app.js` content, so Tailwind will include them:

```bash
make css
```

Expected: no errors, generated CSS is slightly larger (typography plugin styles included).

- [ ] **Step 4: Build and run smoke test**

```bash
make build
go test ./...
```

Expected: all tests pass, binary builds.

- [ ] **Step 5: Commit**

```bash
git add web/static/app.js web/static/style.css
git commit -m "style: apply prose typography to markdown body"
```

---

## Task 9: Remove old CSS and final cleanup

**Files:**
- Verify: `web/static/style.css` contains no references to old CSS variables
- Verify: `web/src/input.css` is complete

- [ ] **Step 1: Confirm `web/static/style.css` is fully generated (no hand-written content)**

```bash
head -5 web/static/style.css
```

Expected: minified Tailwind output starting with `*,::after,::before{...}` or similar — not the old `/* notesview — GitHub-style CSS */` header.

- [ ] **Step 2: Run full build and tests**

```bash
make all
go test ./...
```

Expected: CSS generated, binary built, all tests pass.

- [ ] **Step 3: Commit `web/src/` and final state**

```bash
git add web/src/ web/static/style.css
git commit -m "style: complete Tailwind migration — remove hand-written CSS"
```

---

## Verification Checklist (manual, in browser)

Start the server and verify visually:

```bash
./bin/notesview serve --path .
```

- [ ] Top bar renders correctly (fixed, border, buttons)
- [ ] Sidebar opens/closes with toggle button
- [ ] Sidebar highlights active file
- [ ] Breadcrumbs show correct path with separators
- [ ] Directory listing shows bordered rows
- [ ] Markdown renders with typography styles (headings, lists, code blocks)
- [ ] Code blocks have syntax highlighting via chroma
- [ ] Frontmatter title/description/tags render correctly
- [ ] Tags show as blue pills
- [ ] Sidebar pushes content on wide screens (≥900px)
- [ ] Mobile layout adjusts padding at 768px
