BIN := notesview
BUILD_DIR := bin

.PHONY: build test lint clean assets assets-watch all

all: assets build

build:
	go build -o $(BUILD_DIR)/$(BIN) ./cmd/$(BIN)

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
