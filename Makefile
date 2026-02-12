.PHONY: build build-server build-all test run lint clean install install-plugin

BINARY=mark42
SERVER=mark42-server
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

## Build

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/memory

build-server:
	go build $(LDFLAGS) -o $(SERVER) ./cmd/server

build-all: build build-server

## Test

test:
	go test -v -race ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## Run

run: build
	./$(BINARY) --db ./test.db

## Development

lint:
	go tool golangci-lint run

fmt:
	go tool golangci-lint fmt

tidy:
	go mod tidy

## Clean

clean:
	rm -f $(BINARY) $(SERVER) coverage.out coverage.html test.db
	rm -rf bin/

## Install

install: build
	cp $(BINARY) ~/bin/

install-server: build-server
	cp $(SERVER) ~/bin/

install-all: build-all
	mkdir -p ~/bin
	cp $(BINARY) $(SERVER) ~/bin/
	cp $(BINARY) $(SERVER) /opt/homebrew/bin/ 2>/dev/null || true

## Plugin Installation

install-plugin: build-all
	@echo "Installing mark42 plugin..."
	mkdir -p bin/
	cp $(BINARY) $(SERVER) bin/
	@echo "Plugin binaries ready in bin/"
	@echo "To complete installation, copy to ~/.claude/plugins/local/mark42/"

## Migration (from JSON Memory MCP)

migrate:
	./$(BINARY) migrate --from ~/.claude/memory.json --to ~/.claude/memory.db
