package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	domain "github.com/sastromikus/gophkeeper/internal/model"
	"golang.org/x/crypto/argon2"
)

const (
	// CurrentKeyDerivationVersion identifies the currently supported key envelope format.
	CurrentKeyDerivationVersion uint32 = 1
	// DataKeySize is the AES-256 data encryption key length in bytes.
	DataKeySize = 32
	// SaltSize is the Argon2id salt length in bytes.
	SaltSize = 16
	// NonceSize is the AES-GCM nonce length in bytes.
	NonceSize = 12

	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
)

var (
	// ErrInvalidKeyMaterial indicates an invalid key, salt, nonce, or key envelope.
	ErrInvalidKeyMaterial = errors.New("invalid key material")
	// ErrDecryptionFailed indicates that encrypted data could not be authenticated or decrypted.
	ErrDecryptionFailed = errors.New("decryption failed")
	// ErrUnsupportedVersion indicates an unknown cryptographic format version.
	ErrUnsupportedVersion = errors.New("unsupported cryptographic version")
)

// KeyEnvelope contains the encrypted data key and the information needed to derive its wrapping key.
type KeyEnvelope struct {
	EncryptedDataKey     []byte
	Salt                 []byte
	Nonce                []byte
	KeyDerivationVersion uint32
}

// EncryptedRecordData contains one encrypted record payload and its encrypted metadata.
type EncryptedRecordData struct {
	Type              domain.RecordType
	EncryptedPayload  []byte
	EncryptedMetadata []byte
	PayloadNonce      []byte
	MetadataNonce     []byte
}

// Service performs client-side key management and authenticated encryption.
type Service struct {
	random io.Reader
}

// NewService creates a client-side cryptographic service backed by crypto/rand.
func NewService() *Service {
	return &Service{random: cryptorand.Reader}
}

// newServiceWithRandom creates a service with deterministic randomness for tests.
func newServiceWithRandom(random io.Reader) (*Service, error) {
	if random == nil {
		return nil, errors.New("random source is required")
	}
	return &Service{random: random}, nil
}

// CreateDataKey generates a random data key and protects it with a key derived from masterPassword.
func (service *Service) CreateDataKey(masterPassword string) ([]byte, KeyEnvelope, error) {
	if masterPassword == "" {
		return nil, KeyEnvelope{}, fmt.Errorf("%w: master password is required", ErrInvalidKeyMaterial)
	}

	dataKey, err := service.randomBytes(DataKeySize)
	if err != nil {
		return nil, KeyEnvelope{}, fmt.Errorf("generate data key: %w", err)
	}
	salt, err := service.randomBytes(SaltSize)
	if err != nil {
		Wipe(dataKey)
		return nil, KeyEnvelope{}, fmt.Errorf("generate key salt: %w", err)
	}
	wrappingKey := deriveWrappingKey(masterPassword, salt)
	defer Wipe(wrappingKey)

	nonce, encryptedDataKey, err := service.seal(wrappingKey, dataKey, keyEnvelopeAAD(CurrentKeyDerivationVersion))
	if err != nil {
		Wipe(dataKey)
		return nil, KeyEnvelope{}, fmt.Errorf("encrypt data key: %w", err)
	}

	return dataKey, KeyEnvelope{
		EncryptedDataKey:     encryptedDataKey,
		Salt:                 salt,
		Nonce:                nonce,
		KeyDerivationVersion: CurrentKeyDerivationVersion,
	}, nil
}

// OpenDataKey derives the wrapping key and decrypts a data key envelope.
func (service *Service) OpenDataKey(masterPassword string, envelope KeyEnvelope) ([]byte, error) {
	if masterPassword == "" {
		return nil, fmt.Errorf("%w: master password is required", ErrInvalidKeyMaterial)
	}
	if envelope.KeyDerivationVersion != CurrentKeyDerivationVersion {
		return nil, fmt.Errorf("%w: key derivation version %d", ErrUnsupportedVersion, envelope.KeyDerivationVersion)
	}
	if len(envelope.Salt) != SaltSize || len(envelope.Nonce) != NonceSize || len(envelope.EncryptedDataKey) == 0 {
		return nil, fmt.Errorf("%w: malformed key envelope", ErrInvalidKeyMaterial)
	}

	wrappingKey := deriveWrappingKey(masterPassword, envelope.Salt)
	defer Wipe(wrappingKey)
	dataKey, err := open(wrappingKey, envelope.Nonce, envelope.EncryptedDataKey, keyEnvelopeAAD(envelope.KeyDerivationVersion))
	if err != nil {
		return nil, fmt.Errorf("%w: open data key", ErrDecryptionFailed)
	}
	if len(dataKey) != DataKeySize {
		Wipe(dataKey)
		return nil, fmt.Errorf("%w: unexpected data key length", ErrInvalidKeyMaterial)
	}
	return dataKey, nil
}

