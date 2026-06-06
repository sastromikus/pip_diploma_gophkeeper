[CmdletBinding()]
param(
    [string]$Output = "coverage.out",
    [switch]$Html,
    [switch]$Enforce,
    [double]$Minimum = 70.0,
    [string]$DatabaseDSN = ""
)

$ErrorActionPreference = "Stop"

if ($DatabaseDSN) {
    $env:GOPHKEEPER_TEST_DATABASE_DSN = $DatabaseDSN
}
if (-not $env:GOPHKEEPER_TEST_DATABASE_DSN) {
    Write-Warning "GOPHKEEPER_TEST_DATABASE_DSN is not set; PostgreSQL integration tests will be skipped and coverage will be lower."
}

$module = "github.com/sastromikus/gophkeeper"
$generatedPackage = "$module/api/gophkeeper/v1"
$packages = @(go list ./... | Where-Object { $_ -ne $generatedPackage })

if ($LASTEXITCODE -ne 0) {
    throw "go list failed"
}
if ($packages.Count -eq 0) {
    throw "no packages found"
}

& go test -count=1 -covermode=atomic -coverprofile $Output @packages
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$summary = & go tool cover -func $Output
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

$summary | Write-Output
$totalLine = $summary | Select-Object -Last 1
if ($totalLine -notmatch '([0-9]+(?:\.[0-9]+)?)%$') {
    throw "cannot parse total coverage from: $totalLine"
}

$total = [double]::Parse(
    $Matches[1],
    [System.Globalization.CultureInfo]::InvariantCulture
)

if ($Html) {
    $htmlPath = [System.IO.Path]::ChangeExtension($Output, ".html")
    & go tool cover -html $Output -o $htmlPath
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    Write-Host "HTML report: $htmlPath"
}

if ($Enforce -and $total -lt $Minimum) {
    Write-Error ("coverage {0:N1}% is below required {1:N1}%" -f $total, $Minimum)
    exit 1
}

Write-Host ("Total handwritten-code coverage: {0:N1}%" -f $total)
