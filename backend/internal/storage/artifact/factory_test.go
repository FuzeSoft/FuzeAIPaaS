package artifact

import (
	"testing"

	"fuze-ai-paas/backend/internal/ports"
)

func TestConfigFromEnvDefaultsToLocal(t *testing.T) {
	cfg := ConfigFromEnv(func(string) string { return "" })
	if cfg.Backend != BackendLocal {
		t.Fatalf("expected local backend by default, got %q", cfg.Backend)
	}
	if cfg.LocalRoot == "" {
		t.Fatal("default local root must not be empty")
	}
}

func TestConfigFromEnvReadsS3(t *testing.T) {
	env := map[string]string{
		"ARTIFACT_BACKEND":       "s3",
		"ARTIFACT_S3_ENDPOINT":   "minio.svc:9000",
		"ARTIFACT_S3_BUCKET":     "fuze-artifacts",
		"ARTIFACT_S3_ACCESS_KEY": "ak",
		"ARTIFACT_S3_SECRET_KEY": "sk",
		"ARTIFACT_S3_USE_SSL":    "true",
		"ARTIFACT_S3_REGION":     "cn-north-1",
	}
	cfg := ConfigFromEnv(func(k string) string { return env[k] })
	if cfg.Backend != BackendS3 || cfg.Endpoint != "minio.svc:9000" || cfg.Bucket != "fuze-artifacts" {
		t.Fatalf("s3 config not parsed: %+v", cfg)
	}
	if !cfg.UseSSL || cfg.Region != "cn-north-1" || cfg.AccessKey != "ak" || cfg.SecretKey != "sk" {
		t.Fatalf("s3 config incomplete: %+v", cfg)
	}
}

func TestNewLocalBackend(t *testing.T) {
	store, err := New(Config{Backend: BackendLocal, LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("New local: %v", err)
	}
	if _, ok := store.(*LocalStore); !ok {
		t.Fatalf("expected *LocalStore, got %T", store)
	}
	var _ ports.ArtifactStore = store
}

func TestNewRejectsUnknownBackend(t *testing.T) {
	if _, err := New(Config{Backend: "ftp"}); err == nil {
		t.Fatal("unknown backend must be rejected")
	}
}

func TestNewS3RequiresEndpointAndBucket(t *testing.T) {
	if _, err := New(Config{Backend: BackendS3, Bucket: "b"}); err == nil {
		t.Fatal("missing endpoint must be rejected")
	}
	if _, err := New(Config{Backend: BackendS3, Endpoint: "e:9000"}); err == nil {
		t.Fatal("missing bucket must be rejected")
	}
}