# Security policy

## Supported versions

Only the current `master`/`main` branch is supported while the project is under active development.

## Reporting a vulnerability

Do not publish credentials, tokens, private keys, decrypted vault data, or database dumps in a public issue. Report the problem privately to the repository owner and include only the minimum reproduction data required.

## Security model

GophKeeper encrypts vault payloads and metadata on the client with AES-256-GCM. The server stores ciphertext, nonces, synchronization metadata, password hashes, and hashes of opaque session tokens. TLS protects data in transit.

The account password is currently also used to derive the key-encryption key. Therefore the design protects against disclosure of the server database, but it is not a zero-knowledge design against a malicious server that captures login passwords.

## Operational requirements

- Run the server with TLS outside local development.
- Keep database credentials, certificates, and private keys outside Git.
- Use a dedicated PostgreSQL role and database.
- Rotate certificates and revoke exposed sessions after an incident.
- Back up PostgreSQL and client SQLite files only to protected storage.
