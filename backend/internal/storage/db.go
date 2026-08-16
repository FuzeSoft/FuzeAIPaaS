package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"fuze-ai-paas/backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

const defaultSQLitePath = "fuze-ai-paas.db"

type DBConfig struct {
	Driver string
	DSN    string
}

func DBConfigFromEnv(getenv func(string) string) DBConfig {
	driver := strings.ToLower(strings.TrimSpace(getenv("DB_DRIVER")))
	if driver == "" {
		driver = DriverSQLite
	}
	dsn := strings.TrimSpace(getenv("DB_DSN"))
	if dsn == "" && driver == DriverSQLite {
		
		dsn = strings.TrimSpace(getenv("DB_PATH"))
	}
	if dsn == "" && driver == DriverSQLite {
		dsn = defaultSQLitePath
	}
	return DBConfig{Driver: driver, DSN: dsn}
}

func OpenDB(cfg DBConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case DriverSQLite:
		dsn := cfg.DSN
		if dsn == "" {
			dsn = defaultSQLitePath
		}
		dialector = sqlite.Open(dsn)
	case DriverPostgres:
		
		if strings.TrimSpace(cfg.DSN) == "" {
			return nil, fmt.Errorf("storage: postgres driver requires DB_DSN")
		}
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("storage: unsupported db driver %q (want %q or %q)", cfg.Driver, DriverSQLite, DriverPostgres)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}
	
	if sqlDB, perr := db.DB(); perr == nil {
		ConfigureConnectionPool(sqlDB)
	}
	
	if cfg.Driver == DriverSQLite {
		if err := configureSQLiteConcurrency(db); err != nil {
			return nil, err
		}
	}
	if err := Migrate(db); err != nil {
		return nil, err
	}
	seedData(db)
	return db, nil
}

func configureSQLiteConcurrency(db *gorm.DB) error {
	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	}
	for _, p := range pragmas {
		if err := db.Exec(p).Error; err != nil {
			return fmt.Errorf("storage: sqlite pragma %q failed: %w", p, err)
		}
	}
	return nil
}

func ConfigureConnectionPool(sqlDB *sql.DB) {
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)
}

func NewDBFromEnv(getenv func(string) string) (*gorm.DB, error) {
	return NewDBFromEnvWithPath(getenv, "")
}

func NewDBFromEnvWithPath(getenv func(string) string, path string) (*gorm.DB, error) {
	cfg := DBConfigFromEnv(getenv)
	if strings.TrimSpace(path) != "" && cfg.Driver == DriverSQLite {
		cfg.DSN = strings.TrimSpace(path)
	}
	return OpenDB(cfg)
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.Resource{},
		&models.Job{},
		&models.Cluster{},
		&models.Queue{},
		&models.InferenceService{},
		&models.Dataset{},
		&models.Model{},
		&models.ModelVersion{},
		
		&models.User{},
		&models.Tenant{},
		&models.Quota{},
		&models.AuditLog{},
		
		&models.Experiment{},
		&models.Run{},
		
		&models.HPOStudy{},
		&models.HPOTrial{},
		&models.HPOTrialReport{},
		
		&models.PersonalAccessToken{},
		
		&models.TokenBlacklist{},
		
		&models.Evaluation{},
		
		&models.EvaluationReview{},
		
		&models.LLMRoute{},
		&models.LLMPrice{},
		&models.GPUPrice{},
		&models.GPUUsageRecord{},
		&models.LLMTokenQuota{},
		&models.LLMUsageRecord{},
		&models.LLMTrace{},
		&models.LLMPrompt{},
		&models.LLMKnowledgeBase{},
		&models.LLMDocument{},
		&models.LLMAdapter{},
		
		&models.LLMAdapterMount{},
		
		&models.LLMGuardrailRule{},
		
		&models.ModelCredential{},
		
		&models.Workspace{},
		
		&models.IdPConfig{},
		
		&models.AlertRule{},
		&models.AlertSilence{},
		
		&AgentRow{},
		&AgentRunRow{},
		
		&ToolRow{},
		
		&models.DataPipeline{},
		&models.PipelineStep{},
		&models.PipelineStepRun{},
		&models.AnnotationTask{},
		
		&models.LineageEdge{},
		
		&models.CompressionTask{},
		
		&models.EdgeNodeRow{},
		&models.EdgeDeploymentRow{},
		&models.DriftReportRow{},
		&models.DriftBaselineRow{},
		&models.EdgeLabelFeedbackRow{},
	)
}