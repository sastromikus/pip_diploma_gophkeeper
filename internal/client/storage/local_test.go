package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/model"
)

func TestLocalDatabaseRoundTrip(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()
	record := testLocalRecord(t, SyncStatusSynced)

	if err := database.Save(ctx, record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := database.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertLocalRecord(t, got, record)

	got.Data.EncryptedPayload[0] ^= 0xff
	again, err := database.Get(ctx, record.ID)
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if again.Data.EncryptedPayload[0] != record.Data.EncryptedPayload[0] {
		t.Fatal("Get() returned storage-owned byte slices")
	}
}

func TestLocalDatabaseListAndPending(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	synced := testLocalRecord(t, SyncStatusSynced)
	pending := testLocalRecord(t, SyncStatusUpdated)
	pending.ID = mustID(t, "22222222-2222-4222-8222-222222222222")
	deleted := testLocalRecord(t, SyncStatusDeleted)
	deleted.ID = mustID(t, "33333333-3333-4333-8333-333333333333")
	deletedAt := deleted.UpdatedAt.Add(time.Second)
	deleted.DeletedAt = &deletedAt
	deleted.Data.EncryptedPayload = nil
	deleted.Data.EncryptedMetadata = nil
	deleted.Data.PayloadNonce = nil
	deleted.Data.MetadataNonce = nil

	for _, record := range []LocalRecord{synced, pending, deleted} {
		if err := database.Save(ctx, record); err != nil {
			t.Fatalf("Save(%s) error = %v", record.SyncStatus, err)
		}
	}

	active, err := database.List(ctx, false)
	if err != nil {
		t.Fatalf("List(false) error = %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("List(false) length = %d, want 2", len(active))
	}
	all, err := database.List(ctx, true)
	if err != nil {
		t.Fatalf("List(true) error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List(true) length = %d, want 3", len(all))
	}
	pendingRecords, err := database.ListPending(ctx)
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(pendingRecords) != 2 {
		t.Fatalf("ListPending() length = %d, want 2", len(pendingRecords))
	}
}

func TestLocalDatabaseRevisionIsMonotonic(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	revision, err := database.LastRevision(ctx)
	if err != nil {
		t.Fatalf("LastRevision() error = %v", err)
	}
	if revision != 0 {
		t.Fatalf("LastRevision() = %d, want 0", revision)
	}
	if err := database.SetLastRevision(ctx, 12); err != nil {
		t.Fatalf("SetLastRevision(12) error = %v", err)
	}
	if err := database.SetLastRevision(ctx, 11); err == nil {
		t.Fatal("SetLastRevision(11) error = nil, want backwards-revision error")
	}
	revision, err = database.LastRevision(ctx)
	if err != nil {
		t.Fatalf("LastRevision() after update error = %v", err)
	}
	if revision != 12 {
		t.Fatalf("LastRevision() = %d, want 12", revision)
	}
}

func TestLocalDatabaseDelete(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()
	record := testLocalRecord(t, SyncStatusSynced)
	if err := database.Save(ctx, record); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := database.Delete(ctx, record.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := database.Get(ctx, record.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
	if err := database.Delete(ctx, record.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("Delete() repeated error = %v, want ErrNotFound", err)
	}
}

func TestLocalRecordValidate(t *testing.T) {
	record := testLocalRecord(t, SyncStatusSynced)
	if err := record.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	created := testLocalRecord(t, SyncStatusCreated)
	created.Version = 0
	created.Revision = 0
	if err := created.Validate(); err != nil {
		t.Fatalf("created Validate() error = %v", err)
	}

	created.Version = 1
	if err := created.Validate(); err == nil {
		t.Fatal("created Validate() error = nil with server version")
	}

	deleted := testLocalRecord(t, SyncStatusDeleted)
	if err := deleted.Validate(); err == nil {
		t.Fatal("deleted Validate() error = nil without tombstone")
	}
}

func TestLocalDatabaseCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.db")
	database, err := OpenLocalDatabase(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenLocalDatabase() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() repeated error = %v", err)
	}
}

func openTestLocalDatabase(t *testing.T) *LocalDatabase {
	t.Helper()
	database, err := OpenLocalDatabase(context.Background(), filepath.Join(t.TempDir(), "gophkeeper.db"))
	if err != nil {
		t.Fatalf("OpenLocalDatabase() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return database
}

func testLocalRecord(t *testing.T, status SyncStatus) LocalRecord {
	t.Helper()
	createdAt := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	return LocalRecord{
		ID: mustID(t, "11111111-1111-4111-8111-111111111111"),
		Data: clientcrypto.EncryptedRecordData{
			Type: model.RecordTypeCredentials, EncryptionVersion: 1,
			EncryptedPayload: []byte{1, 2, 3}, EncryptedMetadata: []byte{4, 5, 6},
			PayloadNonce: []byte{7, 8}, MetadataNonce: []byte{9, 10},
		},
		Version: 1, Revision: 2, CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Second), SyncStatus: status,
	}
}

func mustID(t *testing.T, value string) model.ID {
	t.Helper()
	id, err := model.ParseID(value)
	if err != nil {
		t.Fatalf("ParseID(%q) error = %v", value, err)
	}
	return id
}

func assertLocalRecord(t *testing.T, got, want LocalRecord) {
	t.Helper()
	if got.ID != want.ID || got.Data.Type != want.Data.Type || got.Data.EncryptionVersion != want.Data.EncryptionVersion ||
		got.Version != want.Version || got.Revision != want.Revision || got.SyncStatus != want.SyncStatus ||
		!got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("record metadata = %#v, want %#v", got, want)
	}
	if string(got.Data.EncryptedPayload) != string(want.Data.EncryptedPayload) ||
		string(got.Data.EncryptedMetadata) != string(want.Data.EncryptedMetadata) ||
		string(got.Data.PayloadNonce) != string(want.Data.PayloadNonce) ||
		string(got.Data.MetadataNonce) != string(want.Data.MetadataNonce) {
		t.Fatalf("record encrypted data = %#v, want %#v", got.Data, want.Data)
	}
}
