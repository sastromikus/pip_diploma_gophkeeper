package service

import (
	"context"
	"fmt"

	"github.com/sastromikus/gophkeeper/internal/model"
)

const (
	DefaultListLimit = int32(100)
	MaxListLimit     = int32(500)
	DefaultSyncLimit = int32(100)
	MaxSyncLimit     = int32(500)
)

// RecordRepository persists encrypted records and synchronization tombstones.
type RecordRepository interface {
	Create(context.Context, model.Record) (model.Record, error)
	Get(context.Context, model.ID, model.ID) (model.Record, error)
	List(context.Context, model.ID, model.ID, int32) ([]model.Record, error)
	Update(context.Context, model.Record, int64) (model.Record, error)
	Delete(context.Context, model.ID, model.ID, int64) (model.Record, error)
	ListChangedAfter(context.Context, model.ID, int64, int32) ([]model.Record, error)
}

// EncryptedRecordInput contains client-produced encrypted record fields.
type EncryptedRecordInput struct {
	Type              model.RecordType
	EncryptionVersion uint32
	EncryptedPayload  []byte
	EncryptedMetadata []byte
	PayloadNonce      []byte
	MetadataNonce     []byte
}

// CreateRecordInput describes a record creation request.
type CreateRecordInput struct {
	UserID model.ID
	ID     model.ID
	Data   EncryptedRecordInput
}

// UpdateRecordInput describes an optimistic record update.
type UpdateRecordInput struct {
	UserID          model.ID
	ID              model.ID
	ExpectedVersion int64
	Data            EncryptedRecordInput
}

// ListRecordsInput describes an active-record page request.
type ListRecordsInput struct {
	UserID  model.ID
	AfterID model.ID
	Limit   int32
}

// ListRecordsResult contains one active-record page and its next cursor.
type ListRecordsResult struct {
	Records []model.Record
	NextID  model.ID
	HasMore bool
}

// SyncRecordsInput describes an incremental synchronization request.
type SyncRecordsInput struct {
	UserID        model.ID
	AfterRevision int64
	Limit         int32
}

// SyncRecordsResult contains ordered changes and the next durable cursor.
type SyncRecordsResult struct {
	Records      []model.Record
	NextRevision int64
	HasMore      bool
}

// VaultService manages encrypted records owned by authenticated users.
type VaultService struct {
	repository RecordRepository
	limits     model.RecordLimits
}

// NewVaultService creates a vault service.
func NewVaultService(repository RecordRepository, limits model.RecordLimits) (*VaultService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: record repository is required", model.ErrInvalidInput)
	}
	if limits.MaxEncryptedPayloadSize <= 0 || limits.MaxEncryptedMetadataSize <= 0 {
		return nil, fmt.Errorf("%w: encrypted record limits must be positive", model.ErrInvalidInput)
	}
	return &VaultService{repository: repository, limits: limits}, nil
}

// Create stores a new encrypted record.
func (service *VaultService) Create(ctx context.Context, input CreateRecordInput) (model.Record, error) {
	record := model.Record{
		ID: input.ID, UserID: input.UserID, Type: input.Data.Type,
		EncryptionVersion: input.Data.EncryptionVersion,
		EncryptedPayload:  cloneBytes(input.Data.EncryptedPayload), EncryptedMetadata: cloneBytes(input.Data.EncryptedMetadata),
		PayloadNonce: cloneBytes(input.Data.PayloadNonce), MetadataNonce: cloneBytes(input.Data.MetadataNonce),
	}
	if err := validateEncryptedRecordInput(record, service.limits); err != nil {
		return model.Record{}, err
	}
	created, err := service.repository.Create(ctx, record)
	if err != nil {
		return model.Record{}, fmt.Errorf("create record: %w", err)
	}
	return created, nil
}

// Get returns one active record owned by the user.
func (service *VaultService) Get(ctx context.Context, userID, recordID model.ID) (model.Record, error) {
	if userID.IsZero() || recordID.IsZero() {
		return model.Record{}, fmt.Errorf("%w: user and record IDs are required", model.ErrInvalidInput)
	}
	record, err := service.repository.Get(ctx, userID, recordID)
	if err != nil {
		return model.Record{}, fmt.Errorf("get record: %w", err)
	}
	return record, nil
}

// List returns a bounded page of active records.
func (service *VaultService) List(ctx context.Context, input ListRecordsInput) (ListRecordsResult, error) {
	if input.UserID.IsZero() {
		return ListRecordsResult{}, fmt.Errorf("%w: user ID is required", model.ErrInvalidInput)
	}
	limit, err := normalizeLimit(input.Limit, DefaultListLimit, MaxListLimit)
	if err != nil {
		return ListRecordsResult{}, err
	}
	records, err := service.repository.List(ctx, input.UserID, input.AfterID, limit+1)
	if err != nil {
		return ListRecordsResult{}, fmt.Errorf("list records: %w", err)
	}
	result := ListRecordsResult{Records: records}
	if len(result.Records) > int(limit) {
		result.Records = result.Records[:limit]
		result.HasMore = true
	}
	if len(result.Records) > 0 {
		result.NextID = result.Records[len(result.Records)-1].ID
	}
	return result, nil
}

