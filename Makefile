.PHONY: build install test clean import fmt lint

BINARY   := claude-memory
IMPORT   := claude-memory-import
GOFLAGS  := -trimpath
CGO      := 1

build:
	CGO_ENABLED=$(CGO) go build $(GOFLAGS) -o bin/$(BINARY) .
	CGO_ENABLED=$(CGO) go build $(GOFLAGS) -o bin/$(IMPORT) ./cmd/import

install: build
	cp bin/$(BINARY) /usr/local/bin/$(BINARY)
	cp bin/$(IMPORT) /usr/local/bin/$(IMPORT)

test:
	CGO_ENABLED=$(CGO) go test ./...

clean:
	rm -rf bin/

import: build
	@if [ -z "$(DIR)" ]; then echo "Usage: make import DIR=<path>"; exit 1; fi
	bin/$(IMPORT) --dir $(DIR)

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

help:
	@echo "Targets:"
	@echo "  build    - Build binaries to bin/"
	@echo "  install  - Build and install to /usr/local/bin"
	@echo "  test     - Run all tests"
	@echo "  clean    - Remove build artifacts"
	@echo "  import   - Import markdown files (DIR=<path>)"
	@echo "  fmt      - Format Go source"
	@echo "  lint     - Run linter"
