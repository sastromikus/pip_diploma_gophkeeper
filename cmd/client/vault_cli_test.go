package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientapp "github.com/sastromikus/gophkeeper/internal/client/app"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const testRecordID = "123e4567-e89b-42d3-a456-426614174000"

func TestParseVaultCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "add", args: []string{"add", "credentials", "-insecure"}, want: "add"},
		{name: "list", args: []string{"list", "-insecure"}, want: "list"},
		{name: "get binary output", args: []string{"get", testRecordID, "backup.bin", "-insecure"}, want: "get"},
		{name: "update", args: []string{"update", testRecordID}, want: "update"},
		{name: "delete", args: []string{"delete", testRecordID}, want: "delete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := parseVaultCommand(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if command.name != tt.want {
				t.Fatalf("name = %q, want %q", command.name, tt.want)
			}
		})
	}
}

func TestParseVaultCommandRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"add"}, {"add", "unknown"}, {"get"}, {"get", "invalid"}, {"unknown"}} {
		if _, err := parseVaultCommand(args); err == nil {
			t.Fatalf("parseVaultCommand(%q) expected error", args)
		}
	}
}

func TestReadRecordInputCredentials(t *testing.T) {
	input := strings.NewReader("mail\nalice@example.com\nsecret\nwork account\n")
	reader := bufioForTest(input)
	payload, metadata, err := readRecordInput(input, reader, &bytes.Buffer{}, model.RecordTypeCredentials)
	if err != nil {
		t.Fatal(err)
	}
	credentials, ok := payload.(clientmodel.Credentials)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}
	if credentials.Name != "mail" || credentials.Login != "alice@example.com" || credentials.Password != "secret" {
		t.Fatalf("credentials = %#v", credentials)
	}
	if metadata.Text != "work account" {
		t.Fatalf("metadata = %q", metadata.Text)
	}
}

func TestWriteRecordBinary(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "output.bin")
	id, err := model.ParseID(testRecordID)
	if err != nil {
		t.Fatal(err)
	}
	view := clientapp.RecordView{ID: id, Type: model.RecordTypeBinary, Version: 1, Payload: clientmodel.Binary{Filename: "input.bin", MIMEType: "application/octet-stream", Data: []byte("data")}}
	var output bytes.Buffer
	if err := writeRecord(&output, view, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Fatalf("file contents = %q", data)
	}
	if err := writeRecord(&output, view, path); err == nil {
		t.Fatal("expected refusal to overwrite existing file")
	}
}

func bufioForTest(input *strings.Reader) *bufio.Reader { return bufio.NewReader(input) }

func TestParseRecordTypeAliases(t *testing.T) {
	for _, value := range []string{"card", "bank-card", "bank_card"} {
		recordType, err := parseRecordType(value)
		if err != nil {
			t.Fatal(err)
		}
		if recordType != model.RecordTypeBankCard {
			t.Fatalf("parseRecordType(%q) = %q", value, recordType)
		}
	}
}

func TestReadMultiline(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("first line\nsecond line\n.\n"))
	value, err := readMultiline(reader, &bytes.Buffer{}, "Text:\n")
	if err != nil {
		t.Fatal(err)
	}
	if value != "first line\nsecond line" {
		t.Fatalf("value = %q", value)
	}
}

func TestReadRecordInputTextCardAndBinary(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		input := strings.NewReader("note\nfirst\nsecond\n.\nmetadata\n")
		payload, metadata, err := readRecordInput(input, bufioForTest(input), &bytes.Buffer{}, model.RecordTypeText)
		if err != nil {
			t.Fatal(err)
		}
		text, ok := payload.(clientmodel.Text)
		if !ok || text.Title != "note" || text.Body != "first\nsecond" || metadata.Text != "metadata" {
			t.Fatalf("payload=%#v metadata=%#v", payload, metadata)
		}
	})

	t.Run("card", func(t *testing.T) {
		input := strings.NewReader("main card\n4111111111111111\nAlice Example\n12/30\n123\npersonal\n")
		payload, metadata, err := readRecordInput(input, bufioForTest(input), &bytes.Buffer{}, model.RecordTypeBankCard)
		if err != nil {
			t.Fatal(err)
		}
		card, ok := payload.(clientmodel.BankCard)
		if !ok || card.Name != "main card" || card.Number != "4111111111111111" || card.CVV != "123" || metadata.Text != "personal" {
			t.Fatalf("payload=%#v metadata=%#v", payload, metadata)
		}
	})

	t.Run("binary", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sample.bin")
		if err := os.WriteFile(path, []byte("binary-data"), 0o600); err != nil {
			t.Fatal(err)
		}
		input := strings.NewReader(path + "\nbackup\n")
		payload, metadata, err := readRecordInput(input, bufioForTest(input), &bytes.Buffer{}, model.RecordTypeBinary)
		if err != nil {
			t.Fatal(err)
		}
		binary, ok := payload.(clientmodel.Binary)
		if !ok || binary.Filename != "sample.bin" || string(binary.Data) != "binary-data" || metadata.Text != "backup" {
			t.Fatalf("payload=%#v metadata=%#v", payload, metadata)
		}
	})
}

func TestWriteRecordTextCredentialsAndCard(t *testing.T) {
	id, _ := model.ParseID(testRecordID)
	tests := []struct {
		name    string
		payload any
		typeID  model.RecordType
		want    string
	}{
		{name: "credentials", typeID: model.RecordTypeCredentials, payload: clientmodel.Credentials{Name: "mail", Login: "alice", Password: "secret"}, want: "Login: alice"},
		{name: "text", typeID: model.RecordTypeText, payload: clientmodel.Text{Title: "note", Body: "body"}, want: "body"},
		{name: "card", typeID: model.RecordTypeBankCard, payload: clientmodel.BankCard{Name: "main", Number: "4111111111111111", Holder: "Alice", ExpiryDate: "12/30", CVV: "123"}, want: "**** 1111"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			view := clientapp.RecordView{ID: id, Type: tt.typeID, Version: 1, Payload: tt.payload, Metadata: clientmodel.Metadata{Text: "metadata"}}
			if err := writeRecord(&output, view, ""); err != nil {
				t.Fatal(err)
			}
			result := output.String()
			if !strings.Contains(result, tt.want) || !strings.Contains(result, "metadata") {
				t.Fatalf("output = %q", result)
			}
			if tt.typeID == model.RecordTypeBankCard {
				if strings.Contains(result, "4111111111111111") {
					t.Fatalf("card number was not masked: %q", result)
				}
				if strings.Contains(result, "CVV:") {
					t.Fatalf("CVV must not be printed: %q", result)
				}
			}
		})
	}
}
