@echo off
setlocal

where protoc >nul 2>nul
if errorlevel 1 (
    echo protoc was not found in PATH. 1>&2
    exit /b 1
)

where protoc-gen-go >nul 2>nul
if errorlevel 1 (
    echo protoc-gen-go was not found in PATH. 1>&2
    exit /b 1
)

where protoc-gen-go-grpc >nul 2>nul
if errorlevel 1 (
    echo protoc-gen-go-grpc was not found in PATH. 1>&2
    exit /b 1
)

protoc ^
  -I proto ^
  --go_out=. ^
  --go_opt=module=github.com/sastromikus/gophkeeper ^
  --go-grpc_out=. ^
  --go-grpc_opt=module=github.com/sastromikus/gophkeeper ^
  proto/gophkeeper/v1/auth.proto ^
  proto/gophkeeper/v1/vault.proto

if errorlevel 1 exit /b 1

gofmt -w api\gophkeeper\v1
endlocal
