package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

type vaultServiceCLIStub struct {
	createdType model.RecordType
	created     any
	metadata    clientmodel.Metadata
	getResult   clientapp.RecordView
	listResult  []clientapp.RecordSummary
	updated     any
	deletedID   model.ID
	err         error
}

func (stub *vaultServiceCLIStub) Create(_ context.Context, _ string, recordType model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	if stub.err != nil {
		return clienttransport.RemoteRecord{}, stub.err
	}
	stub.createdType = recordType
	stub.created = payload
	stub.metadata = metadata
	id, _ := model.ParseID(testRecordID)
	return clienttransport.RemoteRecord{ID: id, Version: 1}, nil
}

func (stub *vaultServiceCLIStub) Get(_ context.Context, _ string, _ model.ID) (clientapp.RecordView, error) {
	if stub.err != nil {
		return clientapp.RecordView{}, stub.err
	}
	return stub.getResult, nil
}

func (stub *vaultServiceCLIStub) List(_ context.Context, _ string) ([]clientapp.RecordSummary, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	return stub.listResult, nil
}

func (stub *vaultServiceCLIStub) Update(_ context.Context, _ string, _ model.ID, _ model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	if stub.err != nil {
		return clienttransport.RemoteRecord{}, stub.err
	}
	stub.updated = payload
	stub.metadata = metadata
	id, _ := model.ParseID(testRecordID)
	return clienttransport.RemoteRecord{ID: id, Version: 2}, nil
}

func (stub *vaultServiceCLIStub) Delete(_ context.Context, id model.ID) error {
	if stub.err != nil {
		return stub.err
	}
	stub.deletedID = id
	return nil
}

func TestExecuteVaultCommandAddAndList(t *testing.T) {
	stub := &vaultServiceCLIStub{}
	var output bytes.Buffer
	command := vaultCommand{name: "add", recordType: model.RecordTypeCredentials}
	input := strings.NewReader("master\nmail\nalice@example.com\nsecret\nwork\n")
	if err := executeVaultCommand(command, stub, input, &output); err != nil {
		t.Fatalf("executeVaultCommand(add) error = %v", err)
	}
	if stub.createdType != model.RecordTypeCredentials {
		t.Fatalf("created type = %q", stub.createdType)
	}
	if _, ok := stub.created.(clientmodel.Credentials); !ok {
		t.Fatalf("created payload type = %T", stub.created)
	}
	if !strings.Contains(output.String(), "Record saved locally") {
		t.Fatalf("output = %q", output.String())
	}

	id, _ := model.ParseID(testRecordID)
	stub.listResult = []clientapp.RecordSummary{{ID: id, Type: model.RecordTypeCredentials, Version: 1, Title: "mail"}}
	output.Reset()
	if err := executeVaultCommand(vaultCommand{name: "list"}, stub, strings.NewReader("master\n"), &output); err != nil {
		t.Fatalf("executeVaultCommand(list) error = %v", err)
	}
	if !strings.Contains(output.String(), testRecordID) || !strings.Contains(output.String(), "mail") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestExecuteVaultCommandGetUpdateDelete(t *testing.T) {
	id, _ := model.ParseID(testRecordID)
	stub := &vaultServiceCLIStub{getResult: clientapp.RecordView{
		ID: id, Type: model.RecordTypeText, Version: 1,
		Payload: clientmodel.Text{Title: "old", Body: "body"},
	}}
	var output bytes.Buffer
	if err := executeVaultCommand(vaultCommand{name: "get", id: id}, stub, strings.NewReader("master\n"), &output); err != nil {
		t.Fatalf("executeVaultCommand(get) error = %v", err)
	}
	if !strings.Contains(output.String(), "old") {
		t.Fatalf("output = %q", output.String())
	}

	output.Reset()
	input := strings.NewReader("master\nnew title\nnew body\n.\nmetadata\n")
	if err := executeVaultCommand(vaultCommand{name: "update", id: id}, stub, input, &output); err != nil {
		t.Fatalf("executeVaultCommand(update) error = %v", err)
	}
	updated, ok := stub.updated.(clientmodel.Text)
	if !ok || updated.Title != "new title" || updated.Body != "new body" {
		t.Fatalf("updated payload = %#v", stub.updated)
	}

	output.Reset()
	if err := executeVaultCommand(vaultCommand{name: "delete", id: id}, stub, strings.NewReader(""), &output); err != nil {
		t.Fatalf("executeVaultCommand(delete) error = %v", err)
	}
	if stub.deletedID != id {
		t.Fatalf("deleted ID = %q", stub.deletedID)
	}
}

func TestExecuteVaultCommandEmptyListAndErrors(t *testing.T) {
	stub := &vaultServiceCLIStub{}
	var output bytes.Buffer
	if err := executeVaultCommand(vaultCommand{name: "list"}, stub, strings.NewReader("master\n"), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Vault is empty") {
		t.Fatalf("output = %q", output.String())
	}

	stub.err = errors.New("service failed")
	if err := executeVaultCommand(vaultCommand{name: "delete"}, stub, strings.NewReader(""), &bytes.Buffer{}); !errors.Is(err, stub.err) {
		t.Fatalf("error = %v", err)
	}
	if err := executeVaultCommand(vaultCommand{name: "unknown"}, stub, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("expected unsupported command error")
	}
}
