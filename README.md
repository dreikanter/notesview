# notesview

A local web server for browsing and previewing markdown notes with live reload.

## Features

- Renders markdown files with syntax highlighting
- Live reload via SSE when files change
- Opens files in your preferred editor
- Auto-detects GUI vs terminal editors
- Supports [Ghostty](https://ghostty.org/) terminal

## Installation

```sh
go install github.com/dreikanter/notes-view/cmd/notesview@latest
```

## Usage

```sh
notesview <command> [flags]
```

### serve

Start the local preview server.

```sh
notesview serve [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--path` | `$NOTESVIEW_PATH` → `$NOTES_PATH` → `.` | Notes root directory or file to open |
| `--port`, `-p` | auto | Port to listen on |
| `--open`, `-o` | false | Open browser on start |
| `--editor` | `$NOTESVIEW_EDITOR` → `$VISUAL` → `$EDITOR` | Editor command |
| `--log-level` | `$NOTESVIEW_LOG_LEVEL` → `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `$NOTESVIEW_LOG_FORMAT` → `text` | Log output format: `text` or `json` |
| `--log-file` | `$NOTESVIEW_LOG_FILE` | Optional log file path (logs also go to stdout) |

If `--path` points to a file, the server root is set to the file's parent directory and the file is opened directly in the browser (when `--open` is set).

### Examples

```sh
notesview serve                            # serve current directory
notesview serve --path ~/notes            # serve a specific directory
notesview serve --path ~/notes/todo.md    # open a specific file, serve its directory
notesview serve -p 8080                   # use a fixed port
notesview serve --open                    # open browser on start
notesview serve --editor=code             # use VS Code to open files
```

## Desktop (prototype)

A Wails v2 wrapper ships under `cmd/notesview-desktop`. It reuses the same
HTTP server, but hands the handler to Wails' in-process asset server instead
of binding a TCP port, so the webview loads the app directly.

```sh
make desktop        # build ./notesview-desktop
./notesview-desktop --path ~/notes
```

Configuration mirrors `notesview serve`: the `--path` / `--editor` flags and
the `NOTESVIEW_PATH`, `NOTES_PATH`, `NOTESVIEW_EDITOR`, `VISUAL`, `EDITOR`,
and `NOTESVIEW_LOG_*` environment variables are honoured.

Build requirements (Wails v2 uses the OS webview via cgo):

- **Linux:** `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` (or `-4.0-dev`)
- **macOS:** Xcode Command Line Tools. The `make` target passes
  `CGO_LDFLAGS="-framework UniformTypeIdentifiers -mmacosx-version-min=10.13"`
  automatically; include the same flag if invoking `go build` by hand.
- **Windows:** the [WebView2 runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)

For proper app bundles (macOS `.app`, Windows installer, icons, signing)
install the [Wails CLI](https://wails.io/docs/gettingstarted/installation)
and run `wails build` from `cmd/notesview-desktop`.

## Development

```sh
make all            # build web assets (Vite) and Go binary
make assets         # rebuild web assets only
make assets-watch   # rebuild web assets on source change
make build          # build Go binary only (assumes assets already built)
make desktop        # build the Wails desktop wrapper
make desktop-dev    # desktop build with devtools enabled
make test           # run Go tests
make lint           # run golangci-lint
```

The committed `web/static/` artifacts are built from `web/src/` via Vite, so `go install`
and `go build` work without a Node toolchain. Contributors who touch files under `web/src/`
must rerun `make assets` and commit the regenerated `web/static/` files.
