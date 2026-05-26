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
	CurrentKeyDerivationVersion uint32 = 1
	CurrentEncryptionVersion           = domain.CurrentRecordEncryptionVersion
	DataKeySize                        = 32
	SaltSize                           = 16
	NonceSize                          = domain.RecordNonceSize
	AEADTagSize                        = domain.RecordAuthenticationTagSize
	EncryptedDataKeySize               = DataKeySize + AEADTagSize

	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
)

var (
	ErrInvalidKeyMaterial = errors.New("invalid key material")
	ErrDecryptionFailed   = errors.New("decryption failed")
	ErrUnsupportedVersion = errors.New("unsupported cryptographic version")
)

type KeyEnvelope struct {
	EncryptedDataKey     []byte
	Salt                 []byte
	Nonce                []byte
	KeyDerivationVersion uint32
}

type EncryptedRecordData struct {
	Type              domain.RecordType
	EncryptionVersion uint32
	EncryptedPayload  []byte
	EncryptedMetadata []byte
	PayloadNonce      []byte
	MetadataNonce     []byte
}

type RecordLimits struct {
	MaxBinarySize            int64
	MaxEncryptedPayloadSize  int64
	MaxEncryptedMetadataSize int64
}

func (limits RecordLimits) validate() error {
	if limits.MaxBinarySize <= 0 || limits.MaxEncryptedPayloadSize <= 0 || limits.MaxEncryptedMetadataSize <= 0 {
		return fmt.Errorf("%w: cryptographic record limits must be positive", domain.ErrInvalidInput)
	}
	return nil
}

type Service struct{ random io.Reader }

func NewService() *Service { return &Service{random: cryptorand.Reader} }

func newServiceWithRandom(random io.Reader) (*Service, error) {
	if random == nil {
		return nil, errors.New("random source is required")
	}
	return &Service{random: random}, nil
}

