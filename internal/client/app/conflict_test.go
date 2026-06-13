package app

import (
	"context"
	"testing"
	"time"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type conflictStoreStub struct {
	conflicts  []storage.RecordConflict
	resolvedID model.ID
	resolution storage.ConflictResolution
}

func (stub *conflictStoreStub) ListConflicts(context.Context) ([]storage.RecordConflict, error) {
	return stub.conflicts, nil
}

func (stub *conflictStoreStub) ResolveConflict(_ context.Context, id model.ID, resolution storage.ConflictResolution) error {
	stub.resolvedID = id
	stub.resolution = resolution
	return nil
}

func TestConflictServiceListAndResolve(t *testing.T) {
	id, err := model.ParseID("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	local := storage.LocalRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeCredentials}, Version: 2, SyncStatus: storage.SyncStatusConflict}
	remote := storage.LocalRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeCredentials}, Version: 3, CreatedAt: now, UpdatedAt: now, SyncStatus: storage.SyncStatusSynced}
	stub := &conflictStoreStub{conflicts: []storage.RecordConflict{{Local: local, Remote: remote}}}
	service, err := NewConflictService(stub)
	if err != nil {
		t.Fatalf("NewConflictService() error = %v", err)
	}
	var items []ConflictSummary
	for item, err := range service.List(context.Background()) {
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		items = append(items, item)
	}
	if len(items) != 1 {
		t.Fatalf("List() = %#v", items)
	}
	if items[0].LocalVersion != 2 || items[0].RemoteVersion != 3 {
		t.Fatalf("summary = %#v", items[0])
	}
	if err := service.Resolve(context.Background(), id, storage.ConflictResolutionLocal); err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if stub.resolvedID != id || stub.resolution != storage.ConflictResolutionLocal {
		t.Fatalf("resolved = %s, %s", stub.resolvedID, stub.resolution)
	}
}
