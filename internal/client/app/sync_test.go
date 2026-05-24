package app

import (
	"context"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type syncAPIFake struct {
	created int
	pages   []clienttransport.SyncPage
}

func (fake *syncAPIFake) CreateRecord(_ context.Context, _ string, id model.ID, data clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error) {
	fake.created++
	now := time.Now().UTC()
	return clienttransport.RemoteRecord{ID: id, Data: data, Version: 1, Revision: 2, CreatedAt: now, UpdatedAt: now}, nil
}
func (fake *syncAPIFake) UpdateRecord(context.Context, string, model.ID, int64, clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error) {
	panic("unexpected UpdateRecord")
}
func (fake *syncAPIFake) DeleteRecord(context.Context, string, model.ID, int64) (clienttransport.RemoteRecord, error) {
	panic("unexpected DeleteRecord")
}
func (fake *syncAPIFake) SyncRecords(context.Context, string, int64, uint32) (clienttransport.SyncPage, error) {
	page := fake.pages[0]
	fake.pages = fake.pages[1:]
	return page, nil
}

type syncStoreFake struct {
	pending  []storage.LocalRecord
	saved    []storage.LocalRecord
	revision int64
	applied  []storage.LocalRecord
}

func (fake *syncStoreFake) ListPending(context.Context) ([]storage.LocalRecord, error) {
	return fake.pending, nil
}
func (fake *syncStoreFake) Save(_ context.Context, record storage.LocalRecord) error {
	fake.saved = append(fake.saved, record)
	return nil
}
func (fake *syncStoreFake) LastRevision(context.Context) (int64, error) { return fake.revision, nil }
func (fake *syncStoreFake) ApplyRemotePage(_ context.Context, records []storage.LocalRecord, revision int64) (int, error) {
	fake.applied = append(fake.applied, records...)
	fake.revision = revision
	return 0, nil
}

func TestSyncServiceUploadsAndDownloads(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	now := time.Now().UTC()
	data := clientcrypto.EncryptedRecordData{Type: model.RecordTypeText, EncryptionVersion: 1, EncryptedPayload: make([]byte, clientcrypto.AEADTagSize), EncryptedMetadata: make([]byte, clientcrypto.AEADTagSize), PayloadNonce: make([]byte, clientcrypto.NonceSize), MetadataNonce: make([]byte, clientcrypto.NonceSize)}
	pending := storage.LocalRecord{ID: id, Data: data, CreatedAt: now, UpdatedAt: now, SyncStatus: storage.SyncStatusCreated}
	remoteID, _ := model.ParseID("223e4567-e89b-42d3-a456-426614174000")
	remote := clienttransport.RemoteRecord{ID: remoteID, Data: data, Version: 1, Revision: 3, CreatedAt: now, UpdatedAt: now}
	api := &syncAPIFake{pages: []clienttransport.SyncPage{{Records: []clienttransport.RemoteRecord{remote}, NextRevision: 3}}}
	local := &syncStoreFake{pending: []storage.LocalRecord{pending}}
	service, err := NewSyncService(api, &sessionStoreFake{state: testSessionState()}, local)
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if report.Uploaded != 1 || report.Downloaded != 1 || report.Revision != 3 {
		t.Fatalf("Sync() report = %#v", report)
	}
	if api.created != 1 || len(local.saved) != 1 || local.saved[0].SyncStatus != storage.SyncStatusSynced {
		t.Fatalf("upload state: created=%d saved=%#v", api.created, local.saved)
	}
	if len(local.applied) != 1 || local.applied[0].ID != remoteID {
		t.Fatalf("applied = %#v", local.applied)
	}
}
