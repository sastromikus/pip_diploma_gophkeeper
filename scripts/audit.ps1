param([switch]$Strict)
$ErrorActionPreference = "Stop"
$patterns = @(
  'panic\(',
  'os\.Exit\(',
  'context\.TODO\(',
  'log\.(Print|Printf|Println|Fatal|Fatalf|Fatalln)\(',
  'slog\.(Info|Warn|Error|Debug)\(.*(password|token|secret|cvv|card)',
  '_\s*=\s*[^=].*\.(Close|Rollback|Commit|Sync)\('
)
$failed = $false
foreach ($pattern in $patterns) {
  $matches = git grep -n -E $pattern -- '*.go' 2>$null
  if ($LASTEXITCODE -eq 0) {
    Write-Host "Pattern: $pattern"
    $matches | ForEach-Object { Write-Host "  $_" }
    if ($Strict) { $failed = $true }
  }
}
$trackedSecrets = git ls-files '.env' '*.key' '*.pem' '*.db' '*.sqlite' '*.sqlite3'
if ($trackedSecrets) {
  Write-Host 'Potentially sensitive tracked files:'
  $trackedSecrets | ForEach-Object { Write-Host "  $_" }
  $failed = $true
}
if ($failed) { throw 'Repository audit found items requiring review.' }
Write-Host 'Repository audit completed.'
