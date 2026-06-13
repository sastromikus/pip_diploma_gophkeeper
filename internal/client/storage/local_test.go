package storage

import (
	"bytes"
	"context"
	"database/sql"
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

func TestLocalRecordValidateRejectsMalformedCryptographicData(t *testing.T) {
	record := testLocalRecord(t, SyncStatusSynced)
	record.Data.PayloadNonce = []byte{1}
	if err := record.Validate(); !errors.Is(err, clientcrypto.ErrInvalidKeyMaterial) {
		t.Fatalf("Validate() nonce error = %v, want ErrInvalidKeyMaterial", err)
	}

	record = testLocalRecord(t, SyncStatusSynced)
	record.Data.EncryptedPayload = make([]byte, clientcrypto.AEADTagSize-1)
	if err := record.Validate(); !errors.Is(err, clientcrypto.ErrInvalidKeyMaterial) {
		t.Fatalf("Validate() ciphertext error = %v, want ErrInvalidKeyMaterial", err)
	}

	record = testLocalRecord(t, SyncStatusSynced)
	record.Data.EncryptionVersion++
	if err := record.Validate(); !errors.Is(err, clientcrypto.ErrUnsupportedVersion) {
		t.Fatalf("Validate() version error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestLocalRecordValidateRejectsInconsistentTombstoneStatus(t *testing.T) {
	record := testLocalRecord(t, SyncStatusUpdated)
	deletedAt := record.UpdatedAt.Add(time.Second)
	record.DeletedAt = &deletedAt
	record.Data.EncryptedPayload = nil
	record.Data.EncryptedMetadata = nil
	record.Data.PayloadNonce = nil
	record.Data.MetadataNonce = nil
	if err := record.Validate(); err == nil {
		t.Fatal("Validate() error = nil for updated tombstone")
	}
}

func TestMigrationRejectsNewerSchemaBeforeCreatingDataTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL); INSERT INTO schema_version(version) VALUES (?)`, localSchemaVersion+1); err != nil {
		_ = db.Close()
		t.Fatalf("prepare future schema error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := OpenLocalDatabase(context.Background(), path); err == nil {
		t.Fatal("OpenLocalDatabase() error = nil for newer schema")
	}

	db, err = sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("reopen database error = %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'records'`).Scan(&count); err != nil {
		t.Fatalf("inspect records table error = %v", err)
	}
	if count != 0 {
		t.Fatalf("records table count = %d, want 0", count)
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
			Type:              model.RecordTypeCredentials,
			EncryptionVersion: clientcrypto.CurrentEncryptionVersion,
			EncryptedPayload:  make([]byte, clientcrypto.AEADTagSize),
			EncryptedMetadata: make([]byte, clientcrypto.AEADTagSize),
			PayloadNonce:      make([]byte, clientcrypto.NonceSize),
			MetadataNonce:     make([]byte, clientcrypto.NonceSize),
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

func TestApplyRemotePageAdvancesCursorAndPreservesPendingConflict(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	pending := testLocalRecord(t, SyncStatusUpdated)
	if err := database.Save(ctx, pending); err != nil {
		t.Fatalf("Save() pending error = %v", err)
	}
	remote := pending
	remote.Version = 2
	remote.Revision = 3
	remote.UpdatedAt = remote.UpdatedAt.Add(time.Second)
	remote.SyncStatus = SyncStatusSynced

	conflicts, err := database.ApplyRemotePage(ctx, []LocalRecord{remote}, 3)
	if err != nil {
		t.Fatalf("ApplyRemotePage() error = %v", err)
	}
	if conflicts != 1 {
		t.Fatalf("conflicts = %d, want 1", conflicts)
	}
	stored, err := database.Get(ctx, pending.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.SyncStatus != SyncStatusConflict || stored.Version != pending.Version {
		t.Fatalf("stored conflict = %#v", stored)
	}
	revision, err := database.LastRevision(ctx)
	if err != nil || revision != 3 {
		t.Fatalf("LastRevision() = %d, %v", revision, err)
	}
}

func TestApplyRemotePageStoresNewRecord(t *testing.T) {
	database := openTestLocalDatabase(t)
	record := testLocalRecord(t, SyncStatusSynced)
	conflicts, err := database.ApplyRemotePage(context.Background(), []LocalRecord{record}, record.Revision)
	if err != nil {
		t.Fatalf("ApplyRemotePage() error = %v", err)
	}
	if conflicts != 0 {
		t.Fatalf("conflicts = %d, want 0", conflicts)
	}
	stored, err := database.Get(context.Background(), record.ID)
	if err != nil || stored.Revision != record.Revision {
		t.Fatalf("Get() = %#v, %v", stored, err)
	}
}

func TestConflictLifecycle(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	local := testLocalRecord(t, SyncStatusUpdated)
	if err := database.Save(ctx, local); err != nil {
		t.Fatalf("Save() local error = %v", err)
	}
	remote := local
	remote.Data.EncryptedPayload = bytes.Repeat([]byte{9}, clientcrypto.AEADTagSize)
	remote.Version++
	remote.Revision++
	remote.UpdatedAt = remote.UpdatedAt.Add(time.Second)
	remote.SyncStatus = SyncStatusSynced

	conflicts, err := database.ApplyRemotePage(ctx, []LocalRecord{remote}, remote.Revision)
	if err != nil || conflicts != 1 {
		t.Fatalf("ApplyRemotePage() = %d, %v", conflicts, err)
	}
	listed, err := database.ListConflicts(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListConflicts() = %#v, %v", listed, err)
	}
	if listed[0].Local.Version != local.Version || listed[0].Remote.Version != remote.Version {
		t.Fatalf("conflict versions = %#v", listed[0])
	}

	if err := database.ResolveConflict(ctx, local.ID, ConflictResolutionLocal); err != nil {
		t.Fatalf("ResolveConflict(local) error = %v", err)
	}
	resolved, err := database.Get(ctx, local.ID)
	if err != nil {
		t.Fatalf("Get() resolved local error = %v", err)
	}
	if resolved.SyncStatus != SyncStatusUpdated || resolved.Version != remote.Version || resolved.Revision != remote.Revision {
		t.Fatalf("resolved local = %#v", resolved)
	}
	if _, err := database.GetConflict(ctx, local.ID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("GetConflict() error = %v, want ErrNotFound", err)
	}
}

func TestResolveConflictKeepsServerVersion(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	local := testLocalRecord(t, SyncStatusUpdated)
	if err := database.Save(ctx, local); err != nil {
		t.Fatalf("Save() local error = %v", err)
	}
	remote := local
	remote.Data.EncryptedPayload = bytes.Repeat([]byte{7}, clientcrypto.AEADTagSize)
	remote.Version++
	remote.Revision++
	remote.UpdatedAt = remote.UpdatedAt.Add(time.Second)
	remote.SyncStatus = SyncStatusSynced
	if _, err := database.ApplyRemotePage(ctx, []LocalRecord{remote}, remote.Revision); err != nil {
		t.Fatalf("ApplyRemotePage() error = %v", err)
	}
	if err := database.ResolveConflict(ctx, local.ID, ConflictResolutionServer); err != nil {
		t.Fatalf("ResolveConflict(server) error = %v", err)
	}
	resolved, err := database.Get(ctx, local.ID)
	if err != nil {
		t.Fatalf("Get() resolved server error = %v", err)
	}
	if resolved.SyncStatus != SyncStatusSynced || string(resolved.Data.EncryptedPayload) != string(remote.Data.EncryptedPayload) {
		t.Fatalf("resolved server = %#v", resolved)
	}
}

func TestSaveConflictNormalizesLocallyCreatedRecord(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	local := testLocalRecord(t, SyncStatusCreated)
	local.Version = 0
	local.Revision = 0
	if err := database.Save(ctx, local); err != nil {
		t.Fatalf("Save() local error = %v", err)
	}
	remote := testLocalRecord(t, SyncStatusSynced)
	remote.ID = local.ID
	remote.Data.EncryptedPayload = bytes.Repeat([]byte{8}, clientcrypto.AEADTagSize)
	remote.Version = 3
	remote.Revision = 7
	remote.UpdatedAt = remote.UpdatedAt.Add(time.Second)

	if err := database.SaveConflict(ctx, local, remote); err != nil {
		t.Fatalf("SaveConflict() error = %v", err)
	}
	conflict, err := database.GetConflict(ctx, local.ID)
	if err != nil {
		t.Fatalf("GetConflict() error = %v", err)
	}
	if conflict.Local.SyncStatus != SyncStatusConflict || conflict.Local.Version != remote.Version || conflict.Local.Revision != remote.Revision {
		t.Fatalf("normalized local conflict = %#v", conflict.Local)
	}
	if !bytes.Equal(conflict.Local.Data.EncryptedPayload, local.Data.EncryptedPayload) {
		t.Fatal("SaveConflict() did not preserve local ciphertext")
	}
}

func TestApplyRemotePageDoesNotOverwriteExistingConflict(t *testing.T) {
	database := openTestLocalDatabase(t)
	ctx := context.Background()

	local := testLocalRecord(t, SyncStatusUpdated)
	if err := database.Save(ctx, local); err != nil {
		t.Fatalf("Save() local error = %v", err)
	}
	remote := local
	remote.Data.EncryptedPayload = bytes.Repeat([]byte{6}, clientcrypto.AEADTagSize)
	remote.Version++
	remote.Revision++
	remote.UpdatedAt = remote.UpdatedAt.Add(time.Second)
	remote.SyncStatus = SyncStatusSynced
	if err := database.SaveConflict(ctx, local, remote); err != nil {
		t.Fatalf("SaveConflict() error = %v", err)
	}

	conflicts, err := database.ApplyRemotePage(ctx, []LocalRecord{remote}, remote.Revision)
	if err != nil {
		t.Fatalf("ApplyRemotePage() error = %v", err)
	}
	if conflicts != 0 {
		t.Fatalf("ApplyRemotePage() conflicts = %d, want 0 for existing conflict", conflicts)
	}
	stored, err := database.Get(ctx, local.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.SyncStatus != SyncStatusConflict || !bytes.Equal(stored.Data.EncryptedPayload, local.Data.EncryptedPayload) {
		t.Fatalf("existing local conflict was overwritten: %#v", stored)
	}
}
