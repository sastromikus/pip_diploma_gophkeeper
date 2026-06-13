package app

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

// LocalVaultStore describes encrypted local-record operations used by the
// offline-first vault service.
type LocalVaultStore interface {
	Save(context.Context, storage.LocalRecord) error
	Get(context.Context, model.ID) (storage.LocalRecord, error)
	List(context.Context, bool) ([]storage.LocalRecord, error)
	Delete(context.Context, model.ID) error
}

// LocalVaultService manages encrypted records in the local SQLite store. The
// sync command is responsible for exchanging pending ciphertext with the
// server.
type LocalVaultService struct {
	store  SessionStore
	local  LocalVaultStore
	crypto VaultCrypto
	limits clientcrypto.RecordLimits
	now    func() time.Time
}

// NewLocalVaultService creates an offline-first encrypted vault service.
func NewLocalVaultService(store SessionStore, local LocalVaultStore, crypto VaultCrypto) (*LocalVaultService, error) {
	if store == nil || local == nil || crypto == nil {
		return nil, errors.New("local vault dependencies are required")
	}
	return &LocalVaultService{
		store:  store,
		local:  local,
		crypto: crypto,
		limits: clientcrypto.RecordLimits{
			MaxBinarySize:            defaultMaxBinarySize,
			MaxEncryptedPayloadSize:  defaultMaxEncryptedPayloadSize,
			MaxEncryptedMetadataSize: defaultMaxEncryptedMetadataSize,
		},
		now: time.Now,
	}, nil
}

// Create encrypts a record and queues it locally for synchronization.
func (service *LocalVaultService) Create(ctx context.Context, password string, recordType model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	if ctx == nil {
		return clienttransport.RemoteRecord{}, errors.New("create local record: context is required")
	}
	id, err := generateID()
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	_, key, err := service.unlock(password)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	defer clientcrypto.Wipe(key)

	encrypted, err := service.crypto.EncryptRecord(key, id, recordType, payload, metadata, service.limits)
	if err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("encrypt local record: %w", err)
	}
	now := service.now().UTC()
	local := storage.LocalRecord{
		ID: id, Data: encrypted, CreatedAt: now, UpdatedAt: now,
		SyncStatus: storage.SyncStatusCreated,
	}
	if err := service.local.Save(ctx, local); err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("save created local record: %w", err)
	}
	return remoteFromLocal(local), nil
}

