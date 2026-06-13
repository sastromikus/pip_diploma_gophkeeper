package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSessionStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := NewFileSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	want := SessionState{Login: "alice", Token: "token", ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Second), EncryptedDataKey: []byte{1, 2}, KeySalt: []byte{3}, KeyNonce: []byte{4}, KeyDerivationVersion: 1}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Login != want.Login || got.Token != want.Token || !got.ExpiresAt.Equal(want.ExpiresAt) || got.KeyDerivationVersion != 1 {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestNewFileSessionStoreRejectsEmptyPath(t *testing.T) {
	if _, err := NewFileSessionStore(""); err == nil {
		t.Fatal("expected error")
	}
}
