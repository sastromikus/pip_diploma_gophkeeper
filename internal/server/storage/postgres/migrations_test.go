package postgres

import (
	"io/fs"
	"strings"
	"testing"
)

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
		"CREATE SEQUENCE records_revision_seq",
		"records_active_or_tombstone",
		"-- +goose Down",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration does not contain %q", fragment)
		}
	}
}
