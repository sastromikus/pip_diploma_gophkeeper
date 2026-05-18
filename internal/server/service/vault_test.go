package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

const (
	testUserID   = model.ID("11111111-1111-4111-8111-111111111111")
	testRecordID = model.ID("22222222-2222-4222-8222-222222222222")
)

type vaultRepositoryStub struct {
	createFn func(context.Context, model.Record) (model.Record, error)
	getFn    func(context.Context, model.ID, model.ID) (model.Record, error)
	listFn   func(context.Context, model.ID, model.ID, int32) ([]model.Record, error)
	updateFn func(context.Context, model.Record, int64) (model.Record, error)
	deleteFn func(context.Context, model.ID, model.ID, int64) (model.Record, error)
	syncFn   func(context.Context, model.ID, int64, int32) ([]model.Record, error)
}

func (stub *vaultRepositoryStub) Create(ctx context.Context, record model.Record) (model.Record, error) {
	return stub.createFn(ctx, record)
}
func (stub *vaultRepositoryStub) Get(ctx context.Context, userID, recordID model.ID) (model.Record, error) {
	return stub.getFn(ctx, userID, recordID)
}
func (stub *vaultRepositoryStub) List(ctx context.Context, userID, afterID model.ID, limit int32) ([]model.Record, error) {
	return stub.listFn(ctx, userID, afterID, limit)
}
func (stub *vaultRepositoryStub) Update(ctx context.Context, record model.Record, expectedVersion int64) (model.Record, error) {
	return stub.updateFn(ctx, record, expectedVersion)
}
func (stub *vaultRepositoryStub) Delete(ctx context.Context, userID, recordID model.ID, expectedVersion int64) (model.Record, error) {
	return stub.deleteFn(ctx, userID, recordID, expectedVersion)
}
func (stub *vaultRepositoryStub) ListChangedAfter(ctx context.Context, userID model.ID, revision int64, limit int32) ([]model.Record, error) {
	return stub.syncFn(ctx, userID, revision, limit)
}

func TestVaultServiceCreate(t *testing.T) {
	repository := &vaultRepositoryStub{createFn: func(_ context.Context, record model.Record) (model.Record, error) {
		if record.UserID != testUserID || record.ID != testRecordID {
			t.Fatalf("unexpected IDs: %+v", record)
		}
		record.Version, record.Revision = 1, 1
		record.CreatedAt, record.UpdatedAt = time.Now(), time.Now()
		return record, nil
	}}
	vault := newTestVault(t, repository)
	created, err := vault.Create(context.Background(), CreateRecordInput{UserID: testUserID, ID: testRecordID, Data: validEncryptedInput()})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("Version = %d, want 1", created.Version)
	}
}

func TestVaultServiceCreateRejectsOversizedPayload(t *testing.T) {
	vault := newTestVault(t, &vaultRepositoryStub{})
	input := validEncryptedInput()
	input.EncryptedPayload = make([]byte, 1025)
	_, err := vault.Create(context.Background(), CreateRecordInput{UserID: testUserID, ID: testRecordID, Data: input})
	if !errors.Is(err, model.ErrPayloadTooLarge) {
		t.Fatalf("Create() error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestVaultServiceListPagination(t *testing.T) {
	records := []model.Record{{ID: model.ID("22222222-2222-4222-8222-222222222221")}, {ID: testRecordID}, {ID: model.ID("22222222-2222-4222-8222-222222222223")}}
	repository := &vaultRepositoryStub{listFn: func(_ context.Context, _ model.ID, _ model.ID, limit int32) ([]model.Record, error) {
		if limit != 3 {
			t.Fatalf("repository limit = %d, want 3", limit)
		}
		return records, nil
	}}
	vault := newTestVault(t, repository)
	result, err := vault.List(context.Background(), ListRecordsInput{UserID: testUserID, Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !result.HasMore || len(result.Records) != 2 || result.NextID != testRecordID {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestVaultServiceSyncCursor(t *testing.T) {
	records := []model.Record{{Revision: 11}, {Revision: 12}, {Revision: 13}}
	repository := &vaultRepositoryStub{syncFn: func(_ context.Context, _ model.ID, after int64, limit int32) ([]model.Record, error) {
		if after != 10 || limit != 3 {
			t.Fatalf("after=%d limit=%d", after, limit)
		}
		return records, nil
	}}
	vault := newTestVault(t, repository)
	result, err := vault.Sync(context.Background(), SyncRecordsInput{UserID: testUserID, AfterRevision: 10, Limit: 2})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !result.HasMore || len(result.Records) != 2 || result.NextRevision != 12 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestVaultServicePreservesRepositoryErrors(t *testing.T) {
	repository := &vaultRepositoryStub{getFn: func(context.Context, model.ID, model.ID) (model.Record, error) {
		return model.Record{}, model.ErrNotFound
	}}
	vault := newTestVault(t, repository)
	_, err := vault.Get(context.Background(), testUserID, testRecordID)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func newTestVault(t *testing.T, repository RecordRepository) *VaultService {
	t.Helper()
	vault, err := NewVaultService(repository, model.RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 256})
	if err != nil {
		t.Fatalf("NewVaultService() error = %v", err)
	}
	return vault
}

func validEncryptedInput() EncryptedRecordInput {
	return EncryptedRecordInput{
		Type: model.RecordTypeCredentials, EncryptionVersion: 1,
		EncryptedPayload: []byte("payload"), EncryptedMetadata: []byte("metadata"),
		PayloadNonce: []byte("payload-nonce"), MetadataNonce: []byte("metadata-nonce"),
	}
}