// Get decrypts one active local record.
func (service *LocalVaultService) Get(ctx context.Context, password string, id model.ID) (RecordView, error) {
	if ctx == nil {
		return RecordView{}, errors.New("get local record: context is required")
	}
	if id.IsZero() {
		return RecordView{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	local, err := service.local.Get(ctx, id)
	if err != nil {
		return RecordView{}, err
	}
	if local.DeletedAt != nil {
		return RecordView{}, model.ErrNotFound
	}
	_, key, err := service.unlock(password)
	if err != nil {
		return RecordView{}, err
	}
	defer clientcrypto.Wipe(key)

	payload, err := payloadTarget(local.Data.Type)
	if err != nil {
		return RecordView{}, err
	}
	metadata := clientmodel.Metadata{}
	if err := service.crypto.DecryptRecord(key, local.ID, local.Data, payload, &metadata, service.limits); err != nil {
		return RecordView{}, fmt.Errorf("decrypt local record: %w", err)
	}
	return RecordView{ID: local.ID, Type: local.Data.Type, Version: local.Version, Payload: dereferencePayload(payload), Metadata: metadata}, nil
}

// List lazily yields decrypted display-safe summaries of active local records.
func (service *LocalVaultService) List(ctx context.Context, password string) iter.Seq2[RecordSummary, error] {
	return func(yield func(RecordSummary, error) bool) {
		if ctx == nil {
			yield(RecordSummary{}, errors.New("list local records: context is required"))
			return
		}
		records, err := service.local.List(ctx, false)
		if err != nil {
			yield(RecordSummary{}, fmt.Errorf("list encrypted local records: %w", err))
			return
		}
		if len(records) == 0 {
			return
		}
		_, key, err := service.unlock(password)
		if err != nil {
			yield(RecordSummary{}, err)
			return
		}
		defer clientcrypto.Wipe(key)

		for _, record := range records {
			payload, err := payloadTarget(record.Data.Type)
			if err != nil {
				yield(RecordSummary{}, fmt.Errorf("prepare local record %s: %w", record.ID, err))
				return
			}
			metadata := clientmodel.Metadata{}
			if err := service.crypto.DecryptRecord(key, record.ID, record.Data, payload, &metadata, service.limits); err != nil {
				yield(RecordSummary{}, fmt.Errorf("decrypt local record %s: %w", record.ID, err))
				return
			}
			summary := RecordSummary{ID: record.ID, Type: record.Data.Type, Version: record.Version, Title: payloadTitle(dereferencePayload(payload)), SyncStatus: record.SyncStatus}
			if !yield(summary, nil) {
				return
			}
		}
	}
}

// Update encrypts replacement data and marks the local record as pending.
func (service *LocalVaultService) Update(ctx context.Context, password string, id model.ID, recordType model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	if ctx == nil {
		return clienttransport.RemoteRecord{}, errors.New("update local record: context is required")
	}
	if id.IsZero() {
		return clienttransport.RemoteRecord{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	current, err := service.local.Get(ctx, id)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	if current.DeletedAt != nil {
		return clienttransport.RemoteRecord{}, model.ErrNotFound
	}
	if current.Data.Type != recordType {
		return clienttransport.RemoteRecord{}, fmt.Errorf("%w: changing record type is not supported", model.ErrInvalidInput)
	}
	_, key, err := service.unlock(password)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	defer clientcrypto.Wipe(key)

	encrypted, err := service.crypto.EncryptRecord(key, id, recordType, payload, metadata, service.limits)
	if err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("encrypt updated local record: %w", err)
	}
	current.Data = encrypted
	current.UpdatedAt = service.now().UTC()
	switch current.SyncStatus {
	case storage.SyncStatusCreated:
		// It has never reached the server, so it remains a create operation.
	case storage.SyncStatusConflict:
		// Preserve conflict status until the user explicitly resolves it.
	default:
		current.SyncStatus = storage.SyncStatusUpdated
	}
	if err := service.local.Save(ctx, current); err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("save updated local record: %w", err)
	}
	return remoteFromLocal(current), nil
}

// Delete queues a server-backed record for deletion or removes a never-synced
// local record immediately.
func (service *LocalVaultService) Delete(ctx context.Context, id model.ID) error {
	if ctx == nil {
		return errors.New("delete local record: context is required")
	}
	if id.IsZero() {
		return fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	current, err := service.local.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.SyncStatus == storage.SyncStatusConflict {
		return fmt.Errorf("%w: resolve record conflict before deleting", model.ErrVersionConflict)
	}
	if current.SyncStatus == storage.SyncStatusCreated {
		if err := service.local.Delete(ctx, id); err != nil {
			return fmt.Errorf("remove unsynchronized local record: %w", err)
		}
		return nil
	}
	if current.DeletedAt != nil {
		return model.ErrNotFound
	}
	now := service.now().UTC()
	current.Data.EncryptedPayload = nil
	current.Data.EncryptedMetadata = nil
	current.Data.PayloadNonce = nil
	current.Data.MetadataNonce = nil
	current.UpdatedAt = now
	current.DeletedAt = &now
	current.SyncStatus = storage.SyncStatusDeleted
	if err := service.local.Save(ctx, current); err != nil {
		return fmt.Errorf("save local deletion: %w", err)
	}
	return nil
}

func (service *LocalVaultService) unlock(password string) (storage.SessionState, []byte, error) {
	if password == "" {
		return storage.SessionState{}, nil, errors.New("master password is required")
	}
	state, err := service.store.Load()
	if err != nil {
		return storage.SessionState{}, nil, fmt.Errorf("load current session: %w", err)
	}
	envelope := clientcrypto.KeyEnvelope{EncryptedDataKey: state.EncryptedDataKey, Salt: state.KeySalt, Nonce: state.KeyNonce, KeyDerivationVersion: state.KeyDerivationVersion}
	key, err := service.crypto.OpenDataKey(password, state.Login, envelope)
	if err != nil {
		return storage.SessionState{}, nil, fmt.Errorf("unlock account data key: %w", err)
	}
	return state, key, nil
}

func remoteFromLocal(record storage.LocalRecord) clienttransport.RemoteRecord {
	return clienttransport.RemoteRecord{
		ID: record.ID, Data: record.Data, Version: record.Version, Revision: record.Revision,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, DeletedAt: record.DeletedAt,
	}
}
