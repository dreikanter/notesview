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
