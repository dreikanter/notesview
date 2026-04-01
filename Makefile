BIN := notesview
BUILD_DIR := bin
CSS_SRC := web/src/input.css
CSS_OUT := web/static/style.css

.PHONY: build test lint clean css css-watch all

all: css build

build:
	go build -o $(BUILD_DIR)/$(BIN) ./cmd/$(BIN)

test:
	go test ./...

lint:
	golangci-lint run ./...

css:
	npx tailwindcss -i $(CSS_SRC) -o $(CSS_OUT) --minify

css-watch:
	npx tailwindcss -i $(CSS_SRC) -o $(CSS_OUT) --watch

clean:
	rm -rf $(BUILD_DIR)
