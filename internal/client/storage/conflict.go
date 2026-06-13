package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sastromikus/gophkeeper/internal/model"
)

// ConflictResolution selects which encrypted version should survive a conflict.
type ConflictResolution string

const (
	// ConflictResolutionLocal keeps the local ciphertext and queues it for upload
	// against the latest server version.
	ConflictResolutionLocal ConflictResolution = "local"
	// ConflictResolutionServer replaces the local ciphertext with the server copy.
	ConflictResolutionServer ConflictResolution = "server"
)

// RecordConflict contains the preserved local and server versions of one record.
type RecordConflict struct {
	Local  LocalRecord
	Remote LocalRecord
}

// SaveConflict atomically preserves the local pending version together with
// the current server version and marks the local record as conflicted.
func (database *LocalDatabase) SaveConflict(ctx context.Context, local, remote LocalRecord) error {
	if err := database.usable(ctx); err != nil {
		return err
	}
	if local.ID.IsZero() || remote.ID.IsZero() || local.ID != remote.ID {
		return fmt.Errorf("%w: conflict records must have the same non-zero ID", model.ErrInvalidInput)
	}
	if local.SyncStatus == SyncStatusSynced || local.SyncStatus == SyncStatusConflict {
		return fmt.Errorf("%w: local conflict source must be pending", model.ErrInvalidInput)
	}
	if remote.SyncStatus != SyncStatusSynced {
		return fmt.Errorf("%w: remote conflict version must be synchronized", model.ErrInvalidInput)
	}
	if err := local.Validate(); err != nil {
		return fmt.Errorf("validate local conflict version: %w", err)
	}
	if err := remote.Validate(); err != nil {
		return fmt.Errorf("validate remote conflict version: %w", err)
	}

	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin save conflict: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// A locally created record has no server metadata. Adopt the current
	// server metadata so the conflicted row remains valid and can later be
	// retried with optimistic locking after choosing the local version.
	local.Version = remote.Version
	local.Revision = remote.Revision
	local.CreatedAt = remote.CreatedAt
	if local.UpdatedAt.Before(remote.UpdatedAt) {
		local.UpdatedAt = remote.UpdatedAt
	}
	if local.DeletedAt != nil && local.DeletedAt.Before(local.UpdatedAt) {
		deletedAt := local.UpdatedAt
		local.DeletedAt = &deletedAt
	}
	local.SyncStatus = SyncStatusConflict
	if err := local.Validate(); err != nil {
		return fmt.Errorf("validate normalized local conflict version: %w", err)
	}
	if err := saveLocalRecordTx(ctx, tx, local); err != nil {
		return fmt.Errorf("save local conflict version: %w", err)
	}
	if err := saveConflictRecordTx(ctx, tx, remote); err != nil {
		return fmt.Errorf("save remote conflict version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit saved conflict: %w", err)
	}
	return nil
}

// ListConflicts returns all unresolved record conflicts ordered by record ID.
func (database *LocalDatabase) ListConflicts(ctx context.Context) ([]RecordConflict, error) {
	if err := database.usable(ctx); err != nil {
		return nil, err
	}
	rows, err := database.db.QueryContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM record_conflicts
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list remote conflict versions: %w", err)
	}
	var remotes []LocalRecord
	for rows.Next() {
		record, scanErr := scanLocalRecord(rows)
		if scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan remote conflict version: %w", scanErr)
		}
		remotes = append(remotes, record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate remote conflict versions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close remote conflict rows: %w", err)
	}

	conflicts := make([]RecordConflict, 0, len(remotes))
	for _, remote := range remotes {
		local, err := database.Get(ctx, remote.ID)
		if err != nil {
			return nil, fmt.Errorf("get local conflict version %s: %w", remote.ID, err)
		}
		if local.SyncStatus != SyncStatusConflict {
			return nil, fmt.Errorf("%w: record %s has a remote conflict copy but local status is %q", model.ErrInvalidInput, remote.ID, local.SyncStatus)
		}
		conflicts = append(conflicts, RecordConflict{Local: local, Remote: remote})
	}
	return conflicts, nil
}

// GetConflict returns both versions of one unresolved conflict.
func (database *LocalDatabase) GetConflict(ctx context.Context, id model.ID) (RecordConflict, error) {
	if err := database.usable(ctx); err != nil {
		return RecordConflict{}, err
	}
	if id.IsZero() {
		return RecordConflict{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	local, err := database.Get(ctx, id)
	if err != nil {
		return RecordConflict{}, err
	}
	if local.SyncStatus != SyncStatusConflict {
		return RecordConflict{}, model.ErrNotFound
	}
	remote, err := scanLocalRecord(database.db.QueryRowContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM record_conflicts
WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return RecordConflict{}, model.ErrNotFound
	}
	if err != nil {
		return RecordConflict{}, fmt.Errorf("get remote conflict version: %w", err)
	}
	return RecordConflict{Local: local, Remote: remote}, nil
}

// ResolveConflict atomically selects the local or server version and removes
// the stored remote conflict copy.
func (database *LocalDatabase) ResolveConflict(ctx context.Context, id model.ID, resolution ConflictResolution) error {
	if err := database.usable(ctx); err != nil {
		return err
	}
	if id.IsZero() {
		return fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	if resolution != ConflictResolutionLocal && resolution != ConflictResolutionServer {
		return fmt.Errorf("%w: unsupported conflict resolution %q", model.ErrInvalidInput, resolution)
	}

	tx, err := database.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin conflict resolution: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	local, err := scanLocalRecord(tx.QueryRowContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM records WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get local conflict version: %w", err)
	}
	if local.SyncStatus != SyncStatusConflict {
		return model.ErrNotFound
	}
	remote, err := scanLocalRecord(tx.QueryRowContext(ctx, `
SELECT id, type, encryption_version, encrypted_payload, encrypted_metadata,
       payload_nonce, metadata_nonce, version, revision, created_at, updated_at,
       deleted_at, sync_status
FROM record_conflicts WHERE id = ?`, id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("get remote conflict version: %w", err)
	}

	selected := remote
	if resolution == ConflictResolutionLocal {
		if remote.DeletedAt != nil {
			return fmt.Errorf("%w: a server tombstone cannot be overwritten; keep the server version and create a new record", model.ErrVersionConflict)
		}
		selected = local
		selected.Version = remote.Version
		selected.Revision = remote.Revision
		selected.CreatedAt = remote.CreatedAt
		if selected.UpdatedAt.Before(remote.UpdatedAt) {
			selected.UpdatedAt = remote.UpdatedAt
		}
		if selected.DeletedAt != nil {
			if selected.DeletedAt.Before(selected.UpdatedAt) {
				value := selected.UpdatedAt
				selected.DeletedAt = &value
			}
			selected.SyncStatus = SyncStatusDeleted
		} else {
			selected.SyncStatus = SyncStatusUpdated
		}
	}
	if err := saveLocalRecordTx(ctx, tx, selected); err != nil {
		return fmt.Errorf("save resolved conflict: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM record_conflicts WHERE id = ?", id.String()); err != nil {
		return fmt.Errorf("delete resolved remote conflict version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit conflict resolution: %w", err)
	}
	return nil
}
