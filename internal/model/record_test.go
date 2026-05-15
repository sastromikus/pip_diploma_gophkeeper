package model

import (
	"errors"
	"testing"
	"time"
)

func TestRecordValidate(t *testing.T) {
	record := validRecord()
	limits := RecordLimits{MaxPayloadSize: 1024, MaxMetadataSize: 1024}
	if err := record.Validate(limits); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if record.Deleted() {
		t.Fatal("Deleted() = true, want false")
	}
}

func TestRecordValidateRejectsOversizedPayload(t *testing.T) {
	record := validRecord()
	record.EncryptedPayload = []byte("too large")
	err := record.Validate(RecordLimits{MaxPayloadSize: 2, MaxMetadataSize: 1024})
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("Validate() error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestRecordDeleted(t *testing.T) {
	record := validRecord()
	deletedAt := record.UpdatedAt
	record.DeletedAt = &deletedAt
	if !record.Deleted() {
		t.Fatal("Deleted() = false, want true")
	}
}

func validRecord() Record {
	createdAt := time.Unix(100, 0).UTC()
	return Record{
		ID:                ID("550e8400-e29b-41d4-a716-446655440000"),
		UserID:            ID("550e8400-e29b-41d4-a716-446655440001"),
		Type:              RecordTypeCredentials,
		EncryptedPayload:  []byte("payload"),
		EncryptedMetadata: []byte("metadata"),
		PayloadNonce:      []byte("payload-nonce"),
		MetadataNonce:     []byte("metadata-nonce"),
		Version:           1,
		Revision:          1,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
}
