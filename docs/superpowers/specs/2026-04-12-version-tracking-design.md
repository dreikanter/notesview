# Version Tracking Design

Add version tracking, `--version` flag, and changelog to notesview, matching the pattern established in [notesctl](https://github.com/dreikanter/notesctl).

## Version Variable & Flag

Add to `cmd/notesview/main.go`:

```go
var version = "dev"

func init() {
    if version == "dev" {
        if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
            version = info.Main.Version
        }
    }
    rootCmd.Version = version
}
```

- `version` defaults to `"dev"` for source builds without tags
- `debug.ReadBuildInfo()` fallback covers `go install` from module cache
- `rootCmd.Version` gives Cobra's built-in `--version` flag automatically
- Build-time override via `-ldflags "-X main.version=..."`

Output: `notesview version v0.1.0`

## Makefile

Update to inject version at build time:

```makefile
BIN := notesview
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN) ./cmd/$(BIN)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

update:
	git checkout main
	git pull --tags
	$(MAKE) install
	@echo "Installed: $$(notesview --version)"
```

- `VERSION` from `git describe --tags --always --dirty`, falling back to `dev`
- `-ldflags` added to `build` target
- New `install` and `update` targets matching [notesctl](https://github.com/dreikanter/notesctl)

## GitHub Action: Auto-Tag on PR Merge

New file `.github/workflows/tag.yml`:

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

Identical to [notesctl](https://github.com/dreikanter/notesctl). Auto-increments patch version on every merged PR.

## CHANGELOG.md

Keep a Changelog format at project root. Initial entry for this PR:

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

PR number filled in once the PR is created.

## Files Changed

| File | Action |
|------|--------|
| `cmd/notesview/main.go` | Add `version` var, `init()`, `debug` import |
| `Makefile` | Add `VERSION`, `LDFLAGS`, `install`, `update` targets |
| `.github/workflows/tag.yml` | New file |
| `CHANGELOG.md` | New file |

## Verification

- `make build && bin/notesview --version` prints version from git tag
- `go build ./cmd/notesview && ./notesview --version` prints `notesview version dev`
- `go run ./cmd/notesview --version` prints `notesview version dev`
