package postgres

import (
	"io/fs"
	"strings"
	"testing"
)

func TestMigrationTableName(t *testing.T) {
	if migrationTableName != "gophkeeper_goose_db_version" {
		t.Fatalf("migrationTableName = %q", migrationTableName)
	}
}

func TestEmbeddedInitialMigration(t *testing.T) {
	contents, err := fs.ReadFile(migrationFiles, "migrations/00001_initial.sql")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	sql := string(contents)
	required := []string{
		"-- +goose Up",
		"CREATE TABLE users",
		"CREATE TABLE sessions",
		"CREATE TABLE records",
		"CREATE SEQUENCE IF NOT EXISTS gophkeeper_records_revision_seq",
		"records_active_or_tombstone",
		"-- +goose Down",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
}
