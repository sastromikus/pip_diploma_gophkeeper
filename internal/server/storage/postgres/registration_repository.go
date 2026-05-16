package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sastromikus/gophkeeper/internal/model"
)

// RegistrationRepository atomically creates a user and their initial session.
type RegistrationRepository struct {
	pool *pgxpool.Pool
}

// NewRegistrationRepository creates a PostgreSQL registration repository.
func NewRegistrationRepository(pool *pgxpool.Pool) *RegistrationRepository {
	return &RegistrationRepository{pool: pool}
}

// CreateUserAndSession persists registration state in one transaction.
func (repository *RegistrationRepository) CreateUserAndSession(ctx context.Context, user model.User, session model.Session) error {
	if err := user.Validate(); err != nil {
		return err
	}
	if err := session.Validate(); err != nil {
		return err
	}
	if session.UserID != user.ID {
		return fmt.Errorf("%w: session user does not match registered user", model.ErrInvalidInput)
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
        INSERT INTO users (
            id, login, password_hash, encrypted_data_key, key_salt, key_nonce,
            key_derivation_version, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `,
		user.ID.String(), user.Login, user.PasswordHash, user.EncryptedDataKey,
		user.KeySalt, user.KeyNonce, user.KeyDerivationVersion, user.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrAlreadyExists
		}
		return fmt.Errorf("insert registered user: %w", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at, revoked_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `,
		session.ID.String(), session.UserID.String(), session.TokenHash,
		session.CreatedAt, session.ExpiresAt, session.RevokedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return model.ErrAlreadyExists
		}
		return fmt.Errorf("insert initial session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit registration transaction: %w", err)
	}
	return nil
}
