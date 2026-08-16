package storage

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	driver := os.Getenv("TEST_DB_DRIVER")
	dsn := os.Getenv("TEST_DB_DSN")

	var dialector gorm.Dialector
	switch driver {
	case DriverPostgres:
		if dsn == "" {
			t.Fatalf("TEST_DB_DRIVER=postgres requires TEST_DB_DSN")
		}
		
		schema := "t_" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))
		dialector = postgres.Open(dsn)
		base, err := gorm.Open(dialector, &gorm.Config{})
		if err != nil {
			t.Fatalf("open postgres (base): %v", err)
		}
		if err := base.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %q", schema)).Error; err != nil {
			t.Fatalf("create schema %s: %v", schema, err)
		}
		
		scoped := postgres.Open(dsn + fmt.Sprintf("&search_path=%s", schema))
		db, err := gorm.Open(scoped, &gorm.Config{})
		if err != nil {
			t.Fatalf("open postgres (scoped): %v", err)
		}
		if err := Migrate(db); err != nil {
			t.Fatalf("migrate test db: %v", err)
		}
		t.Cleanup(func() {
			if sqlDB, e := db.DB(); e == nil {
				_ = sqlDB.Close()
			}
			
			if err := base.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %q CASCADE", schema)).Error; err != nil {
				t.Logf("drop schema %s: %v", schema, err)
			}
			if sqlDB, e := base.DB(); e == nil {
				_ = sqlDB.Close()
			}
		})
		return db
	default:
		
		dialector = sqlite.Open(":memory:")
		db, err := gorm.Open(dialector, &gorm.Config{})
		if err != nil {
			t.Fatalf("open test db: %v", err)
		}
		if err := Migrate(db); err != nil {
			t.Fatalf("migrate test db: %v", err)
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		return db
	}
}

func TestTestDBFallsBackToSQLite(t *testing.T) {
	old := os.Getenv("TEST_DB_DRIVER")
	_ = os.Unsetenv("TEST_DB_DRIVER")
	defer os.Setenv("TEST_DB_DRIVER", old)

	db := testDB(t)

	var n int64
	if err := db.Model(&models.Tenant{}).Count(&n).Error; err != nil {
		t.Fatalf("count tenants on sqlite fallback: %v", err)
	}
}

func TestSeedIdempotent(t *testing.T) {
	db := testDB(t)

	seedData(db)
	
	seedData(db)

	var tenants int64
	if err := db.Model(&models.Tenant{}).Count(&tenants).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenants != 1 {
		t.Fatalf("expected exactly 1 tenant after idempotent seed, got %d", tenants)
	}

	var users int64
	if err := db.Model(&models.User{}).Count(&users).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 1 {
		t.Fatalf("expected exactly 1 user after idempotent seed, got %d", users)
	}
}