param(
    [string]$OutputDirectory = "certificates/dev"
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command openssl -ErrorAction SilentlyContinue)) {
    throw "openssl was not found in PATH"
}

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null

$caKey = Join-Path $OutputDirectory "ca.key"
$caCert = Join-Path $OutputDirectory "ca.pem"
$serverKey = Join-Path $OutputDirectory "server.key"
$serverCSR = Join-Path $OutputDirectory "server.csr"
$serverCert = Join-Path $OutputDirectory "server.pem"
$extensions = Join-Path $OutputDirectory "server-ext.cnf"

@"
subjectAltName=DNS:localhost,IP:127.0.0.1
extendedKeyUsage=serverAuth
keyUsage=digitalSignature,keyEncipherment
"@ | Set-Content -Encoding ascii $extensions

& openssl genrsa -out $caKey 3072
& openssl req -x509 -new -sha256 -days 3650 -key $caKey -out $caCert -subj "/CN=GophKeeper Development CA"
& openssl genrsa -out $serverKey 3072
& openssl req -new -sha256 -key $serverKey -out $serverCSR -subj "/CN=localhost"
& openssl x509 -req -sha256 -days 825 -in $serverCSR -CA $caCert -CAkey $caKey -CAcreateserial -out $serverCert -extfile $extensions

Remove-Item $serverCSR, $extensions -Force

Write-Host "Development certificates created in $OutputDirectory"
Write-Host "Server certificate: $serverCert"
Write-Host "Server key:         $serverKey"
Write-Host "Client CA:          $caCert"
