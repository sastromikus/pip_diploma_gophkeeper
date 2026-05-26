package model

import (
	"errors"
	"testing"
	"time"
)

func TestRecordValidate(t *testing.T) {
	record := validRecord()
	limits := RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 1024}
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
	err := record.Validate(RecordLimits{MaxEncryptedPayloadSize: 2, MaxEncryptedMetadataSize: 1024})
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
		EncryptionVersion: 1,
		EncryptedPayload:  make([]byte, RecordAuthenticationTagSize),
		EncryptedMetadata: make([]byte, RecordAuthenticationTagSize),
		PayloadNonce:      make([]byte, RecordNonceSize),
		MetadataNonce:     make([]byte, RecordNonceSize),
		Version:           1,
		Revision:          1,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
}

func TestRecordValidateTombstone(t *testing.T) {
	record := validRecord()
	deletedAt := record.UpdatedAt.Add(time.Second)
	record.DeletedAt = &deletedAt
	record.EncryptedPayload = nil
	record.EncryptedMetadata = nil
	record.PayloadNonce = nil
	record.MetadataNonce = nil

	limits := RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 1024}
	if err := record.Validate(limits); err != nil {
		t.Fatalf("Validate() tombstone error = %v", err)
	}
}

func TestRecordValidateRejectsTombstoneWithPayload(t *testing.T) {
	record := validRecord()
	deletedAt := record.UpdatedAt.Add(time.Second)
	record.DeletedAt = &deletedAt

	limits := RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 1024}
	if err := record.Validate(limits); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInput", err)
	}
}

func TestRecordValidateRejectsUnsupportedEncryptionVersion(t *testing.T) {
	record := validRecord()
	record.EncryptionVersion = CurrentRecordEncryptionVersion + 1
	if err := record.Validate(RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 1024}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInput", err)
	}
}

func TestRecordValidateRejectsMalformedCryptographicFields(t *testing.T) {
	limits := RecordLimits{MaxEncryptedPayloadSize: 1024, MaxEncryptedMetadataSize: 1024}
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "payload nonce", mutate: func(record *Record) { record.PayloadNonce = make([]byte, RecordNonceSize-1) }},
		{name: "metadata nonce", mutate: func(record *Record) { record.MetadataNonce = make([]byte, RecordNonceSize-1) }},
		{name: "payload ciphertext", mutate: func(record *Record) { record.EncryptedPayload = make([]byte, RecordAuthenticationTagSize-1) }},
		{name: "metadata ciphertext", mutate: func(record *Record) { record.EncryptedMetadata = make([]byte, RecordAuthenticationTagSize-1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := validRecord()
			test.mutate(&record)
			if err := record.Validate(limits); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Validate() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}
