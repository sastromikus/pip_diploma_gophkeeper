package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sastromikus/gophkeeper/internal/model"
)

func TestRepositoriesIntegration(t *testing.T) {
	dsn := os.Getenv("GOPHKEEPER_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("GOPHKEEPER_TEST_DATABASE_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Migrate(ctx, dsn); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	database, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(database.Close)

	var usersTableExists bool
	if err := database.Pool().QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&usersTableExists); err != nil {
		t.Fatalf("check migrated schema: %v", err)
	}
	if !usersTableExists {
		t.Fatal("migration completed without creating the users table")
	}

	userID := mustID(t, "11111111-1111-4111-8111-111111111111")
	sessionID := mustID(t, "22222222-2222-4222-8222-222222222222")
	recordID := mustID(t, "33333333-3333-4333-8333-333333333333")
	login := fmt.Sprintf("integration-%d", time.Now().UnixNano())

	if _, err := database.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", userID.String()); err != nil {
		t.Fatalf("clean stale integration data: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := database.Pool().Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID.String()); err != nil {
			t.Errorf("clean integration data: %v", err)
		}
	})

	users := NewUserRepository(database.Pool())
	sessions := NewSessionRepository(database.Pool())
	records := NewRecordRepository(database.Pool())

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	user := model.User{
		ID:                   userID,
		Login:                login,
		PasswordHash:         "encoded-password-hash",
		EncryptedDataKey:     []byte("encrypted-key"),
		KeySalt:              []byte("salt"),
		KeyNonce:             []byte("nonce"),
		KeyDerivationVersion: 1,
		CreatedAt:            createdAt,
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("UserRepository.Create() error = %v", err)
	}
	storedUser, err := users.GetByLogin(ctx, login)
	if err != nil {
		t.Fatalf("UserRepository.GetByLogin() error = %v", err)
	}
	if storedUser.ID != userID {
		t.Fatalf("stored user ID = %q, want %q", storedUser.ID, userID)
	}

	session := model.Session{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: []byte("token-hash"),
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(time.Hour),
	}
	if err := sessions.Create(ctx, session); err != nil {
		t.Fatalf("SessionRepository.Create() error = %v", err)
	}
	storedSession, err := sessions.GetByTokenHash(ctx, session.TokenHash)
	if err != nil {
		t.Fatalf("SessionRepository.GetByTokenHash() error = %v", err)
	}
	if !storedSession.ActiveAt(createdAt.Add(time.Minute)) {
		t.Fatal("stored session is unexpectedly inactive")
	}
	if err := sessions.Revoke(ctx, sessionID, createdAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("SessionRepository.Revoke() error = %v", err)
	}

	record := model.Record{
		ID:                recordID,
		UserID:            userID,
		Type:              model.RecordTypeCredentials,
		EncryptionVersion: 1,
		EncryptedPayload:  []byte("ciphertext"),
		EncryptedMetadata: []byte("metadata"),
		PayloadNonce:      []byte("payload-nonce"),
		MetadataNonce:     []byte("metadata-nonce"),
	}
	createdRecord, err := records.Create(ctx, record)
	if err != nil {
		t.Fatalf("RecordRepository.Create() error = %v", err)
	}
	if createdRecord.Version != 1 || createdRecord.Revision < 1 {
		t.Fatalf("created record version/revision = %d/%d", createdRecord.Version, createdRecord.Revision)
	}

	record.EncryptedPayload = []byte("updated-ciphertext")
	updatedRecord, err := records.Update(ctx, record, createdRecord.Version)
	if err != nil {
		t.Fatalf("RecordRepository.Update() error = %v", err)
	}
	if updatedRecord.Version != createdRecord.Version+1 {
		t.Fatalf("updated version = %d, want %d", updatedRecord.Version, createdRecord.Version+1)
	}
	if _, err := records.Update(ctx, record, createdRecord.Version); !errors.Is(err, model.ErrVersionConflict) {
		t.Fatalf("stale Update() error = %v, want ErrVersionConflict", err)
	}

	tombstone, err := records.Delete(ctx, userID, recordID, updatedRecord.Version)
	if err != nil {
		t.Fatalf("RecordRepository.Delete() error = %v", err)
	}
	if !tombstone.Deleted() || len(tombstone.EncryptedPayload) != 0 {
		t.Fatalf("Delete() returned invalid tombstone: %+v", tombstone)
	}
	changes, err := records.ListChangedAfter(ctx, userID, updatedRecord.Revision, 10)
	if err != nil {
		t.Fatalf("RecordRepository.ListChangedAfter() error = %v", err)
	}
	if len(changes) != 1 || changes[0].ID != recordID || !changes[0].Deleted() {
		t.Fatalf("changes = %+v, want one tombstone", changes)
	}
}

func mustID(t *testing.T, value string) model.ID {
	t.Helper()
	id, err := model.ParseID(value)
	if err != nil {
		t.Fatalf("ParseID(%q) error = %v", value, err)
	}
	return id
}
