@echo off
setlocal

call scripts\generate-proto.cmd
if errorlevel 1 exit /b 1

gofmt -w .
if errorlevel 1 exit /b 1

go mod tidy
if errorlevel 1 exit /b 1

go test -count=1 -p=1 ./...
if errorlevel 1 exit /b 1

go vet ./...
if errorlevel 1 exit /b 1

endlocal