func (service *Service) CreateDataKey(masterPassword, accountBinding string) ([]byte, KeyEnvelope, error) {
	if masterPassword == "" {
		return nil, KeyEnvelope{}, fmt.Errorf("%w: master password is required", ErrInvalidKeyMaterial)
	}
	if accountBinding == "" {
		return nil, KeyEnvelope{}, fmt.Errorf("%w: account binding is required", ErrInvalidKeyMaterial)
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
	nonce, encryptedDataKey, err := service.seal(wrappingKey, dataKey, keyEnvelopeAAD(CurrentKeyDerivationVersion, accountBinding))
	if err != nil {
		Wipe(dataKey)
		return nil, KeyEnvelope{}, fmt.Errorf("encrypt data key: %w", err)
	}
	return dataKey, KeyEnvelope{EncryptedDataKey: encryptedDataKey, Salt: salt, Nonce: nonce, KeyDerivationVersion: CurrentKeyDerivationVersion}, nil
}

func (service *Service) OpenDataKey(masterPassword, accountBinding string, envelope KeyEnvelope) ([]byte, error) {
	if masterPassword == "" || accountBinding == "" {
		return nil, fmt.Errorf("%w: master password and account binding are required", ErrInvalidKeyMaterial)
	}
	if envelope.KeyDerivationVersion != CurrentKeyDerivationVersion {
		return nil, fmt.Errorf("%w: key derivation version %d", ErrUnsupportedVersion, envelope.KeyDerivationVersion)
	}
	if len(envelope.Salt) != SaltSize || len(envelope.Nonce) != NonceSize || len(envelope.EncryptedDataKey) != EncryptedDataKeySize {
		return nil, fmt.Errorf("%w: malformed key envelope", ErrInvalidKeyMaterial)
	}
	wrappingKey := deriveWrappingKey(masterPassword, envelope.Salt)
	defer Wipe(wrappingKey)
	dataKey, err := open(wrappingKey, envelope.Nonce, envelope.EncryptedDataKey, keyEnvelopeAAD(envelope.KeyDerivationVersion, accountBinding))
	if err != nil {
		return nil, fmt.Errorf("%w: open data key", ErrDecryptionFailed)
	}
	if len(dataKey) != DataKeySize {
		Wipe(dataKey)
		return nil, fmt.Errorf("%w: unexpected data key length", ErrInvalidKeyMaterial)
	}
	return dataKey, nil
}

func (service *Service) EncryptRecord(dataKey []byte, recordID domain.ID, recordType domain.RecordType, payload any, metadata clientmodel.Metadata, limits RecordLimits) (EncryptedRecordData, error) {
	if err := validateDataKey(dataKey); err != nil {
		return EncryptedRecordData{}, err
	}
	if recordID.IsZero() {
		return EncryptedRecordData{}, fmt.Errorf("%w: record ID is required", domain.ErrInvalidInput)
	}
	if err := recordType.Validate(); err != nil {
		return EncryptedRecordData{}, err
	}
	if err := limits.validate(); err != nil {
		return EncryptedRecordData{}, err
	}
	if err := validatePayload(recordType, payload, limits.MaxBinarySize); err != nil {
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
	payloadNonce, encryptedPayload, err := service.seal(dataKey, payloadJSON, recordAAD(CurrentEncryptionVersion, recordID, recordType, "payload"))
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("encrypt record payload: %w", err)
	}
	metadataNonce, encryptedMetadata, err := service.seal(dataKey, metadataJSON, recordAAD(CurrentEncryptionVersion, recordID, recordType, "metadata"))
	if err != nil {
		return EncryptedRecordData{}, fmt.Errorf("encrypt record metadata: %w", err)
	}
	if int64(len(encryptedPayload)) > limits.MaxEncryptedPayloadSize || int64(len(encryptedMetadata)) > limits.MaxEncryptedMetadataSize {
		return EncryptedRecordData{}, fmt.Errorf("%w: encrypted record exceeds configured limits", domain.ErrPayloadTooLarge)
	}
	return EncryptedRecordData{Type: recordType, EncryptionVersion: CurrentEncryptionVersion, EncryptedPayload: encryptedPayload, EncryptedMetadata: encryptedMetadata, PayloadNonce: payloadNonce, MetadataNonce: metadataNonce}, nil
}

func (service *Service) DecryptRecord(dataKey []byte, recordID domain.ID, encrypted EncryptedRecordData, payload any, metadata *clientmodel.Metadata, limits RecordLimits) error {
	if err := validateDataKey(dataKey); err != nil {
		return err
	}
	if recordID.IsZero() {
		return fmt.Errorf("%w: record ID is required", domain.ErrInvalidInput)
	}
	if err := encrypted.Type.Validate(); err != nil {
		return err
	}
	if encrypted.EncryptionVersion != CurrentEncryptionVersion {
		return fmt.Errorf("%w: record encryption version %d", ErrUnsupportedVersion, encrypted.EncryptionVersion)
	}
	if err := limits.validate(); err != nil {
		return err
	}
	if payload == nil || metadata == nil {
		return fmt.Errorf("%w: payload and metadata destinations are required", domain.ErrInvalidInput)
	}
	if len(encrypted.PayloadNonce) != NonceSize || len(encrypted.MetadataNonce) != NonceSize || len(encrypted.EncryptedPayload) < AEADTagSize || len(encrypted.EncryptedMetadata) < AEADTagSize {
		return fmt.Errorf("%w: malformed encrypted record", ErrInvalidKeyMaterial)
	}
	if int64(len(encrypted.EncryptedPayload)) > limits.MaxEncryptedPayloadSize || int64(len(encrypted.EncryptedMetadata)) > limits.MaxEncryptedMetadataSize {
		return fmt.Errorf("%w: encrypted record exceeds configured limits", domain.ErrPayloadTooLarge)
	}
	payloadJSON, err := open(dataKey, encrypted.PayloadNonce, encrypted.EncryptedPayload, recordAAD(encrypted.EncryptionVersion, recordID, encrypted.Type, "payload"))
	if err != nil {
		return fmt.Errorf("%w: open record payload", ErrDecryptionFailed)
	}
	defer Wipe(payloadJSON)
	metadataJSON, err := open(dataKey, encrypted.MetadataNonce, encrypted.EncryptedMetadata, recordAAD(encrypted.EncryptionVersion, recordID, encrypted.Type, "metadata"))
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
	if err := validatePayload(encrypted.Type, payload, limits.MaxBinarySize); err != nil {
		return fmt.Errorf("validate decrypted payload: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return fmt.Errorf("validate decrypted metadata: %w", err)
	}
	return nil
}

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
func keyEnvelopeAAD(version uint32, accountBinding string) []byte {
	return []byte(fmt.Sprintf("gophkeeper:key-envelope:v%d:account:%s", version, accountBinding))
}
func recordAAD(version uint32, recordID domain.ID, recordType domain.RecordType, part string) []byte {
	return []byte(fmt.Sprintf("gophkeeper:record:v%d:id:%s:type:%s:part:%s", version, recordID.String(), recordType, part))
}
