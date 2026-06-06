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

func TestExecuteConflictCommandListAndResolveLocal(t *testing.T) {
	stub := &conflictStoreCLIStub{}
	service, err := clientapp.NewConflictService(stub)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := executeConflictCommand(conflictCommand{name: "conflicts"}, service, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("expected conflict list output")
	}

	id, _ := model.ParseID("11111111-1111-4111-8111-111111111111")
	output.Reset()
	if err := executeConflictCommand(conflictCommand{name: "resolve", id: id, resolution: clientstorage.ConflictResolutionLocal}, service, &output); err != nil {
		t.Fatal(err)
	}
	if stub.resolution != clientstorage.ConflictResolutionLocal {
		t.Fatalf("resolution = %q", stub.resolution)
	}
	if output.Len() == 0 {
		t.Fatal("expected resolution output")
	}
}

func TestParseConflictCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"resolve"}, {"resolve", "bad", "local"}, {"resolve", "11111111-1111-4111-8111-111111111111", "other"}, {"unknown"}} {
		if _, err := parseConflictCommand(args); err == nil {
			t.Fatalf("parseConflictCommand(%q) expected error", args)
		}
	}
}
