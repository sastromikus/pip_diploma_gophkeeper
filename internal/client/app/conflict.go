package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/sastromikus/gophkeeper/internal/client/storage"
	"github.com/sastromikus/gophkeeper/internal/model"
)

// ConflictStore describes local conflict persistence used by ConflictService.
type ConflictStore interface {
	ListConflicts(context.Context) ([]storage.RecordConflict, error)
	ResolveConflict(context.Context, model.ID, storage.ConflictResolution) error
}

// ConflictSummary contains non-secret conflict metadata suitable for CLI output.
type ConflictSummary struct {
	ID            model.ID
	Type          model.RecordType
	LocalVersion  int64
	RemoteVersion int64
	RemoteDeleted bool
}

// ConflictService lists and resolves encrypted synchronization conflicts.
type ConflictService struct {
	store ConflictStore
}

// NewConflictService creates a conflict application service.
func NewConflictService(store ConflictStore) (*ConflictService, error) {
	if store == nil {
		return nil, errors.New("conflict store is required")
	}
	return &ConflictService{store: store}, nil
}

// List returns unresolved conflicts without decrypting their contents.
func (service *ConflictService) List(ctx context.Context) ([]ConflictSummary, error) {
	if ctx == nil {
		return nil, errors.New("list conflicts: context is required")
	}
	conflicts, err := service.store.ListConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list conflicts: %w", err)
	}
	result := make([]ConflictSummary, 0, len(conflicts))
	for _, conflict := range conflicts {
		result = append(result, ConflictSummary{
			ID: conflict.Local.ID, Type: conflict.Local.Data.Type,
			LocalVersion: conflict.Local.Version, RemoteVersion: conflict.Remote.Version,
			RemoteDeleted: conflict.Remote.DeletedAt != nil,
		})
	}
	return result, nil
}

// Resolve keeps the selected encrypted version. Keeping local queues a new
// update/delete for the next sync; keeping server completes resolution now.
func (service *ConflictService) Resolve(ctx context.Context, id model.ID, resolution storage.ConflictResolution) error {
	if ctx == nil {
		return errors.New("resolve conflict: context is required")
	}
	if id.IsZero() {
		return fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	if err := service.store.ResolveConflict(ctx, id, resolution); err != nil {
		return fmt.Errorf("resolve conflict: %w", err)
	}
	return nil
}
