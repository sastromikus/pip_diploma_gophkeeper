package model

import (
	"errors"
	"testing"
)

func TestRecordTypeValidate(t *testing.T) {
	for _, recordType := range []RecordType{
		RecordTypeCredentials,
		RecordTypeText,
		RecordTypeBinary,
		RecordTypeBankCard,
	} {
		if err := recordType.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", recordType, err)
		}
	}
}

func TestRecordTypeValidateRejectsUnknownType(t *testing.T) {
	err := RecordType("unknown").Validate()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Validate() error = %v, want ErrInvalidInput", err)
	}
}
