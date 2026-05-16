package crypto

import (
	"bytes"
	"errors"
	"testing"

	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	domain "github.com/sastromikus/gophkeeper/internal/model"
)

const testBinaryLimit = int64(1 << 20)

func TestCreateAndOpenDataKey(t *testing.T) {
	service := NewService()
	dataKey, envelope, err := service.CreateDataKey("correct horse battery staple")
	if err != nil {
		t.Fatalf("CreateDataKey() error = %v", err)
	}
	defer Wipe(dataKey)

	opened, err := service.OpenDataKey("correct horse battery staple", envelope)
	if err != nil {
		t.Fatalf("OpenDataKey() error = %v", err)
	}
	defer Wipe(opened)
	if !bytes.Equal(opened, dataKey) {
		t.Fatal("opened data key differs from generated key")
	}
}

func TestOpenDataKeyRejectsWrongPassword(t *testing.T) {
	service := NewService()
	dataKey, envelope, err := service.CreateDataKey("correct password")
	if err != nil {
		t.Fatalf("CreateDataKey() error = %v", err)
	}
	Wipe(dataKey)

	_, err = service.OpenDataKey("wrong password", envelope)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("OpenDataKey() error = %v, want ErrDecryptionFailed", err)
	}
}

func TestOpenDataKeyRejectsUnsupportedVersion(t *testing.T) {
	service := NewService()
	_, err := service.OpenDataKey("password", KeyEnvelope{KeyDerivationVersion: 99})
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("OpenDataKey() error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestEncryptDecryptCredentials(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{7}, DataKeySize)
	input := clientmodel.Credentials{Name: "mail", Login: "alice", Password: "secret"}
	metadata := clientmodel.Metadata{Text: "personal"}

	encrypted, err := service.EncryptRecord(key, domain.RecordTypeCredentials, input, metadata, testBinaryLimit)
	if err != nil {
		t.Fatalf("EncryptRecord() error = %v", err)
	}
	var output clientmodel.Credentials
	var outputMetadata clientmodel.Metadata
	if err := service.DecryptRecord(key, encrypted, &output, &outputMetadata, testBinaryLimit); err != nil {
		t.Fatalf("DecryptRecord() error = %v", err)
	}
	if output != input {
		t.Fatalf("payload = %#v, want %#v", output, input)
	}
	if outputMetadata != metadata {
		t.Fatalf("metadata = %#v, want %#v", outputMetadata, metadata)
	}
}

func TestEncryptRecordUsesDistinctNonces(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{1}, DataKeySize)
	payload := clientmodel.Text{Title: "note", Body: "body"}
	metadata := clientmodel.Metadata{}

	first, err := service.EncryptRecord(key, domain.RecordTypeText, payload, metadata, testBinaryLimit)
	if err != nil {
		t.Fatalf("first EncryptRecord() error = %v", err)
	}
	second, err := service.EncryptRecord(key, domain.RecordTypeText, payload, metadata, testBinaryLimit)
	if err != nil {
		t.Fatalf("second EncryptRecord() error = %v", err)
	}
	if bytes.Equal(first.PayloadNonce, second.PayloadNonce) {
		t.Fatal("payload nonce was reused")
	}
	if bytes.Equal(first.MetadataNonce, second.MetadataNonce) {
		t.Fatal("metadata nonce was reused")
	}
	if bytes.Equal(first.EncryptedPayload, second.EncryptedPayload) {
		t.Fatal("ciphertext is identical despite fresh nonce")
	}
}

func TestDecryptRecordRejectsTampering(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{2}, DataKeySize)
	encrypted, err := service.EncryptRecord(key, domain.RecordTypeText, clientmodel.Text{Title: "note", Body: "body"}, clientmodel.Metadata{}, testBinaryLimit)
	if err != nil {
		t.Fatalf("EncryptRecord() error = %v", err)
	}
	encrypted.EncryptedPayload[0] ^= 0xff

	var output clientmodel.Text
	var metadata clientmodel.Metadata
	err = service.DecryptRecord(key, encrypted, &output, &metadata, testBinaryLimit)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("DecryptRecord() error = %v, want ErrDecryptionFailed", err)
	}
}

func TestDecryptRecordBindsTypeAsAAD(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{3}, DataKeySize)
	encrypted, err := service.EncryptRecord(key, domain.RecordTypeText, clientmodel.Text{Title: "note", Body: "body"}, clientmodel.Metadata{}, testBinaryLimit)
	if err != nil {
		t.Fatalf("EncryptRecord() error = %v", err)
	}
	encrypted.Type = domain.RecordTypeCredentials

	var output clientmodel.Credentials
	var metadata clientmodel.Metadata
	err = service.DecryptRecord(key, encrypted, &output, &metadata, testBinaryLimit)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("DecryptRecord() error = %v, want ErrDecryptionFailed", err)
	}
}

func TestEncryptRecordRejectsMismatchedPayloadType(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{4}, DataKeySize)
	_, err := service.EncryptRecord(key, domain.RecordTypeCredentials, clientmodel.Text{Title: "note", Body: "body"}, clientmodel.Metadata{}, testBinaryLimit)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("EncryptRecord() error = %v, want ErrInvalidInput", err)
	}
}

func TestEncryptDecryptBinary(t *testing.T) {
	service := NewService()
	key := bytes.Repeat([]byte{5}, DataKeySize)
	input := clientmodel.Binary{Filename: "document.bin", MIMEType: "application/octet-stream", Data: []byte{1, 2, 3, 4}}
	encrypted, err := service.EncryptRecord(key, domain.RecordTypeBinary, input, clientmodel.Metadata{}, testBinaryLimit)
	if err != nil {
		t.Fatalf("EncryptRecord() error = %v", err)
	}
	var output clientmodel.Binary
	var metadata clientmodel.Metadata
	if err := service.DecryptRecord(key, encrypted, &output, &metadata, testBinaryLimit); err != nil {
		t.Fatalf("DecryptRecord() error = %v", err)
	}
	if output.Filename != input.Filename || output.MIMEType != input.MIMEType || !bytes.Equal(output.Data, input.Data) {
		t.Fatalf("payload = %#v, want %#v", output, input)
	}
}

func TestWipe(t *testing.T) {
	value := []byte{1, 2, 3}
	Wipe(value)
	if !bytes.Equal(value, []byte{0, 0, 0}) {
		t.Fatalf("Wipe() result = %v", value)
	}
}
