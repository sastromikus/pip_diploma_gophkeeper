package model

import (
	"strings"
	"testing"
)

func TestClientModelsValidate(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "metadata", run: func() error { return (Metadata{Text: "note"}).Validate() }},
		{name: "credentials", run: func() error {
			return (Credentials{Name: "Example", Login: "user", Password: "secret"}).Validate()
		}},
		{name: "text", run: func() error { return (Text{Title: "Note", Body: "body"}).Validate() }},
		{name: "binary", run: func() error {
			return (Binary{Filename: "file.bin", MIMEType: "application/octet-stream", Data: []byte{1}}).Validate(1024)
		}},
		{name: "bank card", run: func() error {
			return (BankCard{Name: "Main", Number: "4111111111111111", Holder: "USER", ExpiryDate: "12/30", CVV: "123"}).Validate()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestMetadataValidateRejectsOversizedValue(t *testing.T) {
	err := (Metadata{Text: strings.Repeat("x", MaxMetadataLength+1)}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestBankCardMaskedNumber(t *testing.T) {
	card := BankCard{Number: "4111 1111 1111 1234"}
	if got, want := card.MaskedNumber(), "**** 1234"; got != want {
		t.Fatalf("MaskedNumber() = %q, want %q", got, want)
	}
}
