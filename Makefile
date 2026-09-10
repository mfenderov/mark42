.PHONY: build build-server build-all test test-coverage crap run lint clean install install-server install-all

BINARY=mark42
SERVER=mark42-server
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
CLI_LDFLAGS=-ldflags "-X github.com/mfenderov/mark42/internal/cli.Version=$(VERSION)"
SERVER_LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

## Build

build:
	go build $(CLI_LDFLAGS) -o $(BINARY) ./cmd/memory

build-server:
	go build $(SERVER_LDFLAGS) -o $(SERVER) ./cmd/server

build-all: build build-server

## Test

test:
	go test -v -race ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# CRAP quality gate: complexity² × (1-coverage)³ + complexity. Max 10 — keep
# functions small and tested. Migrations/cmd excluded (tested indirectly /
# entry-point glue).
crap:
	go test -coverprofile=coverage.out ./...
	go tool gocrap -coverprofile coverage.out \
		-exclude '*_test.go' -exclude 'internal/storage/migrations/*' \
		-exclude 'cmd/*' -exclude 'cmd/*/*' \
		-max 10 ./...

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
	mkdir -p ~/bin
	cp $(BINARY) ~/bin/

install-server: build-server
	mkdir -p ~/bin
	cp $(SERVER) ~/bin/

install-all: build-all
	mkdir -p ~/bin
	cp $(BINARY) $(SERVER) ~/bin/

## Migration (from JSON Memory MCP)

migrate: build
	./$(BINARY) migrate --from ~/.config/mark42/memory.json
