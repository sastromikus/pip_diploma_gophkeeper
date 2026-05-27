package main

import (
	"bytes"
	"context"
	"testing"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientstorage "github.com/sastromikus/gophkeeper/internal/client/storage"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type conflictStoreCLIStub struct {
	resolution clientstorage.ConflictResolution
}

func (stub *conflictStoreCLIStub) ListConflicts(context.Context) ([]clientstorage.RecordConflict, error) {
	id, _ := model.ParseID("11111111-1111-4111-8111-111111111111")
	return []clientstorage.RecordConflict{{
		Local:  clientstorage.LocalRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeText}, Version: 1, SyncStatus: clientstorage.SyncStatusConflict},
		Remote: clientstorage.LocalRecord{ID: id, Data: clientcrypto.EncryptedRecordData{Type: model.RecordTypeText}, Version: 2, SyncStatus: clientstorage.SyncStatusSynced},
	}}, nil
}

func (stub *conflictStoreCLIStub) ResolveConflict(_ context.Context, _ model.ID, resolution clientstorage.ConflictResolution) error {
	stub.resolution = resolution
	return nil
}

func TestParseAndExecuteConflictCommands(t *testing.T) {
	command, err := parseConflictCommand([]string{"resolve", "11111111-1111-4111-8111-111111111111", "server", "-storage", "vault.db"})
	if err != nil {
		t.Fatalf("parseConflictCommand() error = %v", err)
	}
	if command.resolution != clientstorage.ConflictResolutionServer || len(command.configArgs) != 2 {
		t.Fatalf("command = %#v", command)
	}
	stub := &conflictStoreCLIStub{}
	service, err := clientapp.NewConflictService(stub)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := executeConflictCommand(command, service, &output); err != nil {
		t.Fatalf("executeConflictCommand() error = %v", err)
	}
	if stub.resolution != clientstorage.ConflictResolutionServer {
		t.Fatalf("resolution = %q", stub.resolution)
	}
}
