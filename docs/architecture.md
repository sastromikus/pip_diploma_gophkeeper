# Architecture

GophKeeper consists of a CLI client and a gRPC server.

## Client

The client owns plaintext handling and cryptography. It derives a key-encryption key with Argon2id, unwraps a random data-encryption key, and encrypts each record's payload and metadata with AES-256-GCM. Record ID, type, format version, and encrypted part are authenticated through AAD.

The client uses SQLite as an offline-first encrypted store. Local records have synchronization states such as `created`, `updated`, `deleted`, `synced`, and `conflict`. Plaintext vault data and the unwrapped data-encryption key are not persisted.

## Server

The server exposes authentication and vault services over gRPC. PostgreSQL stores users, opaque session-token hashes, encrypted records, tombstones, versions, and monotonic synchronization revisions. Service logic is separated from gRPC and repository implementations.

## Synchronization

Clients upload pending ciphertext and then download changes after the last committed revision. The revision cursor advances only after a complete page is applied locally. Optimistic record versions detect concurrent changes. Both sides of a conflict are retained until the user explicitly chooses the local or server version.

## Trust boundaries

- Plaintext vault data is processed only by the client.
- The server authenticates users and authorizes record ownership.
- TLS protects client-server transport.
- PostgreSQL and SQLite are treated as persistent but potentially exposed storage, so vault data remains encrypted there.
