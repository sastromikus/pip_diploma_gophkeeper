.PHONY: build build-server build-client proto test test-integration coverage coverage-check race fmt vet clean

BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/gophkeeper-server
CLIENT_BIN := $(BIN_DIR)/gophkeeper-client

VERSION ?= dev
BUILD_DATE ?= unknown
COMMIT ?= unknown
MODULE := github.com/sastromikus/gophkeeper
LDFLAGS := -X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)'

ifeq ($(OS),Windows_NT)
SERVER_BIN := $(SERVER_BIN).exe
CLIENT_BIN := $(CLIENT_BIN).exe
endif

build: build-server build-client

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-server: | $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(SERVER_BIN) ./cmd/server

build-client: | $(BIN_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(CLIENT_BIN) ./cmd/client

proto:
	./scripts/generate-proto.sh

test:
	go test ./...

test-integration:
	go test ./internal/server/storage/postgres -run Integration -v

coverage:
	./scripts/coverage.sh

coverage-check:
	./scripts/coverage.sh coverage.out "$(GOPHKEEPER_TEST_DATABASE_DSN)" 70

race:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	go clean
	rm -rf $(BIN_DIR)
