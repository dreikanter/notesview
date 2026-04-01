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
go install github.com/dreikanter/notesview/cmd/notesview@latest
```

## Usage

```sh
notesview [options] [path]
```

Path resolution order: CLI argument → `$NOTES_PATH` env var → current directory.

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-port`, `-p` | auto | Port to listen on |
| `-open`, `-o` | true | Open browser on start |
| `-editor` | `$EDITOR` | Editor command override |

### Examples

```sh
notesview ~/notes          # serve a specific directory
notesview -p 8080          # use a fixed port
notesview -open=false       # don't open browser automatically
notesview -editor=code      # use VS Code to open files
```

## Development

```sh
go test ./...
go build ./cmd/notesview
```
