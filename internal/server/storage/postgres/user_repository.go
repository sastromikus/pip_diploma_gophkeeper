package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const userColumns = `id::text, login, password_hash, encrypted_data_key, key_salt, key_nonce, key_derivation_version, created_at`

// UserRepository persists registered users.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a PostgreSQL user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create inserts a user and maps login uniqueness violations to ErrAlreadyExists.
func (repository *UserRepository) Create(ctx context.Context, user model.User) error {
	if err := user.Validate(); err != nil {
		return err
	}
	_, err := repository.pool.Exec(ctx, `
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
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByLogin returns a user by exact login.
func (repository *UserRepository) GetByLogin(ctx context.Context, login string) (model.User, error) {
	user, err := scanUser(repository.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE login = $1`, login,
	))
	if err != nil {
		if mapped := mapNotFound(err); errors.Is(mapped, model.ErrNotFound) {
			return model.User{}, model.ErrNotFound
		}
		return model.User{}, fmt.Errorf("select user by login: %w", err)
	}
	return user, nil
}

// GetByID returns a user by identifier.
func (repository *UserRepository) GetByID(ctx context.Context, id model.ID) (model.User, error) {
	user, err := scanUser(repository.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id.String(),
	))
	if err != nil {
		if mapped := mapNotFound(err); errors.Is(mapped, model.ErrNotFound) {
			return model.User{}, model.ErrNotFound
		}
		return model.User{}, fmt.Errorf("select user by ID: %w", err)
	}
	return user, nil
}
