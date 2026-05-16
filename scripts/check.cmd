@echo off
setlocal

gofmt -w .
if errorlevel 1 exit /b 1

go mod tidy
if errorlevel 1 exit /b 1

go test ./...
if errorlevel 1 exit /b 1

go vet ./...
if errorlevel 1 exit /b 1

endlocal
