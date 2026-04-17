.PHONY: build test lint clean assets assets-watch all install update desktop desktop-dev

BINARY := notesview
DESKTOP_BINARY := notesview-desktop
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.Version=$(VERSION)

# On macOS, Wails' drag-and-drop code uses UTType, which lives in the
# UniformTypeIdentifiers framework. `wails build` injects this automatically;
# plain `go build` does not, so we add it here.
ifeq ($(shell uname),Darwin)
DESKTOP_CGO_LDFLAGS := -framework UniformTypeIdentifiers -mmacosx-version-min=10.13
endif

all: assets build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/$(BINARY)

# desktop builds the Wails-wrapped native app. Requires a CGO toolchain
# and platform webview deps: WebKit2GTK (Linux), Xcode CLT (macOS),
# WebView2 runtime (Windows). See README.md for details.
desktop: assets
	CGO_LDFLAGS="$(DESKTOP_CGO_LDFLAGS)" \
		go build -tags production -ldflags "$(LDFLAGS) -w -s" -o $(DESKTOP_BINARY) ./cmd/$(DESKTOP_BINARY)

# desktop-dev builds an unoptimised desktop binary with Wails' dev tags
# so the webview devtools are reachable.
desktop-dev: assets
	CGO_LDFLAGS="$(DESKTOP_CGO_LDFLAGS)" \
		go build -tags dev -ldflags "$(LDFLAGS)" -o $(DESKTOP_BINARY) ./cmd/$(DESKTOP_BINARY)

test:
	go test ./...

lint:
	go tool golangci-lint run

assets:
	npx vite build

assets-watch:
	npx vite build --watch

clean:
	rm -f $(BINARY) $(DESKTOP_BINARY)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)

update:
	git checkout main
	git pull --tags
	$(MAKE) install
	@echo "Installed: $$(notesview --version)"
