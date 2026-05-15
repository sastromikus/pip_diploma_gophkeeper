# GophKeeper protobuf API

The API uses Protobuf Edition 2023 and explicitly requests the Go Opaque API.
Generated Go packages belong in `api/gophkeeper/v1` and must not be edited by
hand.

Authentication is passed through gRPC metadata:

```text
authorization: Bearer <token>
```

The server determines the owner from the authenticated session. Consequently,
vault requests and responses do not expose a user ID.

`SyncRecords` uses a monotonic server revision rather than client timestamps.
Records with `deleted_at` set are synchronization tombstones.
