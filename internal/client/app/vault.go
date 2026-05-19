package app

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"

	clientcrypto "github.com/sastromikus/gophkeeper/internal/client/crypto"
	clientmodel "github.com/sastromikus/gophkeeper/internal/client/model"
	"github.com/sastromikus/gophkeeper/internal/client/storage"
	clienttransport "github.com/sastromikus/gophkeeper/internal/client/transport"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const (
	defaultMaxBinarySize            = int64(10 * 1024 * 1024)
	defaultMaxEncryptedPayloadSize  = int64(16 * 1024 * 1024)
	defaultMaxEncryptedMetadataSize = int64(128 * 1024)
)

// VaultAPI describes remote encrypted-record operations used by the client.
type VaultAPI interface {
	CreateRecord(context.Context, string, model.ID, clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error)
	GetRecord(context.Context, string, model.ID) (clienttransport.RemoteRecord, error)
	ListRecords(context.Context, string, string, uint32) (clienttransport.RecordPage, error)
	UpdateRecord(context.Context, string, model.ID, int64, clientcrypto.EncryptedRecordData) (clienttransport.RemoteRecord, error)
	DeleteRecord(context.Context, string, model.ID, int64) (clienttransport.RemoteRecord, error)
}

// VaultCrypto encrypts and decrypts client records.
type VaultCrypto interface {
	OpenDataKey(string, string, clientcrypto.KeyEnvelope) ([]byte, error)
	EncryptRecord([]byte, model.ID, model.RecordType, any, clientmodel.Metadata, clientcrypto.RecordLimits) (clientcrypto.EncryptedRecordData, error)
	DecryptRecord([]byte, model.ID, clientcrypto.EncryptedRecordData, any, *clientmodel.Metadata, clientcrypto.RecordLimits) error
}

// RecordView contains decrypted data for presentation by the CLI.
type RecordView struct {
	ID       model.ID
	Type     model.RecordType
	Version  int64
	Payload  any
	Metadata clientmodel.Metadata
}

// RecordSummary contains display-safe list information.
type RecordSummary struct {
	ID      model.ID
	Type    model.RecordType
	Version int64
	Title   string
}

// VaultService coordinates session loading, key unlocking and remote CRUD.
type VaultService struct {
	api    VaultAPI
	store  SessionStore
	crypto VaultCrypto
	limits clientcrypto.RecordLimits
}

// NewVaultService creates the client vault application service.
func NewVaultService(api VaultAPI, store SessionStore, crypto VaultCrypto) (*VaultService, error) {
	if api == nil || store == nil || crypto == nil {
		return nil, errors.New("client vault dependencies are required")
	}
	return &VaultService{api: api, store: store, crypto: crypto, limits: clientcrypto.RecordLimits{
		MaxBinarySize: defaultMaxBinarySize, MaxEncryptedPayloadSize: defaultMaxEncryptedPayloadSize,
		MaxEncryptedMetadataSize: defaultMaxEncryptedMetadataSize,
	}}, nil
}

// Create encrypts and uploads a new record.
func (service *VaultService) Create(ctx context.Context, password string, recordType model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	id, err := generateID()
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	state, key, err := service.unlock(password)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	defer clientcrypto.Wipe(key)
	encrypted, err := service.crypto.EncryptRecord(key, id, recordType, payload, metadata, service.limits)
	if err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("encrypt record: %w", err)
	}
	record, err := service.api.CreateRecord(ctx, state.Token, id, encrypted)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	return record, nil
}

// Get downloads and decrypts one record.
func (service *VaultService) Get(ctx context.Context, password string, id model.ID) (RecordView, error) {
	state, key, err := service.unlock(password)
	if err != nil {
		return RecordView{}, err
	}
	defer clientcrypto.Wipe(key)
	record, err := service.api.GetRecord(ctx, state.Token, id)
	if err != nil {
		return RecordView{}, err
	}
	payload := payloadTarget(record.Data.Type)
	metadata := clientmodel.Metadata{}
	if err := service.crypto.DecryptRecord(key, record.ID, record.Data, payload, &metadata, service.limits); err != nil {
		return RecordView{}, fmt.Errorf("decrypt record: %w", err)
	}
	return RecordView{ID: record.ID, Type: record.Data.Type, Version: record.Version, Payload: dereferencePayload(payload), Metadata: metadata}, nil
}

