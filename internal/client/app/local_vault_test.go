package app

import (
	"context"
	"errors"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type localVaultStoreFake struct {
	records map[model.ID]storage.LocalRecord
}

func newLocalVaultStoreFake() *localVaultStoreFake {
	return &localVaultStoreFake{records: make(map[model.ID]storage.LocalRecord)}
}

func (fake *localVaultStoreFake) Save(_ context.Context, record storage.LocalRecord) error {
	fake.records[record.ID] = record
	return nil
}
func (fake *localVaultStoreFake) Get(_ context.Context, id model.ID) (storage.LocalRecord, error) {
	record, ok := fake.records[id]
	if !ok {
		return storage.LocalRecord{}, model.ErrNotFound
	}
	return record, nil
}
func (fake *localVaultStoreFake) List(_ context.Context, includeDeleted bool) ([]storage.LocalRecord, error) {
	var records []storage.LocalRecord
	for _, record := range fake.records {
		if !includeDeleted && record.DeletedAt != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}
func (fake *localVaultStoreFake) Delete(_ context.Context, id model.ID) error {
	if _, ok := fake.records[id]; !ok {
		return model.ErrNotFound
	}
	delete(fake.records, id)
	return nil
}

type localVaultCryptoFake struct{}

func (localVaultCryptoFake) OpenDataKey(string, string, clientcrypto.KeyEnvelope) ([]byte, error) {
	return make([]byte, 32), nil
}
func (localVaultCryptoFake) EncryptRecord(_ []byte, _ model.ID, recordType model.RecordType, _ any, _ clientmodel.Metadata, _ clientcrypto.RecordLimits) (clientcrypto.EncryptedRecordData, error) {
	return clientcrypto.EncryptedRecordData{
		Type: recordType, EncryptionVersion: clientcrypto.CurrentEncryptionVersion,
		EncryptedPayload: make([]byte, clientcrypto.AEADTagSize), EncryptedMetadata: make([]byte, clientcrypto.AEADTagSize),
		PayloadNonce: make([]byte, clientcrypto.NonceSize), MetadataNonce: make([]byte, clientcrypto.NonceSize),
	}, nil
}
func (localVaultCryptoFake) DecryptRecord(_ []byte, _ model.ID, _ clientcrypto.EncryptedRecordData, payload any, metadata *clientmodel.Metadata, _ clientcrypto.RecordLimits) error {
	*metadata = clientmodel.Metadata{Text: "note"}
	switch value := payload.(type) {
	case *clientmodel.Credentials:
		*value = clientmodel.Credentials{Name: "mail", Login: "alice", Password: "secret"}
	case *clientmodel.Text:
		*value = clientmodel.Text{Title: "title", Body: "body"}
	}
	return nil
}

func TestLocalVaultServiceCreateUpdateAndDeleteUnsynced(t *testing.T) {
	local := newLocalVaultStoreFake()
	service, err := NewLocalVaultService(&sessionStoreFake{state: testSessionState()}, local, localVaultCryptoFake{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }

	created, err := service.Create(context.Background(), "password", model.RecordTypeCredentials, clientmodel.Credentials{Name: "mail", Login: "alice", Password: "secret"}, clientmodel.Metadata{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	stored := local.records[created.ID]
	if stored.SyncStatus != storage.SyncStatusCreated || stored.Version != 0 || stored.Revision != 0 {
		t.Fatalf("created local record = %#v", stored)
	}

	service.now = func() time.Time { return time.Unix(200, 0).UTC() }
	if _, err := service.Update(context.Background(), "password", created.ID, model.RecordTypeCredentials, clientmodel.Credentials{Name: "mail2", Login: "alice", Password: "secret"}, clientmodel.Metadata{}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := local.records[created.ID].SyncStatus; got != storage.SyncStatusCreated {
		t.Fatalf("updated unsynced status = %q", got)
	}

	if err := service.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := local.records[created.ID]; ok {
		t.Fatal("never-synced record was not removed locally")
	}
}

func TestLocalVaultServiceQueuesServerBackedUpdateAndDelete(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	now := time.Unix(100, 0).UTC()
	local := newLocalVaultStoreFake()
	local.records[id] = storage.LocalRecord{
		ID: id,
		Data: clientcrypto.EncryptedRecordData{
			Type: model.RecordTypeText, EncryptionVersion: clientcrypto.CurrentEncryptionVersion,
			EncryptedPayload: make([]byte, clientcrypto.AEADTagSize), EncryptedMetadata: make([]byte, clientcrypto.AEADTagSize),
			PayloadNonce: make([]byte, clientcrypto.NonceSize), MetadataNonce: make([]byte, clientcrypto.NonceSize),
		},
		Version: 3, Revision: 5, CreatedAt: now, UpdatedAt: now, SyncStatus: storage.SyncStatusSynced,
	}
	service, _ := NewLocalVaultService(&sessionStoreFake{state: testSessionState()}, local, localVaultCryptoFake{})
	service.now = func() time.Time { return time.Unix(200, 0).UTC() }

	if _, err := service.Update(context.Background(), "password", id, model.RecordTypeText, clientmodel.Text{Title: "updated", Body: "body"}, clientmodel.Metadata{}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := local.records[id].SyncStatus; got != storage.SyncStatusUpdated {
		t.Fatalf("updated status = %q", got)
	}
	if err := service.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	deleted := local.records[id]
	if deleted.SyncStatus != storage.SyncStatusDeleted || deleted.DeletedAt == nil {
		t.Fatalf("deleted local record = %#v", deleted)
	}
	if len(deleted.Data.EncryptedPayload) != 0 || len(deleted.Data.PayloadNonce) != 0 {
		t.Fatal("local tombstone retained encrypted data")
	}
}

func TestLocalVaultServiceGetAndList(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	now := time.Now().UTC()
	local := newLocalVaultStoreFake()
	local.records[id] = storage.LocalRecord{
		ID: id,
		Data: clientcrypto.EncryptedRecordData{
			Type: model.RecordTypeCredentials, EncryptionVersion: clientcrypto.CurrentEncryptionVersion,
			EncryptedPayload: make([]byte, clientcrypto.AEADTagSize), EncryptedMetadata: make([]byte, clientcrypto.AEADTagSize),
			PayloadNonce: make([]byte, clientcrypto.NonceSize), MetadataNonce: make([]byte, clientcrypto.NonceSize),
		},
		Version: 1, Revision: 1, CreatedAt: now, UpdatedAt: now, SyncStatus: storage.SyncStatusSynced,
	}
	service, _ := NewLocalVaultService(&sessionStoreFake{state: testSessionState()}, local, localVaultCryptoFake{})

	view, err := service.Get(context.Background(), "password", id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if credentials, ok := view.Payload.(clientmodel.Credentials); !ok || credentials.Name != "mail" {
		t.Fatalf("Get() payload = %#v", view.Payload)
	}
	summaries, err := service.List(context.Background(), "password")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Title != "mail" {
		t.Fatalf("List() = %#v", summaries)
	}
}

func TestLocalVaultServiceRejectsInvalidDependenciesAndMissingRecord(t *testing.T) {
	if _, err := NewLocalVaultService(nil, newLocalVaultStoreFake(), localVaultCryptoFake{}); err == nil {
		t.Fatal("NewLocalVaultService() accepted missing session store")
	}
	service, _ := NewLocalVaultService(&sessionStoreFake{state: testSessionState()}, newLocalVaultStoreFake(), localVaultCryptoFake{})
	if _, err := service.Get(context.Background(), "password", ""); err == nil {
		t.Fatal("Get() accepted zero ID")
	}
	if err := service.Delete(context.Background(), "missing-id"); !errors.Is(err, model.ErrInvalidInput) {
		// Parse validation is not performed by Delete; a non-canonical ID is still
		// rejected by the local store fake as not found. Keep this branch explicit.
		if !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("Delete() error = %v", err)
		}
	}
}
