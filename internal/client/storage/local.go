package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/model"
	_ "modernc.org/sqlite"
)

const localSchemaVersion = 1

// SyncStatus describes the relationship between a local record and the server.
type SyncStatus string

const (
	SyncStatusSynced   SyncStatus = "synced"
	SyncStatusCreated  SyncStatus = "created"
	SyncStatusUpdated  SyncStatus = "updated"
	SyncStatusDeleted  SyncStatus = "deleted"
	SyncStatusConflict SyncStatus = "conflict"
)

// LocalRecord contains an encrypted client-side record and its synchronization state.
type LocalRecord struct {
	ID         model.ID
	Data       clientcrypto.EncryptedRecordData
	Version    int64
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
	SyncStatus SyncStatus
}

// Validate checks whether a local record is internally consistent.
func (record LocalRecord) Validate() error {
	if record.ID.IsZero() {
		return fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	if err := record.Data.Type.Validate(); err != nil {
		return err
	}
	if record.Data.EncryptionVersion == 0 {
		return fmt.Errorf("%w: encryption version is required", model.ErrInvalidInput)
	}
	if !record.SyncStatus.valid() {
		return fmt.Errorf("%w: unsupported sync status %q", model.ErrInvalidInput, record.SyncStatus)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: invalid record timestamps", model.ErrInvalidInput)
	}
	if record.DeletedAt != nil && record.DeletedAt.Before(record.UpdatedAt) {
		return fmt.Errorf("%w: deletion time cannot precede update time", model.ErrInvalidInput)
	}

	if record.SyncStatus == SyncStatusCreated {
		if record.Version != 0 || record.Revision != 0 {
			return fmt.Errorf("%w: a locally created record cannot have server version metadata", model.ErrInvalidInput)
		}
	} else if record.Version < 1 || record.Revision < 1 {
		return fmt.Errorf("%w: server-backed records require positive version and revision", model.ErrInvalidInput)
	}

	if record.SyncStatus == SyncStatusDeleted || record.DeletedAt != nil {
		if record.DeletedAt == nil {
			return fmt.Errorf("%w: deleted status requires deletion time", model.ErrInvalidInput)
		}
		if len(record.Data.EncryptedPayload) != 0 || len(record.Data.EncryptedMetadata) != 0 || len(record.Data.PayloadNonce) != 0 || len(record.Data.MetadataNonce) != 0 {
			return fmt.Errorf("%w: local tombstone must not contain encrypted data", model.ErrInvalidInput)
		}
		return nil
	}

	if len(record.Data.EncryptedPayload) == 0 || len(record.Data.EncryptedMetadata) == 0 || len(record.Data.PayloadNonce) == 0 || len(record.Data.MetadataNonce) == 0 {
		return fmt.Errorf("%w: active local record requires encrypted data and nonces", model.ErrInvalidInput)
	}
	return nil
}

func (status SyncStatus) valid() bool {
	switch status {
	case SyncStatusSynced, SyncStatusCreated, SyncStatusUpdated, SyncStatusDeleted, SyncStatusConflict:
		return true
	default:
		return false
	}
}

// LocalDatabase stores encrypted records and synchronization state in SQLite.
type LocalDatabase struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error
}

// OpenLocalDatabase opens and migrates a client-local SQLite database.
func OpenLocalDatabase(ctx context.Context, path string) (*LocalDatabase, error) {
	if ctx == nil {
		return nil, errors.New("open local database: context is required")
	}
	if path == "" {
		return nil, errors.New("open local database: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create local database directory: %w", err)
	}

	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open local SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	local := &LocalDatabase{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping local SQLite database: %w", err)
	}
	if err := local.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return local, nil
}

// Close closes the local database. It is safe to call more than once.
func (database *LocalDatabase) Close() error {
	if database == nil || database.db == nil {
		return nil
	}
	database.closeOnce.Do(func() {
		database.closeErr = database.db.Close()
	})
	return database.closeErr
}