// EncryptRecord validates, serializes, and encrypts a plaintext record and its metadata.
func (service *Service) EncryptRecord(dataKey []byte, recordType domain.RecordType, payload any, metadata clientmodel.Metadata, maxBinarySize int64) (EncryptedRecordData, error) {
	if err := validateDataKey(dataKey); err != nil {
		return EncryptedRecordData{}, err
	}
	if err := recordType.Validate(); err != nil {
		return EncryptedRecordData{}, err
	}
	if err := validatePayload(recordType, payload, maxBinarySize); err != nil {
		return EncryptedRecordData{}, err
	}
	if err := metadata.Validate(); err != nil {
		return EncryptedRecordData{}, fmt.Errorf("validate metadata: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("serialize record payload: %w", err)
	}
	defer Wipe(payloadJSON)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("serialize record metadata: %w", err)
	}
	defer Wipe(metadataJSON)

	payloadNonce, encryptedPayload, err := service.seal(dataKey, payloadJSON, recordAAD(recordType, "payload"))
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("encrypt record payload: %w", err)
	}
	metadataNonce, encryptedMetadata, err := service.seal(dataKey, metadataJSON, recordAAD(recordType, "metadata"))
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("encrypt record metadata: %w", err)
	}

	return EncryptedRecordData{
		Type:              recordType,
		EncryptedPayload:  encryptedPayload,
		EncryptedMetadata: encryptedMetadata,
		PayloadNonce:      payloadNonce,
		MetadataNonce:     metadataNonce,
	}, nil
}

// DecryptRecord decrypts and deserializes one record into payload and metadata.
func (service *Service) DecryptRecord(dataKey []byte, encrypted EncryptedRecordData, payload any, metadata *clientmodel.Metadata, maxBinarySize int64) error {
	if err := validateDataKey(dataKey); err != nil {
		return err
	}
	if err := encrypted.Type.Validate(); err != nil {
		return err
	}
	if payload == nil || metadata == nil {
		return fmt.Errorf("%w: payload and metadata destinations are required", domain.ErrInvalidInput)
	}
	if len(encrypted.PayloadNonce) != NonceSize || len(encrypted.MetadataNonce) != NonceSize || len(encrypted.EncryptedPayload) == 0 || len(encrypted.EncryptedMetadata) == 0 {
		return fmt.Errorf("%w: malformed encrypted record", ErrInvalidKeyMaterial)
	}

	payloadJSON, err := open(dataKey, encrypted.PayloadNonce, encrypted.EncryptedPayload, recordAAD(encrypted.Type, "payload"))
	if err != nil {
		return fmt.Errorf("%w: open record payload", ErrDecryptionFailed)
	}
	defer Wipe(payloadJSON)
	metadataJSON, err := open(dataKey, encrypted.MetadataNonce, encrypted.EncryptedMetadata, recordAAD(encrypted.Type, "metadata"))
	if err != nil {
		return fmt.Errorf("%w: open record metadata", ErrDecryptionFailed)
	}
	defer Wipe(metadataJSON)

	if err := json.Unmarshal(payloadJSON, payload); err != nil {
		return fmt.Errorf("decode record payload: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, metadata); err != nil {
		return fmt.Errorf("decode record metadata: %w", err)
	}
	if err := validatePayload(encrypted.Type, payload, maxBinarySize); err != nil {
		return fmt.Errorf("validate decrypted payload: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("validate decrypted metadata: %w", err)
	}
	return nil
}

// Wipe overwrites a byte slice in place. Callers remain responsible for avoiding extra copies.
func Wipe(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func deriveWrappingKey(masterPassword string, salt []byte) []byte {
	return argon2.IDKey([]byte(masterPassword), salt, argonTime, argonMemory, argonThreads, DataKeySize)
}

func (service *Service) seal(key, plaintext, aad []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	nonce, err := service.randomBytes(aead.NonceSize())
	if err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, aead.Seal(nil, nonce, plaintext, aad), nil
}

func open(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, ErrInvalidKeyMaterial
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func (service *Service) randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(service.random, value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateDataKey(dataKey []byte) error {
	if len(dataKey) != DataKeySize {
		return fmt.Errorf("%w: data key must contain %d bytes", ErrInvalidKeyMaterial, DataKeySize)
	}
	return nil
}

func validatePayload(recordType domain.RecordType, payload any, maxBinarySize int64) error {
	switch recordType {
	case domain.RecordTypeCredentials:
		value, ok := payloadValue[clientmodel.Credentials](payload)
		if !ok {
			return payloadTypeError(recordType)
		}
		return value.Validate()
	case domain.RecordTypeText:
		value, ok := payloadValue[clientmodel.Text](payload)
		if !ok {
			return payloadTypeError(recordType)
		}
		return value.Validate()
	case domain.RecordTypeBinary:
		value, ok := payloadValue[clientmodel.Binary](payload)
		if !ok {
			return payloadTypeError(recordType)
		}
		return value.Validate(maxBinarySize)
	case domain.RecordTypeBankCard:
		value, ok := payloadValue[clientmodel.BankCard](payload)
		if !ok {
			return payloadTypeError(recordType)
		}
		return value.Validate()
	default:
		return recordType.Validate()
	}
}

func payloadValue[T any](payload any) (T, bool) {
	if value, ok := payload.(T); ok {
		return value, true
	}
	if pointer, ok := payload.(*T); ok && pointer != nil {
		return *pointer, true
	}
	var zero T
	return zero, false
}

func payloadTypeError(recordType domain.RecordType) error {
	return fmt.Errorf("%w: payload type does not match record type %q", domain.ErrInvalidInput, recordType)
}

func keyEnvelopeAAD(version uint32) []byte {
	return []byte(fmt.Sprintf("gophkeeper:key-envelope:v%d", version))
}

func recordAAD(recordType domain.RecordType, part string) []byte {
	return []byte("gophkeeper:record:v1:" + string(recordType) + ":" + part)
}
