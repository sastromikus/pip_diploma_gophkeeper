package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Database owns the PostgreSQL connection pool used by server repositories.
type Database struct {
	pool *pgxpool.Pool
}

// Open parses the DSN, creates a connection pool, and verifies connectivity.
func Open(ctx context.Context, dsn string) (*Database, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL configuration: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return &Database{pool: pool}, nil
}

// Close releases all PostgreSQL connections.
func (db *Database) Close() {
	if db == nil || db.pool == nil {
		return
	}
	db.pool.Close()
}

// Pool exposes the pool for constructing repositories in the composition root.
func (db *Database) Pool() *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.pool
}
