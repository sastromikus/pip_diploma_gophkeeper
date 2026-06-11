#!/usr/bin/env sh
set -eu

for tool in protoc protoc-gen-go protoc-gen-go-grpc; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "$tool was not found in PATH" >&2
        exit 1
    fi
done

protoc \
  -I proto \
  --go_out=. \
  --go_opt=module=github.com/sastromikus/gophkeeper \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/sastromikus/gophkeeper \
  proto/gophkeeper/v1/auth.proto \
  proto/gophkeeper/v1/vault.proto

gofmt -w api/gophkeeper/v1
