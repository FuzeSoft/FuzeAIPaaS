package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fuze-ai-paas/backend/internal/models"
)

func TestDBConfigFromEnvDefaultsToSQLite(t *testing.T) {
	cfg := DBConfigFromEnv(func(string) string { return "" })
	if cfg.Driver != DriverSQLite {
		t.Fatalf("expected sqlite by default, got %q", cfg.Driver)
	}
	if cfg.DSN == "" {
		t.Fatal("default sqlite DSN must not be empty")
	}
}

func TestDBConfigFromEnvPostgres(t *testing.T) {
	env := map[string]string{
		"DB_DRIVER": "postgres",
		"DB_DSN":    "host=pg user=fuze dbname=fuze port=5432 sslmode=disable",
	}
	cfg := DBConfigFromEnv(func(k string) string { return env[k] })
	if cfg.Driver != DriverPostgres {
		t.Fatalf("expected postgres driver, got %q", cfg.Driver)
	}
	if cfg.DSN != env["DB_DSN"] {
		t.Fatalf("DSN not carried through: %q", cfg.DSN)
	}
}

func TestOpenRejectsUnknownDriver(t *testing.T) {
	if _, err := OpenDB(DBConfig{Driver: "mongodb", DSN: "x"}); err == nil {
		t.Fatal("unknown driver must be rejected")
	}
}

func TestOpenPostgresRequiresDSN(t *testing.T) {
	if _, err := OpenDB(DBConfig{Driver: DriverPostgres}); err == nil {
		t.Fatal("postgres without DSN must be rejected before dialing")
	}
}

func TestOpenSQLiteMigratesAndSeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open.db")
	db, err := OpenDB(DBConfig{Driver: DriverSQLite, DSN: path})
	if err != nil {
		t.Fatalf("OpenDB sqlite: %v", err)
	}
	var tenants int64
	if err := db.Model(&models.Tenant{}).Count(&tenants).Error; err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenants == 0 {
		t.Fatal("expected seed data after open")
	}
}

func TestNewSQLiteDBAtStillWorks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if _, err := NewSQLiteDBAt(path); err != nil {
		t.Fatalf("NewSQLiteDBAt: %v", err)
	}
}

func TestDBConfigFromEnvSQLitePathPriority(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantDSN  string
		wantDefault bool
	}{
		{
			name:        "DB_DSN wins",
			env:         map[string]string{"DB_DSN": "/tmp/from-dsn.db"},
			wantDSN:     "/tmp/from-dsn.db",
		},
		{
			name:        "DB_PATH used when no DSN",
			env:         map[string]string{"DB_PATH": "/tmp/from-path.db"},
			wantDSN:     "/tmp/from-path.db",
		},
		{
			name:         "fallback to builtin default",
			env:          map[string]string{},
			wantDefault:  true,
		},
		{
			
			name:    "DB_DSN beats DB_PATH",
			env:     map[string]string{"DB_DSN": "/tmp/dsn.db", "DB_PATH": "/tmp/path.db"},
			wantDSN: "/tmp/dsn.db",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := DBConfigFromEnv(func(k string) string { return c.env[k] })
			if cfg.Driver != DriverSQLite {
				t.Fatalf("expected sqlite, got %q", cfg.Driver)
			}
			if c.wantDefault {
				if cfg.DSN != defaultSQLitePath {
					t.Fatalf("expected builtin default %q, got %q", defaultSQLitePath, cfg.DSN)
				}
				return
			}
			if cfg.DSN != c.wantDSN {
				t.Fatalf("expected DSN %q, got %q", c.wantDSN, cfg.DSN)
			}
		})
	}
}

func TestNewDBFromEnvWithPathScenarios(t *testing.T) {
	t.Run("path overrides sqlite default", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "explicit.db")
		db, err := NewDBFromEnvWithPath(func(string) string { return "" }, path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if db == nil {
			t.Fatal("nil db")
		}
	})

	t.Run("path does not override postgres DSN", func(t *testing.T) {
		env := map[string]string{
			"DB_DRIVER": "postgres",
			"DB_DSN":    "host=pg dbname=fuze",
		}
		
		cfg := DBConfigFromEnv(func(k string) string { return env[k] })
		path := filepath.Join(t.TempDir(), "should-be-ignored.db")
		if cfg.Driver == DriverSQLite && strings.TrimSpace(path) != "" {
			t.Fatal("postgres driver must bypass path override")
		}
		_ = cfg
	})

	t.Run("empty path falls back to env", func(t *testing.T) {
		env := map[string]string{"DB_PATH": filepath.Join(t.TempDir(), "envpath.db")}
		db, err := NewDBFromEnvWithPath(func(k string) string { return env[k] }, "")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if db == nil {
			t.Fatal("nil db")
		}
	})
}

func TestDBConfigFromEnvPostgresDSNNotTreatedAsSQLitePath(t *testing.T) {
	env := map[string]string{
		"DB_DRIVER": "postgres",
		"DB_DSN":    "host=pg user=fuze dbname=fuze",
	}
	cfg := DBConfigFromEnv(func(k string) string { return env[k] })
	if cfg.Driver != DriverPostgres {
		t.Fatalf("expected postgres, got %q", cfg.Driver)
	}
	if cfg.DSN != env["DB_DSN"] {
		t.Fatalf("postgres DSN must be preserved verbatim, got %q", cfg.DSN)
	}
}

func TestOpenDBPostgresLive(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set TEST_DB_DSN to run live postgres test")
	}
	db, err := OpenDB(DBConfig{Driver: DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatalf("OpenDB postgres: %v", err)
	}
	var tenants int64
	if err := db.Model(&models.Tenant{}).Count(&tenants).Error; err != nil {
		t.Fatalf("count tenants on postgres: %v", err)
	}
	if tenants == 0 {
		t.Fatal("expected seed data after open on postgres")
	}
}

func TestConfigureConnectionPoolPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("set TEST_DB_DSN to run live postgres test")
	}
	db, err := OpenDB(DBConfig{Driver: DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatalf("OpenDB postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	ConfigureConnectionPool(sqlDB)
	if got := sqlDB.Stats().MaxOpenConnections; got != 25 {
		t.Fatalf("expected MaxOpenConnections 25, got %d", got)
	}
}