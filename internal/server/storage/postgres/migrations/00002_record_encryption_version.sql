-- +goose Up
ALTER TABLE records
    ADD COLUMN encryption_version INTEGER NOT NULL DEFAULT 1
    CHECK (encryption_version > 0);
ALTER TABLE records ALTER COLUMN encryption_version DROP DEFAULT;

-- +goose Down
ALTER TABLE records DROP COLUMN IF EXISTS encryption_version;
