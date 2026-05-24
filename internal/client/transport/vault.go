package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// RemoteRecord contains encrypted record data and server-managed metadata.
type RemoteRecord struct {
	ID        model.ID
	Data      clientcrypto.EncryptedRecordData
	Version   int64
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// RecordPage contains one page of active records.
type RecordPage struct {
	Records       []RemoteRecord
	NextPageToken string
	HasMore       bool
}

// CreateRecord stores a client-encrypted record.
func (client *Client) CreateRecord(ctx context.Context, token string, id model.ID, data clientcrypto.EncryptedRecordData) (RemoteRecord, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return RemoteRecord{}, err
	}
	if id.IsZero() {
		return RemoteRecord{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	request := gophkeeperv1.CreateRecordRequest_builder{Id: stringPointer(id.String()), Data: encryptedRecordMessage(data)}.Build()
	response, err := client.vault.CreateRecord(ctx, request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("create record: %w", mapRPCError(err))
	}
	return remoteRecord(response.GetRecord())
}

// GetRecord returns one encrypted record.
func (client *Client) GetRecord(ctx context.Context, token string, id model.ID) (RemoteRecord, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return RemoteRecord{}, err
	}
	if id.IsZero() {
		return RemoteRecord{}, fmt.Errorf("%w: record ID is required", model.ErrInvalidInput)
	}
	request := gophkeeperv1.GetRecordRequest_builder{Id: stringPointer(id.String())}.Build()
	response, err := client.vault.GetRecord(ctx, request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("get record: %w", mapRPCError(err))
	}
	return remoteRecord(response.GetRecord())
}

// ListRecords returns a page of active encrypted records.
func (client *Client) ListRecords(ctx context.Context, token, afterID string, limit uint32) (RecordPage, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return RecordPage{}, err
	}
	request := gophkeeperv1.ListRecordsRequest_builder{AfterId: stringPointer(afterID), Limit: &limit}.Build()
	response, err := client.vault.ListRecords(ctx, request)
	if err != nil {
		return RecordPage{}, fmt.Errorf("list records: %w", mapRPCError(err))
	}
	page := RecordPage{NextPageToken: response.GetNextPageToken(), HasMore: response.GetHasMore(), Records: make([]RemoteRecord, 0, len(response.GetRecords()))}
	for _, message := range response.GetRecords() {
		record, err := remoteRecord(message)
		if err != nil {
			return RecordPage{}, fmt.Errorf("decode listed record: %w", err)
		}
		page.Records = append(page.Records, record)
	}
	return page, nil
}

// UpdateRecord replaces encrypted record data using optimistic locking.
func (client *Client) UpdateRecord(ctx context.Context, token string, id model.ID, expectedVersion int64, data clientcrypto.EncryptedRecordData) (RemoteRecord, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return RemoteRecord{}, err
	}
	if id.IsZero() || expectedVersion < 1 {
		return RemoteRecord{}, fmt.Errorf("%w: record ID and positive expected version are required", model.ErrInvalidInput)
	}
	request := gophkeeperv1.UpdateRecordRequest_builder{Id: stringPointer(id.String()), ExpectedVersion: &expectedVersion, Data: encryptedRecordMessage(data)}.Build()
	response, err := client.vault.UpdateRecord(ctx, request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("update record: %w", mapRPCError(err))
	}
	return remoteRecord(response.GetRecord())
}

// DeleteRecord creates a tombstone using optimistic locking.
func (client *Client) DeleteRecord(ctx context.Context, token string, id model.ID, expectedVersion int64) (RemoteRecord, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return RemoteRecord{}, err
	}
	if id.IsZero() || expectedVersion < 1 {
		return RemoteRecord{}, fmt.Errorf("%w: record ID and positive expected version are required", model.ErrInvalidInput)
	}
	request := gophkeeperv1.DeleteRecordRequest_builder{Id: stringPointer(id.String()), ExpectedVersion: &expectedVersion}.Build()
	response, err := client.vault.DeleteRecord(ctx, request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("delete record: %w", mapRPCError(err))
	}
	return remoteRecord(response.GetRecord())
}

