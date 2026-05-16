@echo off
setlocal

if not exist bin mkdir bin

if "%VERSION%"=="" set VERSION=dev
if "%BUILD_DATE%"=="" set BUILD_DATE=unknown
if "%COMMIT%"=="" set COMMIT=unknown

set MODULE=github.com/sastromikus/gophkeeper
set LDFLAGS=-X %MODULE%/internal/version.Version=%VERSION% -X %MODULE%/internal/version.BuildDate=%BUILD_DATE% -X %MODULE%/internal/version.Commit=%COMMIT%

go build -ldflags="%LDFLAGS%" -o bin\gophkeeper-server.exe .\cmd\server
if errorlevel 1 exit /b 1

go build -ldflags="%LDFLAGS%" -o bin\gophkeeper-client.exe .\cmd\client
if errorlevel 1 exit /b 1

endlocal
