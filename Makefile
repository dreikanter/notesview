.PHONY: build test test-unit lint clean assets assets-watch all install update

BINARY := nview
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.Version=$(VERSION)

all: assets build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/$(BINARY)

test-unit:
	npm run test:unit

test: test-unit
	go test ./...

lint:
	go tool golangci-lint run

assets:
	npx vite build

assets-watch:
	npx vite build --watch

clean:
	rm -f $(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)

update:
	git checkout main
	git pull --tags
	$(MAKE) install
	@echo "Installed: $$(nview --version)"
