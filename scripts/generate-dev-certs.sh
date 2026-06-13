#!/usr/bin/env sh
set -eu

output_dir="${1:-certificates/dev}"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl was not found in PATH" >&2
  exit 1
fi

mkdir -p "$output_dir"
umask 077

ca_key="$output_dir/ca.key"
ca_cert="$output_dir/ca.pem"
server_key="$output_dir/server.key"
server_csr="$output_dir/server.csr"
server_cert="$output_dir/server.pem"
extensions="$output_dir/server-ext.cnf"

cat >"$extensions" <<'EOT'
subjectAltName=DNS:localhost,IP:127.0.0.1
extendedKeyUsage=serverAuth
keyUsage=digitalSignature,keyEncipherment
EOT

openssl genrsa -out "$ca_key" 3072
openssl req -x509 -new -sha256 -days 3650 -key "$ca_key" -out "$ca_cert" -subj "/CN=GophKeeper Development CA"
openssl genrsa -out "$server_key" 3072
openssl req -new -sha256 -key "$server_key" -out "$server_csr" -subj "/CN=localhost"
openssl x509 -req -sha256 -days 825 -in "$server_csr" -CA "$ca_cert" -CAkey "$ca_key" -CAcreateserial -out "$server_cert" -extfile "$extensions"

rm -f "$server_csr" "$extensions"

echo "Development certificates created in $output_dir"
echo "Server certificate: $server_cert"
echo "Server key:         $server_key"
echo "Client CA:          $ca_cert"
