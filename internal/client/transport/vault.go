package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	gophkeeperv1 "github.com/sastromikus/gophkeeper/api/gophkeeper/v1"
	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	"github.com/sastromikus/gophkeeper/internal/model"
	"google.golang.org/grpc/metadata"
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
	request := gophkeeperv1.CreateRecordRequest_builder{Id: stringPointer(id.String()), Data: encryptedRecordMessage(data)}.Build()
	response, err := client.vault.CreateRecord(authorize(ctx, token), request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("create record: %w", err)
	}
	return remoteRecord(response.GetRecord())
}

// GetRecord returns one encrypted record.
func (client *Client) GetRecord(ctx context.Context, token string, id model.ID) (RemoteRecord, error) {
	request := gophkeeperv1.GetRecordRequest_builder{Id: stringPointer(id.String())}.Build()
	response, err := client.vault.GetRecord(authorize(ctx, token), request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("get record: %w", err)
	}
	return remoteRecord(response.GetRecord())
}

// ListRecords returns a page of active encrypted records.
func (client *Client) ListRecords(ctx context.Context, token, afterID string, limit uint32) (RecordPage, error) {
	request := gophkeeperv1.ListRecordsRequest_builder{AfterId: stringPointer(afterID), Limit: &limit}.Build()
	response, err := client.vault.ListRecords(authorize(ctx, token), request)
	if err != nil {
		return RecordPage{}, fmt.Errorf("list records: %w", err)
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
	request := gophkeeperv1.UpdateRecordRequest_builder{Id: stringPointer(id.String()), ExpectedVersion: &expectedVersion, Data: encryptedRecordMessage(data)}.Build()
	response, err := client.vault.UpdateRecord(authorize(ctx, token), request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("update record: %w", err)
	}
	return remoteRecord(response.GetRecord())
}

// DeleteRecord creates a tombstone using optimistic locking.
func (client *Client) DeleteRecord(ctx context.Context, token string, id model.ID, expectedVersion int64) (RemoteRecord, error) {
	request := gophkeeperv1.DeleteRecordRequest_builder{Id: stringPointer(id.String()), ExpectedVersion: &expectedVersion}.Build()
	response, err := client.vault.DeleteRecord(authorize(ctx, token), request)
	if err != nil {
		return RemoteRecord{}, fmt.Errorf("delete record: %w", err)
	}
	return remoteRecord(response.GetRecord())
}

func authorize(ctx context.Context, token string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
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
