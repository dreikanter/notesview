# Changelog

## [Unreleased]

### Changed

- Refactor sidebar into a reusable client-side `TreeView` component. Tree state (expanded, selected, focus) lives in the browser; the server exposes `/api/tree/list` for children and a unified `/events` SSE stream that emits both file-change and directory-mutation events. ([#88])

[#88]: https://github.com/dreikanter/notes-view/issues/88

## [0.1.0] - 2026-04-12

### Added

- Build-time version injection via `-ldflags` ([#51])
- `--version` flag for the CLI ([#51])
- CHANGELOG.md to track changes between releases ([#51])
- GitHub Action to auto-tag on PR merge ([#51])

[0.1.0]: https://github.com/dreikanter/notes-view/releases/tag/v0.1.0
[#51]: https://github.com/dreikanter/notes-view/pull/51
