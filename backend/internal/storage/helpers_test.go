package storage

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(DBConfig{Driver: DriverSQLite, DSN: path})
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}