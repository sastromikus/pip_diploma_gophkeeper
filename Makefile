.PHONY: build build-server build-client proto test test-integration test-e2e coverage coverage-check race fmt vet clean

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
	go test -p=1 ./...

test-integration:
	go test ./internal/server/storage/postgres -run Integration -v

test-e2e:
	./scripts/e2e.sh "$(GOPHKEEPER_TEST_DATABASE_DSN)"

coverage:
	./scripts/coverage.sh

coverage-check:
	./scripts/coverage.sh coverage.out "$(GOPHKEEPER_TEST_DATABASE_DSN)" 70

race:
	go test -p=1 -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	go clean
	rm -rf $(BIN_DIR)

.PHONY: test-e2e-tls
test-e2e-tls:
	@test -n "$(GOPHKEEPER_TEST_DATABASE_DSN)" || (echo "GOPHKEEPER_TEST_DATABASE_DSN is required" >&2; exit 2)
	GOPHKEEPER_TEST_DATABASE_DSN="$(GOPHKEEPER_TEST_DATABASE_DSN)" go test -count=1 -run '^TestEndToEndTLSAuthentication$$' -v ./internal/server/app
