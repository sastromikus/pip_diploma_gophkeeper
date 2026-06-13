package model

import (
	"fmt"
	"time"
)

// Record contains encrypted user data and synchronization metadata.
type Record struct {
	ID                ID
	UserID            ID
	Type              RecordType
	EncryptionVersion uint32
	EncryptedPayload  []byte
	EncryptedMetadata []byte
	PayloadNonce      []byte
	MetadataNonce     []byte
	Version           int64
	Revision          int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

// RecordLimits defines server-enforced encrypted record size limits.
type RecordLimits struct {
	MaxEncryptedPayloadSize  int64
	MaxEncryptedMetadataSize int64
}

// Validate checks record invariants and encrypted data sizes.
func (record Record) Validate(limits RecordLimits) error {
	if record.ID.IsZero() {
		return fmt.Errorf("%w: record ID is required", ErrInvalidInput)
	}
	if record.UserID.IsZero() {
		return fmt.Errorf("%w: record user ID is required", ErrInvalidInput)
	}
	if err := record.Type.Validate(); err != nil {
		return err
	}
	if record.EncryptionVersion != CurrentRecordEncryptionVersion {
		return fmt.Errorf("%w: unsupported encryption version %d", ErrInvalidInput, record.EncryptionVersion)
	}
	if limits.MaxEncryptedPayloadSize <= 0 || limits.MaxEncryptedMetadataSize <= 0 {
		return fmt.Errorf("%w: encrypted record limits must be positive", ErrInvalidInput)
	}

	if record.Deleted() {
		if len(record.EncryptedPayload) != 0 || len(record.EncryptedMetadata) != 0 ||
			len(record.PayloadNonce) != 0 || len(record.MetadataNonce) != 0 {
			return fmt.Errorf("%w: tombstone must not contain encrypted data", ErrInvalidInput)
		}
	} else {
		if len(record.EncryptedPayload) == 0 {
			return fmt.Errorf("%w: encrypted payload is required", ErrInvalidInput)
		}
		if len(record.EncryptedMetadata) == 0 {
			return fmt.Errorf("%w: encrypted metadata is required", ErrInvalidInput)
		}

		// Check size limits before validating the encrypted format so oversized
		// input is rejected consistently as ErrPayloadTooLarge.
		if int64(len(record.EncryptedPayload)) > limits.MaxEncryptedPayloadSize {
			return fmt.Errorf("%w: encrypted payload exceeds %d bytes", ErrPayloadTooLarge, limits.MaxEncryptedPayloadSize)
		}
		if int64(len(record.EncryptedMetadata)) > limits.MaxEncryptedMetadataSize {
			return fmt.Errorf("%w: encrypted metadata exceeds %d bytes", ErrPayloadTooLarge, limits.MaxEncryptedMetadataSize)
		}

		if len(record.PayloadNonce) != RecordNonceSize {
			return fmt.Errorf("%w: payload nonce must be %d bytes", ErrInvalidInput, RecordNonceSize)
		}
		if len(record.MetadataNonce) != RecordNonceSize {
			return fmt.Errorf("%w: metadata nonce must be %d bytes", ErrInvalidInput, RecordNonceSize)
		}
		if len(record.EncryptedPayload) < RecordAuthenticationTagSize {
			return fmt.Errorf("%w: encrypted payload is malformed", ErrInvalidInput)
		}
		if len(record.EncryptedMetadata) < RecordAuthenticationTagSize {
			return fmt.Errorf("%w: encrypted metadata is malformed", ErrInvalidInput)
		}
	}

	if record.Version < 1 {
		return fmt.Errorf("%w: record version must be positive", ErrInvalidInput)
	}
	if record.Revision < 1 {
		return fmt.Errorf("%w: record revision must be positive", ErrInvalidInput)
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: record timestamps are required", ErrInvalidInput)
	}
	if record.UpdatedAt.Before(record.CreatedAt) {
		return fmt.Errorf("%w: record update precedes creation", ErrInvalidInput)
	}
	if record.DeletedAt != nil && record.DeletedAt.Before(record.UpdatedAt) {
		return fmt.Errorf("%w: record deletion precedes last update", ErrInvalidInput)
	}
	return nil
}

// Deleted reports whether the record is represented by a tombstone.
func (record Record) Deleted() bool {
	return record.DeletedAt != nil
}