func authorizedContext(ctx context.Context, token string) (context.Context, error) {
	if ctx == nil {
		return nil, errors.New("request context is required")
	}
	if token == "" {
		return nil, errors.New("session token is required")
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), nil
}

func encryptedRecordMessage(data clientcrypto.EncryptedRecordData) *gophkeeperv1.EncryptedRecordData {
	messageType := protoRecordType(data.Type)
	return gophkeeperv1.EncryptedRecordData_builder{
		Type:              &messageType,
		EncryptionVersion: &data.EncryptionVersion,
		EncryptedPayload:  append([]byte(nil), data.EncryptedPayload...),
		EncryptedMetadata: append([]byte(nil), data.EncryptedMetadata...),
		PayloadNonce:      append([]byte(nil), data.PayloadNonce...),
		MetadataNonce:     append([]byte(nil), data.MetadataNonce...),
	}.Build()
}

func remoteRecord(message *gophkeeperv1.Record) (RemoteRecord, error) {
	if message == nil {
		return RemoteRecord{}, errors.New("server returned an empty record")
	}
	id, err := model.ParseID(message.GetId())
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("parse record ID: %w", err)
	}
	recordType, err := domainRecordType(message.GetType())
	if err != nil {
		return RemoteRecord{}, err
	}
	if message.GetVersion() < 1 || message.GetRevision() < 1 {
		return RemoteRecord{}, errors.New("server returned invalid record version metadata")
	}
	if message.GetEncryptionVersion() == 0 {
		return RemoteRecord{}, errors.New("server returned an unsupported encryption version")
	}
	if message.GetCreatedAt() == nil || message.GetUpdatedAt() == nil {
		return RemoteRecord{}, errors.New("server returned incomplete record timestamps")
	}
	if err := message.GetCreatedAt().CheckValid(); err != nil {
		return RemoteRecord{}, fmt.Errorf("invalid record creation time: %w", err)
	}
	if err := message.GetUpdatedAt().CheckValid(); err != nil {
		return RemoteRecord{}, fmt.Errorf("invalid record update time: %w", err)
	}
	if message.GetDeletedAt() != nil {
		if err := message.GetDeletedAt().CheckValid(); err != nil {
			return RemoteRecord{}, fmt.Errorf("invalid record deletion time: %w", err)
		}
		if len(message.GetEncryptedPayload()) != 0 || len(message.GetEncryptedMetadata()) != 0 || len(message.GetPayloadNonce()) != 0 || len(message.GetMetadataNonce()) != 0 {
			return RemoteRecord{}, errors.New("server returned a tombstone containing encrypted data")
		}
	} else if len(message.GetEncryptedPayload()) == 0 || len(message.GetEncryptedMetadata()) == 0 || len(message.GetPayloadNonce()) == 0 || len(message.GetMetadataNonce()) == 0 {
		return RemoteRecord{}, errors.New("server returned incomplete encrypted record data")
	}
	record := RemoteRecord{
		ID: id,
		Data: clientcrypto.EncryptedRecordData{
			Type: recordType, EncryptionVersion: message.GetEncryptionVersion(),
			EncryptedPayload:  append([]byte(nil), message.GetEncryptedPayload()...),
			EncryptedMetadata: append([]byte(nil), message.GetEncryptedMetadata()...),
			PayloadNonce:      append([]byte(nil), message.GetPayloadNonce()...),
			MetadataNonce:     append([]byte(nil), message.GetMetadataNonce()...),
		},
		Version: message.GetVersion(), Revision: message.GetRevision(),
	}
	if message.GetCreatedAt() != nil {
		record.CreatedAt = message.GetCreatedAt().AsTime().UTC()
	}
	if message.GetUpdatedAt() != nil {
		record.UpdatedAt = message.GetUpdatedAt().AsTime().UTC()
	}
	if message.GetDeletedAt() != nil {
		value := message.GetDeletedAt().AsTime().UTC()
		record.DeletedAt = &value
	}
	return record, nil
}

