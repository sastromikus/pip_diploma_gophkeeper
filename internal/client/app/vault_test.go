package app

import (
	"context"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type vaultAPIFake struct {
	createdID model.ID
	created   clientcrypto.EncryptedRecordData
	record    clienttransport.RemoteRecord
	page      clienttransport.RecordPage
	updated   bool
	deleted   bool
}

func (fake *vaultAPIFake) CreateRecord(_ context.Context, _ string, id model.ID, data clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error) {
	fake.createdID, fake.created = id, data
	return clienttransport.RemoteRecord{ID: id, Data: data, Version: 1}, nil
}
func (fake *vaultAPIFake) GetRecord(context.Context, string, model.ID) (clienttransport.RemoteRecord, error) {
	return fake.record, nil
}
func (fake *vaultAPIFake) ListRecords(context.Context, string, string, uint32) (clienttransport.RecordPage, error) {
	return fake.page, nil
}
func (fake *vaultAPIFake) UpdateRecord(_ context.Context, _ string, _ model.ID, _ int64, _ clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error) {
	fake.updated = true
	return fake.record, nil
}
func (fake *vaultAPIFake) DeleteRecord(context.Context, string, model.ID, int64) (clienttransport.RemoteRecord, error) {
	fake.deleted = true
	return fake.record, nil
}

type sessionStoreFake struct{ state storage.SessionState }

func (fake *sessionStoreFake) Save(storage.SessionState) error     { return nil }
func (fake *sessionStoreFake) Load() (storage.SessionState, error) { return fake.state, nil }
func (fake *sessionStoreFake) Delete() error                       { return nil }

type vaultCryptoFake struct{}

func (vaultCryptoFake) OpenDataKey(string, string, clientcrypto.KeyEnvelope) ([]byte, error) {
	return make([]byte, 32), nil
}
func (vaultCryptoFake) EncryptRecord(_ []byte, _ model.ID, recordType model.RecordType, _ any, _ clientmodel.Metadata, _ clientcrypto.RecordLimits) (clientcrypto.EncryptedRecordData, error) {
	return clientcrypto.EncryptedRecordData{Type: recordType, EncryptionVersion: 1, EncryptedPayload: []byte{1}, EncryptedMetadata: []byte{2}, PayloadNonce: []byte{3}, MetadataNonce: []byte{4}}, nil
}
func (vaultCryptoFake) DecryptRecord(_ []byte, _ model.ID, _ clientcrypto.EncryptedRecordData, payload any, metadata *clientmodel.Metadata, _ clientcrypto.RecordLimits) error {
	*metadata = clientmodel.Metadata{Text: "note"}
	switch value := payload.(type) {
	case *clientmodel.Credentials:
		*value = clientmodel.Credentials{Name: "mail", Login: "user", Password: "secret"}
	case *clientmodel.Text:
		*value = clientmodel.Text{Title: "title", Body: "body"}
	}
	return nil
}

func testSessionState() storage.SessionState {
	return storage.SessionState{Login: "alice", Token: "token", ExpiresAt: time.Now().Add(time.Hour), EncryptedDataKey: []byte{1}, KeySalt: []byte{2}, KeyNonce: []byte{3}, KeyDerivationVersion: 1}
}

func TestVaultServiceCreate(t *testing.T) {
	api := &vaultAPIFake{}
	service, err := NewVaultService(api, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), "password", model.RecordTypeCredentials, clientmodel.Credentials{Name: "mail", Login: "user", Password: "secret"}, clientmodel.Metadata{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if record.ID.IsZero() || api.createdID.IsZero() {
		t.Fatal("Create() generated an empty ID")
	}
	if api.created.Type != model.RecordTypeCredentials {
		t.Fatalf("created type = %q", api.created.Type)
	}
}

func TestVaultServiceGet(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	api := &vaultAPIFake{record: clienttransport.RemoteRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeCredentials}, Version: 3}}
	service, _ := NewVaultService(api, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})
	view, err := service.Get(context.Background(), "password", id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	credentials, ok := view.Payload.(clientmodel.Credentials)
	if !ok || credentials.Name != "mail" || view.Metadata.Text != "note" {
		t.Fatalf("Get() view = %#v", view)
	}
}

func TestVaultServiceListAndDelete(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	record := clienttransport.RemoteRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeText}, Version: 2}
	api := &vaultAPIFake{record: record, page: clienttransport.RecordPage{Records: []clienttransport.RemoteRecord{record}}}
	service, _ := NewVaultService(api, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})
	summaries, err := service.List(context.Background(), "password")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Title != "title" {
		t.Fatalf("List() = %#v", summaries)
	}
	if err := service.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !api.deleted {
		t.Fatal("Delete() did not call remote API")
	}
}

func TestVaultServiceUpdate(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	record := clienttransport.RemoteRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeText}, Version: 4}
	api := &vaultAPIFake{record: record}
	service, _ := NewVaultService(api, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})

	_, err := service.Update(context.Background(), "password", id, model.RecordTypeText, clientmodel.Text{Title: "updated", Body: "body"}, clientmodel.Metadata{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !api.updated {
		t.Fatal("Update() did not call remote API")
	}
}

func TestVaultServiceRejectsRecordTypeChange(t *testing.T) {
	id, _ := model.ParseID("123e4567-e89b-42d3-a456-426614174000")
	api := &vaultAPIFake{record: clienttransport.RemoteRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeText}, Version: 1}}
	service, _ := NewVaultService(api, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})

	_, err := service.Update(context.Background(), "password", id, model.RecordTypeCredentials, clientmodel.Credentials{Name: "mail", Login: "user", Password: "secret"}, clientmodel.Metadata{})
	if err == nil {
		t.Fatal("Update() accepted a record type change")
	}
}

func TestVaultServiceRejectsZeroRecordID(t *testing.T) {
	service, _ := NewVaultService(&vaultAPIFake{}, &sessionStoreFake{state: testSessionState()}, vaultCryptoFake{})
	if _, err := service.Get(context.Background(), "password", ""); err == nil {
		t.Fatal("Get() accepted an empty record ID")
	}
	if err := service.Delete(context.Background(), ""); err == nil {
		t.Fatal("Delete() accepted an empty record ID")
	}
}
