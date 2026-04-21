# Changelog

## [Unreleased]

### Changed

- Refactor sidebar into a reusable client-side `TreeView` component. Tree state (expanded, selected, focus) lives in the browser; the server exposes `/api/tree/list` for children and a unified `/events` SSE stream that emits both file-change and directory-mutation events. ([#88])
- Trim long autolinks in rendered notes with a trailing ellipsis at the last path-segment boundary; the full URL stays in `href` and is exposed via a `title` tooltip, and a CSS rule wraps anything not trimmed. ([#93])

[#88]: https://github.com/dreikanter/notes-view/issues/88
[#93]: https://github.com/dreikanter/notes-view/issues/93

## [0.1.0] - 2026-04-12

### Added

- Build-time version injection via `-ldflags` ([#51])
- `--version` flag for the CLI ([#51])
- CHANGELOG.md to track changes between releases ([#51])
- GitHub Action to auto-tag on PR merge ([#51])

[0.1.0]: https://github.com/dreikanter/notes-view/releases/tag/v0.1.0
[#51]: https://github.com/dreikanter/notes-view/pull/51
