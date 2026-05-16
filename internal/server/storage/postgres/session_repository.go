package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sastromikus/gophkeeper/internal/model"
)

const sessionColumns = `id::text, user_id::text, token_hash, created_at, expires_at, revoked_at`

// SessionRepository persists opaque authenticated sessions.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository creates a PostgreSQL session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Create inserts a new session.
func (repository *SessionRepository) Create(ctx context.Context, session model.Session) error {
	if err := session.Validate(); err != nil {
		return err
	}
	_, err := repository.pool.Exec(ctx, `
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
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetByTokenHash returns a session matching the stored token hash.
func (repository *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash []byte) (model.Session, error) {
	session, err := scanSession(repository.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE token_hash = $1`, tokenHash,
	))
	if err != nil {
		if mapped := mapNotFound(err); errors.Is(mapped, model.ErrNotFound) {
			return model.Session{}, model.ErrNotFound
		}
		return model.Session{}, fmt.Errorf("select session by token hash: %w", err)
	}
	return session, nil
}

// Revoke marks a session as revoked. It is idempotent for an already revoked session.
func (repository *SessionRepository) Revoke(ctx context.Context, id model.ID, revokedAt time.Time) error {
	command, err := repository.pool.Exec(ctx, `
        UPDATE sessions
        SET revoked_at = COALESCE(revoked_at, $2)
        WHERE id = $1
    `, id.String(), revokedAt)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if command.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
