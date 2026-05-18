package grpcserver

import (
	"context"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	"github.com/sastromikus/gophkeeper/internal/model"
	"github.com/sastromikus/gophkeeper/internal/server/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VaultApplication exposes encrypted-record operations to the gRPC transport.
type VaultApplication interface {
	Create(context.Context, service.CreateRecordInput) (model.Record, error)
	Get(context.Context, model.ID, model.ID) (model.Record, error)
	List(context.Context, service.ListRecordsInput) (service.ListRecordsResult, error)
	Update(context.Context, service.UpdateRecordInput) (model.Record, error)
	Delete(context.Context, model.ID, model.ID, int64) (model.Record, error)
	Sync(context.Context, service.SyncRecordsInput) (service.SyncRecordsResult, error)
}

// VaultServer implements the VaultService gRPC contract.
type VaultServer struct {
	gophkeeperv1.UnimplementedVaultServiceServer
	vault VaultApplication
}

// NewVaultServer creates a vault gRPC server.
func NewVaultServer(vault VaultApplication) *VaultServer {
	return &VaultServer{vault: vault}
}

// CreateRecord stores a client-encrypted record for the authenticated user.
func (server *VaultServer) CreateRecord(ctx context.Context, request *gophkeeperv1.CreateRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil || request.GetData() == nil {
		return nil, status.Error(codes.InvalidArgument, "request data is required")
	}
	id, err := model.ParseID(request.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	record, err := server.vault.Create(ctx, service.CreateRecordInput{UserID: principal.UserID, ID: id, Data: encryptedInput(request.GetData())})
	if err != nil {
		return nil, mapError(err)
	}
	return gophkeeperv1.RecordResponse_builder{Record: recordMessage(record)}.Build(), nil
}

// GetRecord returns one active record owned by the authenticated user.
func (server *VaultServer) GetRecord(ctx context.Context, request *gophkeeperv1.GetRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	id, err := model.ParseID(request.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	record, err := server.vault.Get(ctx, principal.UserID, id)
	if err != nil {
		return nil, mapError(err)
	}
	return gophkeeperv1.RecordResponse_builder{Record: recordMessage(record)}.Build(), nil
}

// ListRecords returns a bounded page of active records.
func (server *VaultServer) ListRecords(ctx context.Context, request *gophkeeperv1.ListRecordsRequest) (*gophkeeperv1.ListRecordsResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	var afterID model.ID
	if request.GetAfterId() != "" {
		afterID, err = model.ParseID(request.GetAfterId())
		if err != nil {
			return nil, mapError(err)
		}
	}
	result, err := server.vault.List(ctx, service.ListRecordsInput{UserID: principal.UserID, AfterID: afterID, Limit: int32(request.GetLimit())})
	if err != nil {
		return nil, mapError(err)
	}
	messages := make([]*gophkeeperv1.Record, 0, len(result.Records))
	for _, record := range result.Records {
		messages = append(messages, recordMessage(record))
	}
	next := ""
	if !result.NextID.IsZero() {
		next = result.NextID.String()
	}
	return gophkeeperv1.ListRecordsResponse_builder{Records: messages, NextPageToken: &next, HasMore: &result.HasMore}.Build(), nil
}

// UpdateRecord replaces encrypted fields using optimistic locking.
func (server *VaultServer) UpdateRecord(ctx context.Context, request *gophkeeperv1.UpdateRecordRequest) (*gophkeeperv1.RecordResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil || request.GetData() == nil {
		return nil, status.Error(codes.InvalidArgument, "request data is required")
	}
	id, err := model.ParseID(request.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	record, err := server.vault.Update(ctx, service.UpdateRecordInput{UserID: principal.UserID, ID: id, ExpectedVersion: request.GetExpectedVersion(), Data: encryptedInput(request.GetData())})
	if err != nil {
		return nil, mapError(err)
	}
	return gophkeeperv1.RecordResponse_builder{Record: recordMessage(record)}.Build(), nil
}

// DeleteRecord creates a minimal tombstone using optimistic locking.
func (server *VaultServer) DeleteRecord(ctx context.Context, request *gophkeeperv1.DeleteRecordRequest) (*gophkeeperv1.DeleteRecordResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	id, err := model.ParseID(request.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	record, err := server.vault.Delete(ctx, principal.UserID, id, request.GetExpectedVersion())
	if err != nil {
		return nil, mapError(err)
	}
	return gophkeeperv1.DeleteRecordResponse_builder{Record: recordMessage(record)}.Build(), nil
}

// SyncRecords returns ordered changes after an exclusive revision cursor.
func (server *VaultServer) SyncRecords(ctx context.Context, request *gophkeeperv1.SyncRecordsRequest) (*gophkeeperv1.SyncRecordsResponse, error) {
	principal, err := requirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if server.vault == nil {
		return nil, status.Error(codes.Internal, "vault service is not configured")
	}
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	result, err := server.vault.Sync(ctx, service.SyncRecordsInput{UserID: principal.UserID, AfterRevision: request.GetAfterRevision(), Limit: int32(request.GetLimit())})
	if err != nil {
		return nil, mapError(err)
	}
	messages := make([]*gophkeeperv1.Record, 0, len(result.Records))
	for _, record := range result.Records {
		messages = append(messages, recordMessage(record))
	}
	return gophkeeperv1.SyncRecordsResponse_builder{Records: messages, NextRevision: &result.NextRevision, HasMore: &result.HasMore}.Build(), nil
}

func requirePrincipal(ctx context.Context) (Principal, error) {
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.UserID.IsZero() {
		return Principal{}, status.Error(codes.Unauthenticated, "authentication required")
	}
	return principal, nil
}

func encryptedInput(data *gophkeeperv1.EncryptedRecordData) service.EncryptedRecordInput {
	return service.EncryptedRecordInput{
		Type: recordTypeFromProto(data.GetType()), EncryptionVersion: data.GetEncryptionVersion(),
		EncryptedPayload: data.GetEncryptedPayload(), EncryptedMetadata: data.GetEncryptedMetadata(),
		PayloadNonce: data.GetPayloadNonce(), MetadataNonce: data.GetMetadataNonce(),
	}
}

func recordMessage(record model.Record) *gophkeeperv1.Record {
	id := record.ID.String()
	recordType := recordTypeToProto(record.Type)
	encryptionVersion := record.EncryptionVersion
	version := record.Version
	revision := record.Revision
	builder := gophkeeperv1.Record_builder{
		Id: &id, Type: &recordType, EncryptionVersion: &encryptionVersion,
		EncryptedPayload: record.EncryptedPayload, EncryptedMetadata: record.EncryptedMetadata,
		PayloadNonce: record.PayloadNonce, MetadataNonce: record.MetadataNonce,
		Version: &version, Revision: &revision,
		CreatedAt: timestamppb.New(record.CreatedAt), UpdatedAt: timestamppb.New(record.UpdatedAt),
	}
	if record.DeletedAt != nil {
		builder.DeletedAt = timestamppb.New(*record.DeletedAt)
	}
	return builder.Build()
}

func recordTypeFromProto(value gophkeeperv1.RecordType) model.RecordType {
	switch value {
	case gophkeeperv1.RecordType_RECORD_TYPE_CREDENTIALS:
		return model.RecordTypeCredentials
	case gophkeeperv1.RecordType_RECORD_TYPE_TEXT:
		return model.RecordTypeText
	case gophkeeperv1.RecordType_RECORD_TYPE_BINARY:
		return model.RecordTypeBinary
	case gophkeeperv1.RecordType_RECORD_TYPE_BANK_CARD:
		return model.RecordTypeBankCard
	default:
		return model.RecordType("")
	}
}

func recordTypeToProto(value model.RecordType) gophkeeperv1.RecordType {
	switch value {
	case model.RecordTypeCredentials:
		return gophkeeperv1.RecordType_RECORD_TYPE_CREDENTIALS
	case model.RecordTypeText:
		return gophkeeperv1.RecordType_RECORD_TYPE_TEXT
	case model.RecordTypeBinary:
		return gophkeeperv1.RecordType_RECORD_TYPE_BINARY
	case model.RecordTypeBankCard:
		return gophkeeperv1.RecordType_RECORD_TYPE_BANK_CARD
	default:
		return gophkeeperv1.RecordType_RECORD_TYPE_UNSPECIFIED
	}
}
