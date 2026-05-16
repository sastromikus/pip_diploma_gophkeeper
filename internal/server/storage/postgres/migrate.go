package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationTableName = "gophkeeper_goose_db_version"

var migrationMu sync.Mutex

// Migrate applies all embedded PostgreSQL migrations to the configured database.
func Migrate(ctx context.Context, dsn string) (resultErr error) {
	migrationMu.Lock()
	defer migrationMu.Unlock()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL for migrations: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close migration database: %w", err)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL for migrations: %w", err)
	}

	goose.SetBaseFS(migrationFiles)
	goose.SetTableName(migrationTableName)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply PostgreSQL migrations: %w", err)
	}
	return nil
}
