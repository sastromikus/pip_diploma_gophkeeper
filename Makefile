.PHONY: build build-server build-client test fmt vet clean

BIN_DIR := bin
SERVER_BIN := $(BIN_DIR)/gophkeeper-server
CLIENT_BIN := $(BIN_DIR)/gophkeeper-client

ifeq ($(OS),Windows_NT)
SERVER_BIN := $(SERVER_BIN).exe
CLIENT_BIN := $(CLIENT_BIN).exe
endif

build: build-server build-client

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-server: | $(BIN_DIR)
	go build -o $(SERVER_BIN) ./cmd/server

build-client: | $(BIN_DIR)
	go build -o $(CLIENT_BIN) ./cmd/client

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	go clean
	rm -rf $(BIN_DIR)