// Update replaces encrypted fields if the expected version matches.
func (service *VaultService) Update(ctx context.Context, input UpdateRecordInput) (model.Record, error) {
	if input.ExpectedVersion < 1 {
		return model.Record{}, fmt.Errorf("%w: expected version must be positive", model.ErrInvalidInput)
	}
	record := model.Record{
		ID: input.ID, UserID: input.UserID, Type: input.Data.Type,
		EncryptionVersion: input.Data.EncryptionVersion,
		EncryptedPayload:  cloneBytes(input.Data.EncryptedPayload), EncryptedMetadata: cloneBytes(input.Data.EncryptedMetadata),
		PayloadNonce: cloneBytes(input.Data.PayloadNonce), MetadataNonce: cloneBytes(input.Data.MetadataNonce),
	}
	if err := validateEncryptedRecordInput(record, service.limits); err != nil {
		return model.Record{}, err
	}
	updated, err := service.repository.Update(ctx, record, input.ExpectedVersion)
	if err != nil {
		return model.Record{}, fmt.Errorf("update record: %w", err)
	}
	return updated, nil
}

// Delete creates a minimal synchronization tombstone.
func (service *VaultService) Delete(ctx context.Context, userID, recordID model.ID, expectedVersion int64) (model.Record, error) {
	if userID.IsZero() || recordID.IsZero() {
		return model.Record{}, fmt.Errorf("%w: user and record IDs are required", model.ErrInvalidInput)
	}
	if expectedVersion < 1 {
		return model.Record{}, fmt.Errorf("%w: expected version must be positive", model.ErrInvalidInput)
	}
	record, err := service.repository.Delete(ctx, userID, recordID, expectedVersion)
	if err != nil {
		return model.Record{}, fmt.Errorf("delete record: %w", err)
	}
	return record, nil
}

// Sync returns ordered changes after an exclusive server revision cursor.
func (service *VaultService) Sync(ctx context.Context, input SyncRecordsInput) (SyncRecordsResult, error) {
	if input.UserID.IsZero() {
		return SyncRecordsResult{}, fmt.Errorf("%w: user ID is required", model.ErrInvalidInput)
	}
	if input.AfterRevision < 0 {
		return SyncRecordsResult{}, fmt.Errorf("%w: revision cursor must not be negative", model.ErrInvalidInput)
	}
	limit, err := normalizeLimit(input.Limit, DefaultSyncLimit, MaxSyncLimit)
	if err != nil {
		return SyncRecordsResult{}, err
	}
	records, err := service.repository.ListChangedAfter(ctx, input.UserID, input.AfterRevision, limit+1)
	if err != nil {
		return SyncRecordsResult{}, fmt.Errorf("sync records: %w", err)
	}
	result := SyncRecordsResult{Records: records, NextRevision: input.AfterRevision}
	if len(result.Records) > int(limit) {
		result.Records = result.Records[:limit]
		result.HasMore = true
	}
	if len(result.Records) > 0 {
		result.NextRevision = result.Records[len(result.Records)-1].Revision
	}
	return result, nil
}

func validateEncryptedRecordInput(record model.Record, limits model.RecordLimits) error {
	if record.ID.IsZero() || record.UserID.IsZero() {
		return fmt.Errorf("%w: user and record IDs are required", model.ErrInvalidInput)
	}
	if err := record.Type.Validate(); err != nil {
		return err
	}
	if record.EncryptionVersion == 0 {
		return fmt.Errorf("%w: encryption version must be positive", model.ErrInvalidInput)
	}
	if len(record.EncryptedPayload) == 0 || len(record.EncryptedMetadata) == 0 || len(record.PayloadNonce) == 0 || len(record.MetadataNonce) == 0 {
		return fmt.Errorf("%w: encrypted data and nonces are required", model.ErrInvalidInput)
	}
	if int64(len(record.EncryptedPayload)) > limits.MaxEncryptedPayloadSize || int64(len(record.EncryptedMetadata)) > limits.MaxEncryptedMetadataSize {
		return model.ErrPayloadTooLarge
	}
	return nil
}

func normalizeLimit(value, defaultValue, maxValue int32) (int32, error) {
	if value == 0 {
		return defaultValue, nil
	}
	if value < 0 || value > maxValue {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", model.ErrInvalidInput, maxValue)
	}
	return value, nil
}
