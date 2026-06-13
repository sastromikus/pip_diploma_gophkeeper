package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const recordColumns = `id::text, user_id::text, type, encryption_version, encrypted_payload, encrypted_metadata, payload_nonce, metadata_nonce, version, revision, created_at, updated_at, deleted_at`

// RecordRepository persists encrypted vault records and synchronization tombstones.
type RecordRepository struct {
	pool *pgxpool.Pool
}

// NewRecordRepository creates a PostgreSQL record repository.
func NewRecordRepository(pool *pgxpool.Pool) *RecordRepository {
	return &RecordRepository{pool: pool}
}

// Create inserts an active record with server-managed version, revision, and timestamps.
func (repository *RecordRepository) Create(ctx context.Context, record model.Record) (model.Record, error) {
	created, err := scanRecord(repository.pool.QueryRow(ctx, `
        INSERT INTO records (
            id, user_id, type, encryption_version, encrypted_payload, encrypted_metadata,
            payload_nonce, metadata_nonce, version, revision, created_at, updated_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8,
            1, nextval('gophkeeper_records_revision_seq'), NOW(), NOW()
        )
        RETURNING `+recordColumns,
		record.ID.String(), record.UserID.String(), string(record.Type), record.EncryptionVersion,
		record.EncryptedPayload, record.EncryptedMetadata,
		record.PayloadNonce, record.MetadataNonce,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return model.Record{}, model.ErrAlreadyExists
		}
		return model.Record{}, fmt.Errorf("insert record: %w", err)
	}
	return created, nil
}

// Get returns one active record owned by the supplied user.
func (repository *RecordRepository) Get(ctx context.Context, userID, recordID model.ID) (model.Record, error) {
	record, err := scanRecord(repository.pool.QueryRow(ctx, `
        SELECT `+recordColumns+`
        FROM records
        WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
    `, recordID.String(), userID.String()))
	if err != nil {
		if mapped := mapNotFound(err); errors.Is(mapped, model.ErrNotFound) {
			return model.Record{}, model.ErrNotFound
		}
		return model.Record{}, fmt.Errorf("select record: %w", err)
	}
	return record, nil
}

// List returns active records ordered by UUID. afterID is an exclusive cursor.
func (repository *RecordRepository) List(ctx context.Context, userID model.ID, afterID model.ID, limit int32) ([]model.Record, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: list limit must be positive", model.ErrInvalidInput)
	}

	rows, err := repository.pool.Query(ctx, `
        SELECT `+recordColumns+`
        FROM records
        WHERE user_id = $1
          AND deleted_at IS NULL
          AND (NULLIF($2, '') IS NULL OR id > NULLIF($2, '')::uuid)
        ORDER BY id
        LIMIT $3
    `, userID.String(), afterID.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("query active records: %w", err)
	}
	defer rows.Close()

	records := make([]model.Record, 0, limit)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan active record: %w", scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active records: %w", err)
	}
	return records, nil
}

// ListChangedAfter returns changes ordered by their monotonic server revision.
func (repository *RecordRepository) ListChangedAfter(ctx context.Context, userID model.ID, afterRevision int64, limit int32) ([]model.Record, error) {
	if afterRevision < 0 {
		return nil, fmt.Errorf("%w: revision cursor must not be negative", model.ErrInvalidInput)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: synchronization limit must be positive", model.ErrInvalidInput)
	}

	rows, err := repository.pool.Query(ctx, `
        SELECT `+recordColumns+`
        FROM records
        WHERE user_id = $1 AND revision > $2
        ORDER BY revision
        LIMIT $3
    `, userID.String(), afterRevision, limit)
	if err != nil {
		return nil, fmt.Errorf("query record changes: %w", err)
	}
	defer rows.Close()

	records := make([]model.Record, 0, limit)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan record change: %w", scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate record changes: %w", err)
	}
	return records, nil
}

// Update replaces encrypted fields when expectedVersion matches the active row.
func (repository *RecordRepository) Update(ctx context.Context, record model.Record, expectedVersion int64) (model.Record, error) {
	if expectedVersion < 1 {
		return model.Record{}, fmt.Errorf("%w: expected version must be positive", model.ErrInvalidInput)
	}

	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Record{}, fmt.Errorf("begin record update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentVersion int64
	var deleted bool
	var currentType model.RecordType
	err = tx.QueryRow(ctx, `
        SELECT version, deleted_at IS NOT NULL, type
        FROM records
        WHERE id = $1 AND user_id = $2
        FOR UPDATE
    `, record.ID.String(), record.UserID.String()).Scan(&currentVersion, &deleted, &currentType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Record{}, model.ErrNotFound
		}
		return model.Record{}, fmt.Errorf("lock record for update: %w", err)
	}
	if deleted {
		return model.Record{}, model.ErrNotFound
	}
	if currentVersion != expectedVersion {
		return model.Record{}, model.ErrVersionConflict
	}
	if currentType != record.Type {
		return model.Record{}, fmt.Errorf("%w: changing record type is not supported", model.ErrInvalidInput)
	}

	updated, err := scanRecord(tx.QueryRow(ctx, `
        UPDATE records
        SET type = $3,
            encryption_version = $4,
            encrypted_payload = $5,
            encrypted_metadata = $6,
            payload_nonce = $7,
            metadata_nonce = $8,
            version = version + 1,
            revision = nextval('gophkeeper_records_revision_seq'),
            updated_at = NOW()
        WHERE id = $1 AND user_id = $2
        RETURNING `+recordColumns,
		record.ID.String(), record.UserID.String(), string(record.Type), record.EncryptionVersion,
		record.EncryptedPayload, record.EncryptedMetadata,
		record.PayloadNonce, record.MetadataNonce,
	))
	if err != nil {
		return model.Record{}, fmt.Errorf("update record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Record{}, fmt.Errorf("commit record update: %w", err)
	}
	return updated, nil
}

// Delete replaces an active record with a minimal tombstone.
func (repository *RecordRepository) Delete(ctx context.Context, userID, recordID model.ID, expectedVersion int64) (model.Record, error) {
	if expectedVersion < 1 {
		return model.Record{}, fmt.Errorf("%w: expected version must be positive", model.ErrInvalidInput)
	}

	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Record{}, fmt.Errorf("begin record deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentVersion int64
	var deleted bool
	err = tx.QueryRow(ctx, `
        SELECT version, deleted_at IS NOT NULL
        FROM records
        WHERE id = $1 AND user_id = $2
        FOR UPDATE
    `, recordID.String(), userID.String()).Scan(&currentVersion, &deleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Record{}, model.ErrNotFound
		}
		return model.Record{}, fmt.Errorf("lock record for deletion: %w", err)
	}
	if deleted {
		return model.Record{}, model.ErrNotFound
	}
	if currentVersion != expectedVersion {
		return model.Record{}, model.ErrVersionConflict
	}

	tombstone, err := scanRecord(tx.QueryRow(ctx, `
        UPDATE records
        SET encrypted_payload = NULL,
            encrypted_metadata = NULL,
            payload_nonce = NULL,
            metadata_nonce = NULL,
            version = version + 1,
            revision = nextval('gophkeeper_records_revision_seq'),
            updated_at = NOW(),
            deleted_at = NOW()
        WHERE id = $1 AND user_id = $2
        RETURNING `+recordColumns,
		recordID.String(), userID.String(),
	))
	if err != nil {
		return model.Record{}, fmt.Errorf("delete record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Record{}, fmt.Errorf("commit record deletion: %w", err)
	}
	return tombstone, nil
}