// Save inserts or replaces a complete encrypted local record.
func (database *LocalDatabase) Save(ctx context.Context, record LocalRecord) error {
	if err := database.usable(ctx); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("validate local record: %w", err)
	}

	var deletedAt any
	if record.DeletedAt != nil {
		deletedAt = record.DeletedAt.UTC().UnixNano()
	}
	_, err := database.db.ExecContext(ctx, `
INSERT INTO records (
    id, type, encryption_version, encrypted_payload, encrypted_metadata,
    payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
    deleted_at, sync_status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    type = excluded.type,
    encryption_version = excluded.encryption_version,
    encrypted_payload = excluded.encrypted_payload,
    encrypted_metadata = excluded.encrypted_metadata,
    payload_nonce = excluded.payload_nonce,
    metadata_nonce = excluded.metadata_nonce,
    version = excluded.version,
    revision = excluded.revision,
    created_at = excluded.created_at,
    updated_at = excluded.updated_at,
    deleted_at = excluded.deleted_at,
    sync_status = excluded.sync_status`,
		record.ID.String(), string(record.Data.Type), record.Data.EncryptionVersion,
		nullBytes(record.Data.EncryptedPayload), nullBytes(record.Data.EncryptedMetadata),
		nullBytes(record.Data.PayloadNonce), nullBytes(record.Data.MetadataNonce),
		record.Version, record.Revision, record.CreatedAt.UTC().UnixNano(),
		record.UpdatedAt.UTC().UnixNano(), deletedAt, string(record.SyncStatus),
	)
	if err != nil {
		return fmt.Errorf("save local record: %w", err)
	}
	return nil
}

