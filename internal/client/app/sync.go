package app

import (
	"context"
	"errors"
	"fmt"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const syncPageSize uint32 = 100

// SyncAPI describes remote operations required by client synchronization.
type SyncAPI interface {
	CreateRecord(context.Context, string, model.ID, clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error)
	UpdateRecord(context.Context, string, model.ID, int64, clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error)
	DeleteRecord(context.Context, string, model.ID, int64) (clienttransport.RemoteRecord, error)
	SyncRecords(context.Context, string, int64, uint32) (clienttransport.SyncPage, error)
}

// SyncLocalStore describes encrypted local synchronization storage.
type SyncLocalStore interface {
	ListPending(context.Context) ([]storage.LocalRecord, error)
	Save(context.Context, storage.LocalRecord) error
	LastRevision(context.Context) (int64, error)
	ApplyRemotePage(context.Context, []storage.LocalRecord, int64) (int, error)
}

// SyncReport summarizes one synchronization run.
type SyncReport struct {
	Uploaded   int
	Downloaded int
	Conflicts  int
	Revision   int64
}

// SyncService uploads pending encrypted changes and downloads server changes.
type SyncService struct {
	api   SyncAPI
	store SessionStore
	local SyncLocalStore
}

// NewSyncService creates a synchronization application service.
func NewSyncService(api SyncAPI, store SessionStore, local SyncLocalStore) (*SyncService, error) {
	if api == nil || store == nil || local == nil {
		return nil, errors.New("client synchronization dependencies are required")
	}
	return &SyncService{api: api, store: store, local: local}, nil
}

// Sync performs a complete encrypted synchronization cycle.
func (service *SyncService) Sync(ctx context.Context) (SyncReport, error) {
	if ctx == nil {
		return SyncReport{}, errors.New("synchronization context is required")
	}
	state, err := service.store.Load()
	if err != nil {
		return SyncReport{}, fmt.Errorf("load current session: %w", err)
	}
	report := SyncReport{}
	pending, err := service.local.ListPending(ctx)
	if err != nil {
		return report, fmt.Errorf("list pending local records: %w", err)
	}
	for _, record := range pending {
		if record.SyncStatus == storage.SyncStatusConflict {
			report.Conflicts++
			continue
		}
		remote, err := service.upload(ctx, state.Token, record)
		if err != nil {
			if errors.Is(err, model.ErrVersionConflict) || errors.Is(err, model.ErrAlreadyExists) {
				record.SyncStatus = storage.SyncStatusConflict
				if saveErr := service.local.Save(ctx, record); saveErr != nil {
					return report, fmt.Errorf("save upload conflict for %s: %w", record.ID, saveErr)
				}
				report.Conflicts++
				continue
			}
			return report, fmt.Errorf("upload local record %s: %w", record.ID, err)
		}
		if err := service.local.Save(ctx, localRecord(remote, storage.SyncStatusSynced)); err != nil {
			return report, fmt.Errorf("save uploaded record %s: %w", record.ID, err)
		}
		report.Uploaded++
	}

	cursor, err := service.local.LastRevision(ctx)
	if err != nil {
		return report, fmt.Errorf("read synchronization cursor: %w", err)
	}
	for {
		page, err := service.api.SyncRecords(ctx, state.Token, cursor, syncPageSize)
		if err != nil {
			return report, err
		}
		changes := make([]storage.LocalRecord, 0, len(page.Records))
		for _, record := range page.Records {
			changes = append(changes, localRecord(record, storage.SyncStatusSynced))
		}
		conflicts, err := service.local.ApplyRemotePage(ctx, changes, page.NextRevision)
		if err != nil {
			return report, fmt.Errorf("apply synchronized page: %w", err)
		}
		report.Downloaded += len(changes)
		report.Conflicts += conflicts
		cursor = page.NextRevision
		if !page.HasMore {
			break
		}
	}
	report.Revision = cursor
	return report, nil
}

func (service *SyncService) upload(ctx context.Context, token string, record storage.LocalRecord) (clienttransport.RemoteRecord, error) {
	switch record.SyncStatus {
	case storage.SyncStatusCreated:
		return service.api.CreateRecord(ctx, token, record.ID, record.Data)
	case storage.SyncStatusUpdated:
		return service.api.UpdateRecord(ctx, token, record.ID, record.Version, record.Data)
	case storage.SyncStatusDeleted:
		return service.api.DeleteRecord(ctx, token, record.ID, record.Version)
	default:
		return clienttransport.RemoteRecord{}, fmt.Errorf("%w: record %s has unsupported pending state %q", model.ErrInvalidInput, record.ID, record.SyncStatus)
	}
}

func localRecord(record clienttransport.RemoteRecord, status storage.SyncStatus) storage.LocalRecord {
	return storage.LocalRecord{
		ID: record.ID, Data: record.Data, Version: record.Version, Revision: record.Revision,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, DeletedAt: record.DeletedAt,
		SyncStatus: status,
	}
}
