package model

import (
	"errors"
	"testing"
)

func TestParseID(t *testing.T) {
	id, err := ParseID("550E8400-E29B-41D4-A716-446655440000")
	if err != nil {
		t.Fatalf("ParseID() error = %v", err)
	}
	if got, want := id.String(), "550e8400-e29b-41d4-a716-446655440000"; got != want {
		t.Fatalf("ID = %q, want %q", got, want)
	}
}

func TestParseIDRejectsInvalidValue(t *testing.T) {
	_, err := ParseID("not-a-uuid")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ParseID() error = %v, want ErrInvalidInput", err)
	}
}
