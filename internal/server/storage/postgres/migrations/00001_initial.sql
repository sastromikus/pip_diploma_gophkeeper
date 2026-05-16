-- +goose Up
-- +goose StatementBegin
CREATE SEQUENCE records_revision_seq AS BIGINT START WITH 1;

CREATE TABLE users (
    id UUID PRIMARY KEY,
    login VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    encrypted_data_key BYTEA NOT NULL,
    key_salt BYTEA NOT NULL,
    key_nonce BYTEA NOT NULL,
    key_derivation_version INTEGER NOT NULL CHECK (key_derivation_version > 0),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT sessions_expiration_after_creation CHECK (expires_at > created_at),
    CONSTRAINT sessions_revocation_after_creation CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE records (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL CHECK (type IN ('credentials', 'text', 'binary', 'bank_card')),
    encrypted_payload BYTEA,
    encrypted_metadata BYTEA,
    payload_nonce BYTEA,
    metadata_nonce BYTEA,
    version BIGINT NOT NULL CHECK (version > 0),
    revision BIGINT NOT NULL UNIQUE DEFAULT nextval('records_revision_seq'),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT records_update_after_creation CHECK (updated_at >= created_at),
    CONSTRAINT records_deletion_after_update CHECK (deleted_at IS NULL OR deleted_at >= updated_at),
    CONSTRAINT records_active_or_tombstone CHECK (
        (
            deleted_at IS NULL
            AND encrypted_payload IS NOT NULL
            AND encrypted_metadata IS NOT NULL
            AND payload_nonce IS NOT NULL
            AND metadata_nonce IS NOT NULL
        )
        OR
        (
            deleted_at IS NOT NULL
            AND encrypted_payload IS NULL
            AND encrypted_metadata IS NULL
            AND payload_nonce IS NULL
            AND metadata_nonce IS NULL
        )
    )
);

CREATE INDEX records_user_id_id_idx ON records(user_id, id);
CREATE INDEX records_user_revision_idx ON records(user_id, revision);
CREATE INDEX records_user_active_idx ON records(user_id, id) WHERE deleted_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS records;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP SEQUENCE IF EXISTS records_revision_seq;
-- +goose StatementEnd
