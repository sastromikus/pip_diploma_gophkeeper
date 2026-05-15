package model

import "fmt"

// RecordType identifies the logical kind of encrypted vault record.
type RecordType string

const (
	// RecordTypeCredentials stores a service login and password.
	RecordTypeCredentials RecordType = "credentials"
	// RecordTypeText stores an arbitrary text note.
	RecordTypeText RecordType = "text"
	// RecordTypeBinary stores an encrypted binary file.
	RecordTypeBinary RecordType = "binary"
	// RecordTypeBankCard stores bank card details.
	RecordTypeBankCard RecordType = "bank_card"
)

// Validate checks whether the record type is supported.
func (recordType RecordType) Validate() error {
	switch recordType {
	case RecordTypeCredentials, RecordTypeText, RecordTypeBinary, RecordTypeBankCard:
		return nil
	default:
		return fmt.Errorf("%w: unsupported record type %q", ErrInvalidInput, recordType)
	}
}
