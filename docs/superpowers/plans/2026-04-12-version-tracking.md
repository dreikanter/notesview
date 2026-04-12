# Version Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--version` flag, build-time version injection, auto-tagging GitHub Action, and CHANGELOG.md to notesview.

**Architecture:** Version variable in `cmd/notesview/main.go` with `"dev"` default, overridden by `-ldflags` at build time. Cobra's built-in `--version` support handles the flag. GitHub Action auto-tags on merged PRs.

**Tech Stack:** Go (cobra, runtime/debug), Make, GitHub Actions

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `cmd/notesview/main.go` | Modify | Add version variable, init(), debug import |
| `Makefile` | Modify | Add VERSION, LDFLAGS, install, update targets |
| `.github/workflows/tag.yml` | Create | Auto-tag on PR merge |
| `CHANGELOG.md` | Create | Track changes between releases |

---

### Task 1: Add version variable and --version flag

**Files:**
- Modify: `cmd/notesview/main.go`

- [ ] **Step 1: Add version variable, debug import, and init function**

Replace the full contents of `cmd/notesview/main.go` with:

```go
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "notesview",
	Short:         "Markdown notes viewer with live preview",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	rootCmd.Version = version
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify it compiles and --version works**

Run: `go run ./cmd/notesview --version`
Expected output: `notesview version dev`

- [ ] **Step 3: Commit**

```bash
git add cmd/notesview/main.go
git commit -m "Add version variable and --version flag"
```

---

### Task 2: Update Makefile with version injection

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update Makefile**

Replace the full contents of `Makefile` with:

```makefile
BIN := notesview
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build test lint clean assets assets-watch all install update

all: assets build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN) ./cmd/$(BIN)

test:
	go test ./...

lint:
	golangci-lint run ./...

assets:
	npx vite build

assets-watch:
	npx vite build --watch

clean:
	rm -rf $(BUILD_DIR)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

update:
	git checkout main
	git pull --tags
	$(MAKE) install
	@echo "Installed: $$(notesview --version)"
```

- [ ] **Step 2: Verify build injects version**

Run: `make build && bin/notesview --version`
Expected output: `notesview version <git-hash>` (no tags exist yet, so `git describe` returns a commit hash)

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "Add version injection via ldflags to Makefile"
```

---

### Task 3: Add GitHub Action for auto-tagging

**Files:**
- Create: `.github/workflows/tag.yml`

- [ ] **Step 1: Create the workflow file**

Create `.github/workflows/tag.yml`:

```yaml
name: Auto-tag on PR merge

on:
  pull_request:
    types: [closed]
    branches: [main]

jobs:
  tag:
    if: github.event.pull_request.merged == true
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - name: Bump patch version and push tag
        run: |
          LATEST=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)
          if [ -z "$LATEST" ]; then
            NEXT="v0.1.0"
          else
            PATCH=$(echo "$LATEST" | cut -d. -f3)
            NEXT="v0.1.$((PATCH + 1))"
          fi
          git tag "$NEXT"
          git push origin "$NEXT"
          echo "Tagged $NEXT"
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/tag.yml
git commit -m "Add GitHub Action to auto-tag on PR merge"
```

---

### Task 4: Add CHANGELOG.md

**Files:**
- Create: `CHANGELOG.md`

- [ ] **Step 1: Create CHANGELOG.md**

Create `CHANGELOG.md` at project root. The PR number placeholder `N` should be filled in after the PR is created:

```markdown
# Changelog

## [0.1.0] - 2026-04-12

### Added

- Build-time version injection via `-ldflags` ([#N])
- `--version` flag for the CLI ([#N])
- CHANGELOG.md to track changes between releases ([#N])
- GitHub Action to auto-tag on PR merge ([#N])

[0.1.0]: https://github.com/dreikanter/notes-view/releases/tag/v0.1.0
[#N]: https://github.com/dreikanter/notes-view/pull/N
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "Add CHANGELOG.md"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run tests**

Run: `go test ./...`
Expected: all tests pass (no test changes, but verify nothing broke)

- [ ] **Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: no new warnings

- [ ] **Step 3: Verify --version without ldflags**

Run: `go run ./cmd/notesview --version`
Expected: `notesview version dev`

- [ ] **Step 4: Verify --version with ldflags**

Run: `make build && bin/notesview --version`
Expected: `notesview version <git-describe-output>`