// Get returns one encrypted local record.
func (database *LocalDatabase) Get(ctx context.Context, id model.ID) (LocalRecord, error) {
	if err := database.usable(ctx); err != nil {
		return LocalRecord{}, err
	}
	if id.IsZero() {
		return LocalRecord{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	row := database.db.QueryRowContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM records WHERE id = ?`, id.String())
	record, err := scanLocalRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return LocalRecord{}, model.ErrNotFound
	}
	if err != nil {
		return LocalRecord{}, fmt.Errorf("get local record: %w", err)
	}
	return record, nil
}

// List returns all local records ordered by ID. Tombstones are included only when requested.
func (database *LocalDatabase) List(ctx context.Context, includeDeleted bool) ([]LocalRecord, error) {
	if err := database.usable(ctx); err != nil {
		return nil, err
	}
	query := `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM records`
	if !includeDeleted {
		query += " WHERE deleted_at IS NULL"
	}
	query += " ORDER BY id"
	rows, err := database.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list local records: %w", err)
	}
	defer rows.Close()

	var records []LocalRecord
	for rows.Next() {
		record, err := scanLocalRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan local record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate local records: %w", err)
	}
	return records, nil
}

// ListPending returns local records that still require synchronization or conflict resolution.
func (database *LocalDatabase) ListPending(ctx context.Context) ([]LocalRecord, error) {
	if err := database.usable(ctx); err != nil {
		return nil, err
	}
	rows, err := database.db.QueryContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM records
WHERE sync_status <> ?
ORDER BY updated_at, id`, string(SyncStatusSynced))
	if err != nil {
		return nil, fmt.Errorf("list pending local records: %w", err)
	}
	defer rows.Close()

	var records []LocalRecord
	for rows.Next() {
		record, err := scanLocalRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending local record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending local records: %w", err)
	}
	return records, nil
}

// Delete permanently removes a local record. Server deletion is represented separately by a tombstone.
func (database *LocalDatabase) Delete(ctx context.Context, id model.ID) error {
	if err := database.usable(ctx); err != nil {
		return err
	}
	if id.IsZero() {
		return fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	result, err := database.db.ExecContext(ctx, "DELETE FROM records WHERE id = ?", id.String())
	if err != nil {
		return fmt.Errorf("delete local record: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect deleted local record: %w", err)
	}
	if count == 0 {
		return model.ErrNotFound
	}
	return nil
}

// LastRevision returns the last fully applied server synchronization revision.
func (database *LocalDatabase) LastRevision(ctx context.Context) (int64, error) {
	if err := database.usable(ctx); err != nil {
		return 0, err
	}
	var revision int64
	if err := database.db.QueryRowContext(ctx, "SELECT last_revision FROM sync_state WHERE id = 1").Scan(&revision); err != nil {
		return 0, fmt.Errorf("read local sync revision: %w", err)
	}
	return revision, nil
}

// SetLastRevision advances the fully applied server synchronization revision.
func (database *LocalDatabase) SetLastRevision(ctx context.Context, revision int64) error {
	if err := database.usable(ctx); err != nil {
		return err
	}
	if revision < 0 {
		return fmt.Errorf("%w: sync revision cannot be negative", model.ErrInvalidInput)
	}
	result, err := database.db.ExecContext(ctx, `
UPDATE sync_state
SET last_revision = ?
WHERE id = 1 AND last_revision <= ?`, revision, revision)
	if err != nil {
		return fmt.Errorf("update local sync revision: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect local sync revision update: %w", err)
	}
	if count == 0 {
		return errors.New("sync revision cannot move backwards")
	}
	return nil
}

func (database *LocalDatabase) migrate(ctx context.Context) error {
	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin local database migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`INSERT INTO schema_version(version) SELECT 0 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`,
		`CREATE TABLE IF NOT EXISTS records (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('credentials', 'text', 'binary', 'bank_card')),
    encryption_version INTEGER NOT NULL CHECK (encryption_version > 0),
    encrypted_payload BLOB,
    encrypted_metadata BLOB,
    payload_nonce BLOB,
    metadata_nonce BLOB,
    version INTEGER NOT NULL CHECK (version >= 0),
    revision INTEGER NOT NULL CHECK (revision >= 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER,
    sync_status TEXT NOT NULL CHECK (sync_status IN ('synced', 'created', 'updated', 'deleted', 'conflict')),
    CHECK (updated_at >= created_at),
    CHECK (deleted_at IS NULL OR deleted_at >= updated_at),
    CHECK (
        (deleted_at IS NULL AND encrypted_payload IS NOT NULL AND encrypted_metadata IS NOT NULL AND payload_nonce IS NOT NULL AND metadata_nonce IS NOT NULL)
        OR
        (deleted_at IS NOT NULL AND encrypted_payload IS NULL AND encrypted_metadata IS NULL AND payload_nonce IS NULL AND metadata_nonce IS NULL)
    )
)`,
		`CREATE INDEX IF NOT EXISTS records_sync_status_idx ON records(sync_status, updated_at)`,
		`CREATE INDEX IF NOT EXISTS records_revision_idx ON records(revision)`,
		`CREATE TABLE IF NOT EXISTS sync_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_revision INTEGER NOT NULL CHECK (last_revision >= 0)
)`,
		`INSERT INTO sync_state(id, last_revision) VALUES (1, 0) ON CONFLICT(id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply local schema version %d: %w", localSchemaVersion, err)
		}
	}

	var version int
	if err := tx.QueryRowContext(ctx, "SELECT version FROM schema_version LIMIT 1").Scan(&version); err != nil {
		return fmt.Errorf("read local schema version: %w", err)
	}
	if version > localSchemaVersion {
		return fmt.Errorf("local database schema version %d is newer than supported version %d", version, localSchemaVersion)
	}
	if version < localSchemaVersion {
		if _, err := tx.ExecContext(ctx, "UPDATE schema_version SET version = ?", localSchemaVersion); err != nil {
			return fmt.Errorf("record local schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit local database migration: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanLocalRecord(scanner rowScanner) (LocalRecord, error) {
	var (
		idText, typeText, statusText                                     string
		encryptionVersion                                                uint32
		encryptedPayload, encryptedMetadata, payloadNonce, metadataNonce []byte
		version, revision, createdAt, updatedAt                          int64
		deletedAt                                                        sql.NullInt64
	)
	if err := scanner.Scan(
		&idText, &typeText, &encryptionVersion, &encryptedPayload, &encryptedMetadata,
		&payloadNonce, &metadataNonce, &version, &revision, &createdAt, &updatedAt,
		&deletedAt, &statusText,
	); err != nil {
		return LocalRecord{}, err
	}
	id, err := model.ParseID(idText)
	if err != nil {
		return LocalRecord{}, fmt.Errorf("parse local record ID: %w", err)
	}
	recordType := model.RecordType(typeText)
	if err := recordType.Validate(); err != nil {
		return LocalRecord{}, err
	}
	record := LocalRecord{
		ID: id,
		Data: clientcrypto.EncryptedRecordData{
			Type: recordType, EncryptionVersion: encryptionVersion,
			EncryptedPayload:  append([]byte(nil), encryptedPayload...),
			EncryptedMetadata: append([]byte(nil), encryptedMetadata...),
			PayloadNonce:      append([]byte(nil), payloadNonce...),
			MetadataNonce:     append([]byte(nil), metadataNonce...),
		},
		Version: version, Revision: revision,
		CreatedAt: time.Unix(0, createdAt).UTC(), UpdatedAt: time.Unix(0, updatedAt).UTC(),
		SyncStatus: SyncStatus(statusText),
	}
	if deletedAt.Valid {
		value := time.Unix(0, deletedAt.Int64).UTC()
		record.DeletedAt = &value
	}
	if err := record.Validate(); err != nil {
		return LocalRecord{}, fmt.Errorf("validate stored local record: %w", err)
	}
	return record, nil
}

func nullBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func (database *LocalDatabase) usable(ctx context.Context) error {
	if database == nil || database.db == nil {
		return errors.New("local database is not initialized")
	}
	if ctx == nil {
		return errors.New("local database context is required")
	}
	return nil
}
