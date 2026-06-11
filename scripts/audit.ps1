param([switch]$Strict)

$ErrorActionPreference = "Stop"
$failed = $false

function Get-GitMatches {
    param([Parameter(Mandatory = $true)][string]$Pattern)

    $output = & git grep -n -E -- $Pattern -- '*.go' 2>$null
    if ($LASTEXITCODE -eq 0) {
        return @($output)
    }
    if ($LASTEXITCODE -eq 1) {
        return @()
    }
    throw "git grep failed for pattern: $Pattern"
}

function Report-Matches {
    param(
        [Parameter(Mandatory = $true)][string]$Label,
        [Parameter(Mandatory = $true)][object[]]$Matches,
        [switch]$FailInStrictMode
    )

    if ($Matches.Count -eq 0) {
        return
    }

    Write-Host $Label
    $Matches | ForEach-Object { Write-Host "  $_" }
    if ($Strict -and $FailInStrictMode) {
        $script:failed = $true
    }
}

# Test helpers may intentionally panic. Production code may not.
$panicMatches = @(Get-GitMatches 'panic\(' | Where-Object {
    $_ -notmatch '_test\.go:' -and
    $_ -notmatch '^api/gophkeeper/v1/.*\.pb\.go:'
})
Report-Matches -Label 'Unexpected panic calls:' -Matches $panicMatches -FailInStrictMode

# os.Exit is allowed only in the two top-level main functions.
$exitMatches = @(Get-GitMatches 'os\.Exit\(' | Where-Object {
    $_ -notmatch '^cmd/(client|server)/main\.go:'
})
Report-Matches -Label 'Unexpected os.Exit calls:' -Matches $exitMatches -FailInStrictMode

$todoMatches = @(Get-GitMatches 'context\.TODO\(' | Where-Object { $_ -notmatch '_test\.go:' })
Report-Matches -Label 'context.TODO calls in production code:' -Matches $todoMatches -FailInStrictMode

$fatalMatches = @(Get-GitMatches 'log\.(Print|Printf|Println|Fatal|Fatalf|Fatalln)\(' | Where-Object { $_ -notmatch '_test\.go:' })
Report-Matches -Label 'Legacy log package calls in production code:' -Matches $fatalMatches -FailInStrictMode

$sensitiveLogMatches = @(Get-GitMatches 'slog\.(Info|Warn|Error|Debug)\(.*(password|token|secret|cvv|card)' | Where-Object { $_ -notmatch '_test\.go:' })
Report-Matches -Label 'Potential sensitive structured logging:' -Matches $sensitiveLogMatches -FailInStrictMode

# Ignored cleanup errors are reviewed but do not fail strict mode automatically;
# deferred rollback and best-effort cleanup are legitimate patterns.
$ignoredCleanupMatches = @(Get-GitMatches '_\s*=\s*[^=].*\.(Close|Rollback|Commit|Sync)\(' | Where-Object {
    $_ -notmatch '_test\.go:'
})
Report-Matches -Label 'Ignored cleanup errors (manual review):' -Matches $ignoredCleanupMatches

$trackedSecrets = @(& git ls-files -- '.env' '*.key' '*.pem' '*.db' '*.sqlite' '*.sqlite3')
if ($LASTEXITCODE -ne 0) {
    throw 'git ls-files failed'
}
if ($trackedSecrets.Count -gt 0) {
    Write-Host 'Potentially sensitive tracked files:'
    $trackedSecrets | ForEach-Object { Write-Host "  $_" }
    $failed = $true
}

if ($failed) {
    throw 'Repository audit found items requiring review.'
}
Write-Host 'Repository audit completed.'
