package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionState contains the local authentication state. The data key remains
// encrypted; plaintext key material is never persisted by this store.
type SessionState struct {
	Login                string    `json:"login"`
	Token                string    `json:"token"`
	ExpiresAt            time.Time `json:"expires_at"`
	EncryptedDataKey     []byte    `json:"encrypted_data_key"`
	KeySalt              []byte    `json:"key_salt"`
	KeyNonce             []byte    `json:"key_nonce"`
	KeyDerivationVersion uint32    `json:"key_derivation_version"`
}

// FileSessionStore persists authentication state in a user-specific file.
type FileSessionStore struct {
	path string
}

// NewFileSessionStore creates a file-backed session store.
func NewFileSessionStore(path string) (*FileSessionStore, error) {
	if path == "" {
		return nil, errors.New("session file path is required")
	}
	return &FileSessionStore{path: path}, nil
}

// Save atomically replaces the persisted authentication state.
func (store *FileSessionStore) Save(state SessionState) error {
	if state.Login == "" || state.Token == "" || state.ExpiresAt.IsZero() {
		return errors.New("login, token, and expiration are required")
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create client configuration directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".gophkeeper-session-*")
	if err != nil {
		return fmt.Errorf("create temporary session file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restrict temporary session file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary session file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary session file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary session file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		// Windows cannot replace an existing file atomically with os.Rename.
		if removeErr := os.Remove(store.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove previous session file: %w", removeErr)
		}
		if err := os.Rename(temporaryPath, store.path); err != nil {
			return fmt.Errorf("replace session file: %w", err)
		}
	}
	return nil
}

// Load reads persisted authentication state.
func (store *FileSessionStore) Load() (SessionState, error) {
	data, err := os.ReadFile(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionState{}, fmt.Errorf("session is not available: %w", os.ErrNotExist)
		}
		return SessionState{}, fmt.Errorf("read session file: %w", err)
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return SessionState{}, fmt.Errorf("decode session state: %w", err)
	}
	if state.Login == "" || state.Token == "" || state.ExpiresAt.IsZero() {
		return SessionState{}, errors.New("stored session state is incomplete")
	}
	return state, nil
}

// Delete removes persisted authentication state.
func (store *FileSessionStore) Delete() error {
	if err := os.Remove(store.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove session file: %w", err)
	}
	return nil
}
