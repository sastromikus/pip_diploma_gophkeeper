param(
    [Parameter(Mandatory = $false)]
    [string]$DatabaseDSN = $env:GOPHKEEPER_TEST_DATABASE_DSN
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($DatabaseDSN)) {
    throw "DatabaseDSN is required. Pass -DatabaseDSN or set GOPHKEEPER_TEST_DATABASE_DSN."
}

$env:GOPHKEEPER_TEST_DATABASE_DSN = $DatabaseDSN

go test -count=1 -run '^TestEndToEndTLSAuthentication$' -v ./internal/server/app
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
