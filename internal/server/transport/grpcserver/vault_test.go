package grpcserver

import (
	"context"
	"testing"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type vaultApplicationStub struct {
	record model.Record
	list   service.ListRecordsResult
	sync   service.SyncRecordsResult
	err    error
}

func (stub vaultApplicationStub) Create(context.Context, service.CreateRecordInput) (model.Record, error) {
	return stub.record, stub.err
}
func (stub vaultApplicationStub) Get(context.Context, model.ID, model.ID) (model.Record, error) {
	return stub.record, stub.err
}
func (stub vaultApplicationStub) List(context.Context, service.ListRecordsInput) (service.ListRecordsResult, error) {
	return stub.list, stub.err
}
func (stub vaultApplicationStub) Update(context.Context, service.UpdateRecordInput) (model.Record, error) {
	return stub.record, stub.err
}
func (stub vaultApplicationStub) Delete(context.Context, model.ID, model.ID, int64) (model.Record, error) {
	return stub.record, stub.err
}
func (stub vaultApplicationStub) Sync(context.Context, service.SyncRecordsInput) (service.SyncRecordsResult, error) {
	return stub.sync, stub.err
}

func vaultPrincipalContext() context.Context {
	return context.WithValue(context.Background(), principalContextKey{}, Principal{UserID: model.ID("11111111-1111-4111-8111-111111111111")})
}

func vaultRecord() model.Record {
	now := time.Now().UTC()
	return model.Record{
		ID:     model.ID("123e4567-e89b-42d3-a456-426614174000"),
		UserID: model.ID("11111111-1111-4111-8111-111111111111"),
		Type:   model.RecordTypeText, EncryptionVersion: 1,
		EncryptedPayload: []byte{1}, EncryptedMetadata: []byte{2}, PayloadNonce: []byte{3}, MetadataNonce: []byte{4},
		Version: 1, Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
}

func encryptedProtoData() *gophkeeperv1.EncryptedRecordData {
	typ := gophkeeperv1.RecordType_RECORD_TYPE_TEXT
	version := uint32(1)
	return gophkeeperv1.EncryptedRecordData_builder{
		Type: &typ, EncryptionVersion: &version,
		EncryptedPayload: []byte{1}, EncryptedMetadata: []byte{2}, PayloadNonce: []byte{3}, MetadataNonce: []byte{4},
	}.Build()
}

func TestVaultServerCRUD(t *testing.T) {
	record := vaultRecord()
	server := NewVaultServer(vaultApplicationStub{record: record, list: service.ListRecordsResult{Records: []model.Record{record}, NextID: record.ID}, sync: service.SyncRecordsResult{Records: []model.Record{record}, NextRevision: 1}})
	ctx := vaultPrincipalContext()
	id := record.ID.String()

	create, err := server.CreateRecord(ctx, gophkeeperv1.CreateRecordRequest_builder{Id: &id, Data: encryptedProtoData()}.Build())
	if err != nil || create.GetRecord().GetId() != id {
		t.Fatalf("CreateRecord() = %#v, %v", create, err)
	}
	if _, err := server.GetRecord(ctx, gophkeeperv1.GetRecordRequest_builder{Id: &id}.Build()); err != nil {
		t.Fatal(err)
	}
	limit := uint32(10)
	if _, err := server.ListRecords(ctx, gophkeeperv1.ListRecordsRequest_builder{Limit: &limit}.Build()); err != nil {
		t.Fatal(err)
	}
	expected := int64(1)
	if _, err := server.UpdateRecord(ctx, gophkeeperv1.UpdateRecordRequest_builder{Id: &id, ExpectedVersion: &expected, Data: encryptedProtoData()}.Build()); err != nil {
		t.Fatal(err)
	}
	if _, err := server.DeleteRecord(ctx, gophkeeperv1.DeleteRecordRequest_builder{Id: &id, ExpectedVersion: &expected}.Build()); err != nil {
		t.Fatal(err)
	}
	after := int64(0)
	if _, err := server.SyncRecords(ctx, gophkeeperv1.SyncRecordsRequest_builder{AfterRevision: &after, Limit: &limit}.Build()); err != nil {
		t.Fatal(err)
	}
}

func TestVaultServerValidation(t *testing.T) {
	server := NewVaultServer(vaultApplicationStub{})
	if _, err := server.GetRecord(context.Background(), nil); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetRecord() code = %v", status.Code(err))
	}
	if _, err := server.CreateRecord(vaultPrincipalContext(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateRecord() code = %v", status.Code(err))
	}
	badID := "invalid"
	if _, err := server.GetRecord(vaultPrincipalContext(), gophkeeperv1.GetRecordRequest_builder{Id: &badID}.Build()); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("GetRecord() invalid ID code = %v", status.Code(err))
	}
}