// List downloads all active pages and decrypts display-safe summaries.
func (service *VaultService) List(ctx context.Context, password string) ([]RecordSummary, error) {
	state, key, err := service.unlock(password)
	if err != nil {
		return nil, err
	}
	defer clientcrypto.Wipe(key)
	var summaries []RecordSummary
	cursor := ""
	for {
		page, err := service.api.ListRecords(ctx, state.Token, cursor, 100)
		if err != nil {
			return nil, err
		}
		for _, record := range page.Records {
			payload := payloadTarget(record.Data.Type)
			metadata := clientmodel.Metadata{}
			if err := service.crypto.DecryptRecord(key, record.ID, record.Data, payload, &metadata, service.limits); err != nil {
				return nil, fmt.Errorf("decrypt record %s: %w", record.ID, err)
			}
			summaries = append(summaries, RecordSummary{ID: record.ID, Type: record.Data.Type, Version: record.Version, Title: payloadTitle(dereferencePayload(payload))})
		}
		if !page.HasMore {
			break
		}
		if page.NextPageToken == "" || page.NextPageToken == cursor {
			return nil, errors.New("server returned an invalid pagination cursor")
		}
		cursor = page.NextPageToken
	}
	return summaries, nil
}

// Update encrypts replacement data and writes it using the current server version.
func (service *VaultService) Update(ctx context.Context, password string, id model.ID, recordType model.RecordType, payload any, metadata clientmodel.Metadata) (clienttransport.RemoteRecord, error) {
	state, key, err := service.unlock(password)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	defer clientcrypto.Wipe(key)
	current, err := service.api.GetRecord(ctx, state.Token, id)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	encrypted, err := service.crypto.EncryptRecord(key, id, recordType, payload, metadata, service.limits)
	if err != nil {
		return clienttransport.RemoteRecord{}, fmt.Errorf("encrypt updated record: %w", err)
	}
	updated, err := service.api.UpdateRecord(ctx, state.Token, id, current.Version, encrypted)
	if err != nil {
		return clienttransport.RemoteRecord{}, err
	}
	return updated, nil
}

// Delete removes a record using its current server version.
func (service *VaultService) Delete(ctx context.Context, id model.ID) error {
	state, err := service.store.Load()
	if err != nil {
		return fmt.Errorf("load current session: %w", err)
	}
	record, err := service.api.GetRecord(ctx, state.Token, id)
	if err != nil {
		return err
	}
	if _, err := service.api.DeleteRecord(ctx, state.Token, id, record.Version); err != nil {
		return err
	}
	return nil
}

func (service *VaultService) unlock(password string) (storage.SessionState, []byte, error) {
	if password == "" {
		return storage.SessionState{}, nil, errors.New("master password is required")
	}
	state, err := service.store.Load()
	if err != nil {
		return storage.SessionState{}, nil, fmt.Errorf("load current session: %w", err)
	}
	envelope := clientcrypto.KeyEnvelope{EncryptedDataKey: state.EncryptedDataKey, Salt: state.KeySalt, Nonce: state.KeyNonce, KeyDerivationVersion: state.KeyDerivationVersion}
	key, err := service.crypto.OpenDataKey(password, state.Login, envelope)
	if err != nil {
		return storage.SessionState{}, nil, fmt.Errorf("unlock account data key: %w", err)
	}
	return state, key, nil
}

func generateID() (model.ID, error) {
	var value [16]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate record ID: %w", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	text := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
	id, err := model.ParseID(text)
	if err != nil {
		return "", fmt.Errorf("parse generated record ID: %w", err)
	}
	return id, nil
}

func payloadTarget(recordType model.RecordType) any {
	switch recordType {
	case model.RecordTypeCredentials:
		return &clientmodel.Credentials{}
	case model.RecordTypeText:
		return &clientmodel.Text{}
	case model.RecordTypeBinary:
		return &clientmodel.Binary{}
	case model.RecordTypeBankCard:
		return &clientmodel.BankCard{}
	default:
		return &struct{}{}
	}
}

func dereferencePayload(value any) any {
	switch typed := value.(type) {
	case *clientmodel.Credentials:
		return *typed
	case *clientmodel.Text:
		return *typed
	case *clientmodel.Binary:
		return *typed
	case *clientmodel.BankCard:
		return *typed
	default:
		return value
	}
}

func payloadTitle(value any) string {
	switch typed := value.(type) {
	case clientmodel.Credentials:
		return typed.Name
	case clientmodel.Text:
		return typed.Title
	case clientmodel.Binary:
		return typed.Filename
	case clientmodel.BankCard:
		return typed.Name
	default:
		return ""
	}
}