func protoRecordType(value model.RecordType) gophkeeperv1.RecordType {
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

func domainRecordType(value gophkeeperv1.RecordType) (model.RecordType, error) {
	switch value {
	case gophkeeperv1.RecordType_RECORD_TYPE_CREDENTIALS:
		return model.RecordTypeCredentials, nil
	case gophkeeperv1.RecordType_RECORD_TYPE_TEXT:
		return model.RecordTypeText, nil
	case gophkeeperv1.RecordType_RECORD_TYPE_BINARY:
		return model.RecordTypeBinary, nil
	case gophkeeperv1.RecordType_RECORD_TYPE_BANK_CARD:
		return model.RecordTypeBankCard, nil
	default:
		return "", fmt.Errorf("unsupported record type %d", value)
	}
}

func stringPointer(value string) *string { return &value }

// SyncPage contains one ordered page of server-side record changes.
type SyncPage struct {
	Records      []RemoteRecord
	NextRevision int64
	HasMore      bool
}

// SyncRecords returns ordered record changes after an exclusive server revision.
func (client *Client) SyncRecords(ctx context.Context, token string, afterRevision int64, limit uint32) (SyncPage, error) {
	ctx, err := authorizedContext(ctx, token)
	if err != nil {
		return SyncPage{}, err
	}
	if afterRevision < 0 {
		return SyncPage{}, fmt.Errorf("%w: sync revision cannot be negative", model.ErrInvalidInput)
	}
	request := gophkeeperv1.SyncRecordsRequest_builder{AfterRevision: &afterRevision, Limit: &limit}.Build()
	response, err := client.vault.SyncRecords(ctx, request)
	if err != nil {
		return SyncPage{}, fmt.Errorf("sync records: %w", mapRPCError(err))
	}
	if response.GetNextRevision() < afterRevision {
		return SyncPage{}, errors.New("server returned a sync cursor behind the requested revision")
	}
	page := SyncPage{NextRevision: response.GetNextRevision(), HasMore: response.GetHasMore(), Records: make([]RemoteRecord, 0, len(response.GetRecords()))}
	for _, message := range response.GetRecords() {
		record, err := remoteRecord(message)
		if err != nil {
			return SyncPage{}, fmt.Errorf("decode synchronized record: %w", err)
		}
		if record.Revision <= afterRevision {
			return SyncPage{}, errors.New("server returned a record outside the requested revision range")
		}
		page.Records = append(page.Records, record)
	}
	if page.HasMore && (len(page.Records) == 0 || page.NextRevision == afterRevision) {
		return SyncPage{}, errors.New("server returned a non-progressing sync page")
	}
	return page, nil
}

func mapRPCError(err error) error {
	switch status.Code(err) {
	case codes.InvalidArgument:
		return fmt.Errorf("%w: %v", model.ErrInvalidInput, err)
	case codes.NotFound:
		return fmt.Errorf("%w: %v", model.ErrNotFound, err)
	case codes.AlreadyExists:
		return fmt.Errorf("%w: %v", model.ErrAlreadyExists, err)
	case codes.Aborted:
		return fmt.Errorf("%w: %v", model.ErrVersionConflict, err)
	case codes.Unauthenticated:
		return fmt.Errorf("%w: %v", model.ErrUnauthenticated, err)
	case codes.PermissionDenied:
		return fmt.Errorf("%w: %v", model.ErrForbidden, err)
	case codes.ResourceExhausted:
		return fmt.Errorf("%w: %v", model.ErrPayloadTooLarge, err)
	default:
		return err
	}
}
