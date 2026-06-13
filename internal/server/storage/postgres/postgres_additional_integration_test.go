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

func TestRegistrationRepositoryIntegration(t *testing.T) {
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

	userID := mustID(t, "44444444-4444-4444-8444-444444444444")
	sessionID := mustID(t, "55555555-5555-4555-8555-555555555555")
	rollbackUserID := mustID(t, "66666666-6666-4666-8666-666666666666")
	rollbackSessionID := mustID(t, "77777777-7777-4777-8777-777777777777")

	for _, id := range []model.ID{userID, rollbackUserID} {
		if _, err := database.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", id.String()); err != nil {
			t.Fatalf("clean stale registration data: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		for _, id := range []model.ID{userID, rollbackUserID} {
			if _, err := database.Pool().Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", id.String()); err != nil {
				t.Errorf("clean registration data for %s: %v", id, err)
			}
		}
	})

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	login := fmt.Sprintf("registration-%d", time.Now().UnixNano())
	user := integrationUser(userID, login, createdAt)
	session := integrationSession(sessionID, userID, []byte("registration-token-hash"), createdAt)

	repository := NewRegistrationRepository(database.Pool())
	if err := repository.CreateUserAndSession(ctx, user, session); err != nil {
		t.Fatalf("CreateUserAndSession() error = %v", err)
	}

	users := NewUserRepository(database.Pool())
	sessions := NewSessionRepository(database.Pool())
	if stored, err := users.GetByID(ctx, userID); err != nil || stored.Login != login {
		t.Fatalf("GetByID() = %+v, %v", stored, err)
	}
	if stored, err := sessions.GetByTokenHash(ctx, session.TokenHash); err != nil || stored.UserID != userID {
		t.Fatalf("GetByTokenHash() = %+v, %v", stored, err)
	}

	if err := repository.CreateUserAndSession(ctx, user, session); !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("duplicate CreateUserAndSession() error = %v, want ErrAlreadyExists", err)
	}

	mismatched := session
	mismatched.UserID = rollbackUserID
	if err := repository.CreateUserAndSession(ctx, user, mismatched); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("mismatched CreateUserAndSession() error = %v, want ErrInvalidInput", err)
	}

	rollbackUser := integrationUser(rollbackUserID, fmt.Sprintf("rollback-%d", time.Now().UnixNano()), createdAt)
	rollbackSession := integrationSession(rollbackSessionID, rollbackUserID, session.TokenHash, createdAt)
	if err := repository.CreateUserAndSession(ctx, rollbackUser, rollbackSession); !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("duplicate-token CreateUserAndSession() error = %v, want ErrAlreadyExists", err)
	}
	if _, err := users.GetByID(ctx, rollbackUserID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("rolled-back user lookup error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryErrorPathsIntegration(t *testing.T) {
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

	userID := mustID(t, "88888888-8888-4888-8888-888888888888")
	sessionID := mustID(t, "99999999-9999-4999-8999-999999999999")
	firstRecordID := mustID(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	secondRecordID := mustID(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	missingID := mustID(t, "cccccccc-cccc-4ccc-8ccc-cccccccccccc")

	if _, err := database.Pool().Exec(ctx, "DELETE FROM users WHERE id = $1", userID.String()); err != nil {
		t.Fatalf("clean stale repository data: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := database.Pool().Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID.String()); err != nil {
			t.Errorf("clean repository data: %v", err)
		}
	})

	users := NewUserRepository(database.Pool())
	sessions := NewSessionRepository(database.Pool())
	records := NewRecordRepository(database.Pool())
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	user := integrationUser(userID, fmt.Sprintf("errors-%d", time.Now().UnixNano()), createdAt)

	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("UserRepository.Create() error = %v", err)
	}
	if err := users.Create(ctx, user); !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("duplicate UserRepository.Create() error = %v, want ErrAlreadyExists", err)
	}
	if _, err := users.GetByLogin(ctx, "missing-login"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing GetByLogin() error = %v, want ErrNotFound", err)
	}
	if _, err := users.GetByID(ctx, missingID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing GetByID() error = %v, want ErrNotFound", err)
	}

	session := integrationSession(sessionID, userID, []byte("repository-token-hash"), createdAt)
	if err := sessions.Create(ctx, session); err != nil {
		t.Fatalf("SessionRepository.Create() error = %v", err)
	}
	duplicateSession := session
	duplicateSession.ID = missingID
	if err := sessions.Create(ctx, duplicateSession); !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("duplicate SessionRepository.Create() error = %v, want ErrAlreadyExists", err)
	}
	if _, err := sessions.GetByTokenHash(ctx, []byte("missing-token")); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing GetByTokenHash() error = %v, want ErrNotFound", err)
	}
	if err := sessions.Revoke(ctx, missingID, createdAt.Add(time.Minute)); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing Revoke() error = %v, want ErrNotFound", err)
	}

	first := integrationRecord(firstRecordID, userID, model.RecordTypeCredentials)
	second := integrationRecord(secondRecordID, userID, model.RecordTypeText)
	createdFirst, err := records.Create(ctx, first)
	if err != nil {
		t.Fatalf("create first record: %v", err)
	}
	if _, err := records.Create(ctx, first); !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("duplicate RecordRepository.Create() error = %v, want ErrAlreadyExists", err)
	}
	if _, err := records.Create(ctx, second); err != nil {
		t.Fatalf("create second record: %v", err)
	}

	page, err := records.List(ctx, userID, firstRecordID, 10)
	if err != nil || len(page) != 1 || page[0].ID != secondRecordID {
		t.Fatalf("paginated List() = %+v, %v", page, err)
	}
	if _, err := records.List(ctx, userID, "", 0); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("invalid List() error = %v, want ErrInvalidInput", err)
	}
	if _, err := records.ListChangedAfter(ctx, userID, -1, 10); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("negative ListChangedAfter() error = %v, want ErrInvalidInput", err)
	}
	if _, err := records.ListChangedAfter(ctx, userID, 0, 0); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("invalid-limit ListChangedAfter() error = %v, want ErrInvalidInput", err)
	}
	if _, err := records.Get(ctx, userID, missingID); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing RecordRepository.Get() error = %v, want ErrNotFound", err)
	}
	if _, err := records.Update(ctx, first, 0); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("invalid-version Update() error = %v, want ErrInvalidInput", err)
	}
	missingRecord := integrationRecord(missingID, userID, model.RecordTypeText)
	if _, err := records.Update(ctx, missingRecord, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing Update() error = %v, want ErrNotFound", err)
	}
	if _, err := records.Delete(ctx, userID, firstRecordID, 0); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("invalid-version Delete() error = %v, want ErrInvalidInput", err)
	}
	if _, err := records.Delete(ctx, userID, missingID, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("missing Delete() error = %v, want ErrNotFound", err)
	}

	tombstone, err := records.Delete(ctx, userID, firstRecordID, createdFirst.Version)
	if err != nil || !tombstone.Deleted() {
		t.Fatalf("Delete() = %+v, %v", tombstone, err)
	}
	if _, err := records.Delete(ctx, userID, firstRecordID, tombstone.Version); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("repeated Delete() error = %v, want ErrNotFound", err)
	}
}

func integrationUser(id model.ID, login string, createdAt time.Time) model.User {
	return model.User{
		ID:                   id,
		Login:                login,
		PasswordHash:         "encoded-password-hash",
		EncryptedDataKey:     []byte("encrypted-key"),
		KeySalt:              []byte("salt"),
		KeyNonce:             []byte("nonce"),
		KeyDerivationVersion: 1,
		CreatedAt:            createdAt,
	}
}

func integrationSession(id, userID model.ID, tokenHash []byte, createdAt time.Time) model.Session {
	return model.Session{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(time.Hour),
	}
}

func integrationRecord(id, userID model.ID, recordType model.RecordType) model.Record {
	return model.Record{
		ID:                id,
		UserID:            userID,
		Type:              recordType,
		EncryptionVersion: model.CurrentRecordEncryptionVersion,
		EncryptedPayload:  []byte("0123456789abcdef"),
		EncryptedMetadata: []byte("fedcba9876543210"),
		PayloadNonce:      []byte("123456789012"),
		MetadataNonce:     []byte("210987654321"),
	}
}
